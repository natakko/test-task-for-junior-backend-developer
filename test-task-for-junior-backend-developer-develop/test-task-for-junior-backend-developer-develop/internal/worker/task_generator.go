package worker

import (
	"context"
	"log"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type TaskRepository interface {
	Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	ListActiveTemplates(ctx context.Context) ([]taskdomain.RecurrenceConfig, error)
	TemplateExistsForDate(ctx context.Context, templateID int64, date time.Time) (bool, error)
}

type Worker struct {
	repo TaskRepository
}

func New(repo TaskRepository) *Worker {
	return &Worker{repo: repo}
}

func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.GenerateTasksForDate(ctx, time.Now())
		}
	}
}

func (w *Worker) GenerateTasksForDate(ctx context.Context, date time.Time) error {
	templates, err := w.repo.ListActiveTemplates(ctx)
	if err != nil {
		return err
	}

	for _, template := range templates {
		if template.ShouldCreateTask(date) {
			// Проверяем, что template.ID не nil
			if template.ID == nil {
				log.Printf("template ID is nil for template: %s", template.Title)
				continue
			}

			exists, err := w.repo.TemplateExistsForDate(ctx, *template.ID, date)
			if err != nil {
				log.Printf("failed to check template existence: %v", err)
				continue
			}
			if exists {
				continue
			}

			newTask := &taskdomain.Task{
				Title:       template.Title,
				Description: template.Description,
				Status:      taskdomain.StatusNew,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			_, err = w.repo.Create(ctx, newTask)
			if err != nil {
				log.Printf("failed to auto-create task: %v", err)
			} else {
				log.Printf("successfully created task from template %d for date %s", *template.ID, date.Format("2006-01-02"))
			}
		}
	}

	return nil
}