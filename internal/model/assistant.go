package model

import "time"

// AIConversation 对话会话
type AIConversation struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Title     string    `gorm:"type:varchar(255);not null" json:"title"`
	GPUUUID   string    `gorm:"type:varchar(128);index" json:"gpu_uuid"` // 当前会话关联的 GPU
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `gorm:"index" json:"updated_at"`
}

// AIMessage 对话消息
type AIMessage struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ConversationID uint64    `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"type:varchar(16);not null" json:"role"` // user/assistant
	Content        string    `gorm:"type:text;not null" json:"content"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}

func (AIMessage) TableName() string { return "ai_message" }
