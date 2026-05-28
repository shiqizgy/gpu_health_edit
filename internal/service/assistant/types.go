package assistant

// Package assistant 实现 GPU 故障分析 AI 助手。
//
// 架构分三层(为后续 function calling / 集群钻取预留):
//   - context_provider.go : 数据采集层,把系统数据组装成模型可读的上下文
//   - llm_client.go       : 大模型客户端,封装 DeepSeek 流式调用(留 tools 参数)
//   - service.go          : 对话编排层,串起采集 + 模型调用

// Role 对话角色
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message 一条对话消息(OpenAI 兼容格式)
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls / ToolCallID 第一版用不到,但结构先留好,
	// 未来 function calling 直接用,不改类型定义。
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool 工具定义(第一版传 nil,未来 function calling 用)
type Tool struct {
	Type     string       `json:"type"` // 固定 "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall 模型要求调用的工具(未来用)
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ---- DeepSeek 请求/响应结构(OpenAI 兼容) ----

// chatRequest 发给 DeepSeek 的请求体
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []Tool    `json:"tools,omitempty"` // 第一版为空
}

// streamChunk DeepSeek 流式返回的每一段(SSE 的 data: 后面那段 JSON)
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			// ToolCalls 未来 function calling 用
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// errorResponse DeepSeek 返回的错误体
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}
