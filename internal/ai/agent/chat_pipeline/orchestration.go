package chat_pipeline

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

/*
编排图中的节点：
- ChatModelNode:调用大模型
- RetrieverNode:从向量数据库中检索数据
- LambdaNode:数据预处理
- ChatTemplateNode:构建提示词
*/

func BuildChatAgent(ctx context.Context) (r compose.Runnable[*UserMessage, *schema.Message], err error) {
	const (
		InputToRag      = "InputToRag"
		ChatTemplate    = "ChatTemplate"
		ReactAgent      = "ReactAgent"
		MilvusRetriever = "MilvusRetriever"
		InputToChat     = "InputToChat"
	)
	g := compose.NewGraph[*UserMessage, *schema.Message]()
	/*
		添加数据预处理节点
		newInputToRagLambda:仅只把包裹里的 input.Query（也就是用户的原始提问文本，是个单纯的字符串）给掏了出来并返回。
		why:下一个 MilvusRetriever 节点需要的是一个字符串（Query），而不是 UserMessage 这个复杂的结构体。
	*/
	_ = g.AddLambdaNode(InputToRag, compose.InvokableLambdaWithOption(newInputToRagLambda), compose.WithNodeName("UserMessageToRag"))
	/*
		添加构建提示词节点
		newChatTemplate:构建提示词
		why:下一个 ReactAgent 节点需要的是一个提示词（Prompt），而不是 UserMessage 这个复杂的结构体。
	*/
	chatTemplateKeyOfChatTemplate, err := newChatTemplate(ctx)
	if err != nil {
		return nil, err
	}
	_ = g.AddChatTemplateNode(ChatTemplate, chatTemplateKeyOfChatTemplate)
	// 添加 ReActAgent 节点
	reactAgentKeyOfLambda, err := newReactAgentLambda(ctx)
	if err != nil {
		return nil, err
	}
	_ = g.AddLambdaNode(ReactAgent, reactAgentKeyOfLambda, compose.WithNodeName("ReActAgent"))
	// 添加向量数据库检索节点
	milvusRetrieverKeyOfRetriever, err := newRetriever(ctx)
	if err != nil {
		return nil, err
	}
	// 注意下面的 output key 设置，把查询出来的设置为了documents，匹配 ChatTemplate 里面说prompt
	_ = g.AddRetrieverNode(MilvusRetriever, milvusRetrieverKeyOfRetriever, compose.WithOutputKey("documents"))

	_ = g.AddLambdaNode(InputToChat, compose.InvokableLambdaWithOption(newInputToChatLambda), compose.WithNodeName("UserMessageToChat"))
	_ = g.AddEdge(compose.START, InputToRag)
	_ = g.AddEdge(compose.START, InputToChat)
	_ = g.AddEdge(ReactAgent, compose.END)
	_ = g.AddEdge(InputToRag, MilvusRetriever)
	_ = g.AddEdge(MilvusRetriever, ChatTemplate)
	_ = g.AddEdge(InputToChat, ChatTemplate)
	_ = g.AddEdge(ChatTemplate, ReactAgent)
	// compose.WithNodeTriggerMode(compose.AllPredecessor): 只有所有前置节点都执行了，才会执行当前节点
	r, err = g.Compile(ctx, compose.WithGraphName("ChatAgent"), compose.WithNodeTriggerMode(compose.AllPredecessor))
	if err != nil {
		return nil, err
	}
	return r, err
}
