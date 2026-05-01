package task

import "time"

// Status статус задачи
type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

// Task основная структура задачи
type Task struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      Status            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Recurrence  *RecurrenceConfig `json:"recurrence,omitempty"`
	TemplateID  *int64            `json:"template_id,omitempty"`
}

// Valid проверяет корректность статуса
func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}