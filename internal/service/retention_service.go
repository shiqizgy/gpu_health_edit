package service

import (
	"time"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/logger"
)

// RetentionService 定期清理时序/时序型数据（目前只有故障事件、AI对话、以及评分表），
// 只保留最近 RetainDays 天，控制数据库存储占用。
// 固定数据（拓扑/策略/指标/知识/规则）与恒定行数表（快照/汇总）不在清理范围。
type RetentionService struct {
	cfg        config.RententionConfig
	faultEvent *repository.FaultEventRepo
	assistant  *repository.AssistantRepo
}

func NewRetentionService(
	cfg config.RententionConfig,
	faultEvent *repository.FaultEventRepo,
	assistant *repository.AssistantRepo,
) *RetentionService {
	return &RetentionService{cfg: cfg, faultEvent: faultEvent, assistant: assistant}
}

// RunOnce 执行一轮清理。
func (s *RetentionService) RunOnce() {
	days := s.cfg.RetainDays
	if days <= 0 {
		days = 3
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	batch := s.cfg.BatchSize

	if n, err := s.faultEvent.PurgeResolvedBefore(cutoff, batch); err != nil {
		logger.L.Errorf("清理历史故障事件失败: %v", err)
	} else if n > 0 {
		logger.L.Infof("清理 resolved 故障事件 %d 条(早于 %s)", n, cutoff.Format("2006-01-02"))
	}

	if n, err := s.assistant.PurgeConversationsBefore(cutoff, batch); err != nil {
		logger.L.Errorf("清理历史对话失败: %v", err)
	} else if n > 0 {
		logger.L.Infof("清理历史对话 %d 条(早于 %s)", n, cutoff.Format("2006-01-02"))
	}
}
