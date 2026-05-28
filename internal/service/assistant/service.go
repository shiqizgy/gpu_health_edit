package assistant

import (
	"context"
	"fmt"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/redisclient"
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
}

func NewService(
	cfg config.AssistantConfig,
	topo *repository.TopologyRepo,
	health *repository.HealthRepo,
	metric *repository.MetricRepo,
	fault *repository.FaultRepo,
	rc *redisclient.Client,
) *Service {
	llm := NewDeepSeekClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.TimeoutSec)
	provider := NewGPUContextProvider(topo, health, metric, fault, rc)
	return &Service{cfg: cfg, llm: llm, provider: provider}
}

// Chat 单卡对话(流式)。
//   - uuid:    要分析的 GPU
//   - message: 用户本轮提问
//   - history: 之前的对话历史(只含 user/assistant 的问答,不含数据上下文)
//   - onDelta: 流式回调,每段文字调一次(由 handler 写成 SSE)
func (s *Service) Chat(
	ctx context.Context,
	uuid, message string,
	history []Message,
	onDelta func(string),
) error {
	if !s.cfg.Enabled {
		return fmt.Errorf("AI 助手未启用,请在配置中开启")
	}
	if uuid == "" {
		return fmt.Errorf("请先指定要分析的 GPU UUID")
	}

	// 1. 采集该卡的真实数据,组装上下文
	contextText, err := s.provider.Build(ctx, uuid)
	if err != nil {
		return err
	}

	// 2. 组装 messages: system + 数据上下文 + 历史 + 本轮提问
	messages := make([]Message, 0, len(history)+3)
	messages = append(messages, Message{Role: RoleSystem, Content: SystemPrompt})
	messages = append(messages, Message{Role: RoleUser, Content: UserContextPrefix + contextText})

	// 截断历史,防止 context 过长(保留最近 maxHistory 条)
	hist := history
	maxH := s.cfg.MaxHistory
	if maxH > 0 && len(hist) > maxH {
		hist = hist[len(hist)-maxH:]
	}
	messages = append(messages, hist...)

	// 本轮用户提问
	messages = append(messages, Message{Role: RoleUser, Content: message})

	// 3. 流式调模型(第一版 tools 传 nil)
	return s.llm.ChatStream(ctx, messages, nil, onDelta)
}
