package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/model"
	"github.com/gpu-health/platform/internal/repository"
)

// Service 对话编排层。
// 第一版: 查单卡数据 → 拼 prompt → 流式调模型。
// 未来升级: 改为"模型↔工具循环"(function calling),
//
//	provider 变成被模型调用的工具,本层只需改编排逻辑。
type Service struct {
	cfg      config.AssistantConfig
	llm      LLMClient
	provider ContextProvider
	repo     *repository.AssistantRepo
}

func NewService(
	cfg config.AssistantConfig,
	topo *repository.TopologyRepo,
	health *repository.HealthRepo,
	metric *repository.MetricRepo,
	fault *repository.FaultRepo,
	ck *ckclient.Client,
	table string,
	assistantRepo *repository.AssistantRepo,
) *Service {
	llm := NewDeepSeekClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.TimeoutSec)
	provider := NewGPUContextProvider(topo, health, metric, fault, ck, table)
	return &Service{cfg: cfg, llm: llm, provider: provider, repo: assistantRepo}
}

// Chat 单卡对话(流式)。
//   - uuid:    要分析的 GPU
//   - message: 用户本轮提问
//   - history: 之前的对话历史(只含 user/assistant 的问答,不含数据上下文)
//   - onDelta: 流式回调,每段文字调一次(由 handler 写成 SSE)
func (s *Service) Chat(
	ctx context.Context,
	conversationID uint64,
	uuid, message string,
	onDelta func(string),
) error {
	if !s.cfg.Enabled {
		return fmt.Errorf("AI 助手未启用")
	}
	if uuid == "" {
		return fmt.Errorf("请先指定要分析的 GPU UUID")
	}
	if conversationID == 0 {
		return fmt.Errorf("会话 ID 缺失")
	}

	// 1. 存用户消息
	if err := s.repo.AppendMessage(&model.AIMessage{
		ConversationID: conversationID,
		Role:           RoleUser,
		Content:        message,
	}); err != nil {
		return fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 2. 取该会话的历史消息(不含刚存的用户消息)
	historyMsgs, err := s.repo.ListMessages(conversationID)
	if err != nil {
		return err
	}
	// 转成 LLM 用的 Message 列表(剥掉刚插入的最新一条,因为下面单独 append)
	history := make([]Message, 0, len(historyMsgs)-1)
	for i, m := range historyMsgs {
		if i == len(historyMsgs)-1 && m.Role == RoleUser && m.Content == message {
			continue // 跳过刚插入的本轮用户消息
		}
		history = append(history, Message{Role: m.Role, Content: m.Content})
	}
	// 截断历史
	if s.cfg.MaxHistory > 0 && len(history) > s.cfg.MaxHistory {
		history = history[len(history)-s.cfg.MaxHistory:]
	}

	// 3. 采集数据上下文
	contextText, err := s.provider.Build(ctx, uuid)
	if err != nil {
		return err
	}

	// 4. 组装 messages
	messages := make([]Message, 0, len(history)+3)
	messages = append(messages, Message{Role: RoleSystem, Content: SystemPrompt})
	messages = append(messages, Message{Role: RoleUser, Content: UserContextPrefix + contextText})
	messages = append(messages, history...)
	messages = append(messages, Message{Role: RoleUser, Content: message})

	// 5. 流式调模型,边吐边累积
	var aiFullContent strings.Builder
	err = s.llm.ChatStream(ctx, messages, nil, func(delta string) {
		aiFullContent.WriteString(delta)
		onDelta(delta)
	})
	if err != nil {
		return err
	}

	// 6. AI 回答完整后存库
	finalContent := aiFullContent.String()
	if finalContent != "" {
		_ = s.repo.AppendMessage(&model.AIMessage{
			ConversationID: conversationID,
			Role:           RoleAssistant,
			Content:        finalContent,
		})
		_ = s.repo.UpdateConversationTouch(conversationID)
	}
	return nil
}
