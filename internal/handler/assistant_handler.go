package handler

import (
	"context"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/service/assistant"
	"github.com/gpu-health/platform/pkg/response"
)

// AssistantHandler GPU 故障分析 AI 助手(SSE 流式)
type AssistantHandler struct {
	svc  *assistant.Service
	repo *repository.AssistantRepo
}

func NewAssistantHandler(svc *assistant.Service, repo *repository.AssistantRepo) *AssistantHandler {
	return &AssistantHandler{svc: svc, repo: repo}
}

// chatRequest 前端请求体
type chatRequest struct {
	ConversationID uint64 `json:"conversation_id"`
	UUID           string `json:"uuid"`
	Message        string `json:"message"`
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

		err := h.svc.Chat(ctx, req.ConversationID, req.UUID, req.Message, func(delta string) {
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

// ---- 会话 CRUD ----

func (h *AssistantHandler) ListConversations(c *gin.Context) {
	list, err := h.repo.ListConversations(50)
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, list)
}

type createConvReq struct {
	Title   string `json:"title"`
	GPUUUID string `json:"gpu_uuid"`
}

func (h *AssistantHandler) CreateConversation(c *gin.Context) {
	var req createConvReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Title == "" {
		req.Title = "新对话"
	}
	conv := &model.AIConversation{Title: req.Title, GPUUUID: req.GPUUUID}
	if err := h.repo.CreateConversation(conv); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, conv)
}

// GetConversation 返回会话信息 + 全部消息
func (h *AssistantHandler) GetConversation(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	conv, err := h.repo.GetConversation(id)
	if err != nil {
		response.Fail(c, 404, "会话不存在")
		return
	}
	msgs, _ := h.repo.ListMessages(id)
	response.OK(c, gin.H{"conversation": conv, "messages": msgs})
}

func (h *AssistantHandler) DeleteConversation(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.repo.DeleteConversation(id); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// UpdateConversation 改标题
type updateConvReq struct {
	Title string `json:"title"`
}

func (h *AssistantHandler) UpdateConversation(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req updateConvReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.repo.UpdateConversationTitle(id, req.Title); err != nil {
		response.ServerError(c, err.Error())
		return
	}
	response.OK(c, nil)
}

// 让 context 包可用(避免某些编辑器误报)
var _ = context.Background
