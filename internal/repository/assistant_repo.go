package repository

import (
	"time"

	"github.com/gpu-health/platform/internal/model"
	"gorm.io/gorm"
)

type AssistantRepo struct{ db *gorm.DB }

func NewAssistantRepo(db *gorm.DB) *AssistantRepo { return &AssistantRepo{db: db} }

// ---- 会话 ----

func (r *AssistantRepo) ListConversations(limit int) ([]model.AIConversation, error) {
	var out []model.AIConversation
	err := r.db.Order("updated_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

func (r *AssistantRepo) CreateConversation(c *model.AIConversation) error {
	return r.db.Create(c).Error
}

func (r *AssistantRepo) GetConversation(id uint64) (*model.AIConversation, error) {
	var c model.AIConversation
	err := r.db.First(&c, id).Error
	return &c, err
}

func (r *AssistantRepo) UpdateConversationTouch(id uint64) error {
	return r.db.Model(&model.AIConversation{}).Where("id = ?", id).
		Update("updated_at", gorm.Expr("NOW()")).Error
}

func (r *AssistantRepo) UpdateConversationTitle(id uint64, title string) error {
	return r.db.Model(&model.AIConversation{}).Where("id = ?", id).
		Update("title", title).Error
}

func (r *AssistantRepo) DeleteConversation(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", id).Delete(&model.AIMessage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.AIConversation{}, id).Error
	})
}

// ---- 消息 ----

func (r *AssistantRepo) ListMessages(convID uint64) ([]model.AIMessage, error) {
	var out []model.AIMessage
	err := r.db.Where("conversation_id = ?", convID).
		Order("created_at ASC").Find(&out).Error
	return out, err
}

func (r *AssistantRepo) AppendMessage(m *model.AIMessage) error {
	return r.db.Create(m).Error
}

// PurgeConversationsBefore 删除 updated_at 早于 cutoff 的会话及其消息（分批）。
func (r *AssistantRepo) PurgeConversationsBefore(cutoff time.Time, batch int) (int64, error) {
	if batch <= 0 {
		batch = 1000
	}
	var total int64
	for {
		// 先查一批过期会话 ID
		var ids []uint64
		if err := r.db.Model(&model.AIConversation{}).
			Where("updated_at < ?", cutoff).
			Limit(batch).Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			break
		}
		// 先删子表消息，再删会话（避免孤儿消息）
		if err := r.db.Where("conversation_id IN ?", ids).
			Delete(&model.AIMessage{}).Error; err != nil {
			return total, err
		}
		res := r.db.Where("id IN ?", ids).Delete(&model.AIConversation{})
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if len(ids) < batch {
			break
		}
	}
	return total, nil
}
