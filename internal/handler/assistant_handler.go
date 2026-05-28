package handler

import (
	"context"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/service/assistant"
	"github.com/gpu-health/platform/pkg/response"
)

// AssistantHandler GPU 故障分析 AI 助手(SSE 流式)
type AssistantHandler struct {
	svc *assistant.Service
}

func NewAssistantHandler(svc *assistant.Service) *AssistantHandler {
	return &AssistantHandler{svc: svc}
}

// chatRequest 前端请求体
type chatRequest struct {
	UUID    string              `json:"uuid"`
	Message string              `json:"message"`
	History []assistant.Message `json:"history"` // 之前的对话(user/assistant)
}

// Chat SSE 流式对话接口。
// 前端用 fetch + ReadableStream 接收。
// SSE 事件类型:
//
//	event: status  → 状态提示(如"正在分析...")
//	event: message → 逐字内容
//	event: error   → 错误信息
//	event: done    → 结束
func (h *AssistantHandler) Chat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求格式错误: "+err.Error())
		return
	}
	if req.Message == "" {
		response.BadRequest(c, "message 不能为空")
		return
	}

	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲(若有反代)

	// 用 context 跟随请求生命周期(前端断开时取消)
	ctx := c.Request.Context()

	// 用 channel 把 service 的流式回调串到 c.Stream 里
	type sseEvent struct {
		event string
		data  string
	}
	ch := make(chan sseEvent, 16)

	// 先发一个"正在分析"状态
	go func() {
		defer close(ch)

		ch <- sseEvent{event: "status", data: "正在查询该 GPU 的实时数据并分析..."}

		err := h.svc.Chat(ctx, req.UUID, req.Message, req.History, func(delta string) {
			// 每段文字推到 channel
			select {
			case ch <- sseEvent{event: "message", data: delta}:
			case <-ctx.Done():
			}
		})
		if err != nil {
			ch <- sseEvent{event: "error", data: err.Error()}
			return
		}
		ch <- sseEvent{event: "done", data: ""}
	}()

	// c.Stream 持续从 channel 读并写给前端,返回 false 时结束
	c.Stream(func(w io.Writer) bool {
		ev, ok := <-ch
		if !ok {
			return false // channel 关闭,结束流
		}
		c.SSEvent(ev.event, ev.data)
		return true
	})
}

// 让 context 包可用(避免某些编辑器误报)
var _ = context.Background
