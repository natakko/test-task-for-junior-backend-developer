package postgres

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    
    taskdomain "example.com/taskservice/internal/domain/task"
)

type Repository struct {
    pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
    return &Repository{pool: pool}
}

// Create создает новую задачу
func (r *Repository) Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
    const query = `
        INSERT INTO tasks (title, description, status, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, title, description, status, created_at, updated_at
    `

    row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.CreatedAt, task.UpdatedAt)
    created, err := scanTask(row)
    if err != nil {
        return nil, err
    }

    return created, nil
}

// GetByID получает задачу по ID
func (r *Repository) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
    const query = `
        SELECT id, title, description, status, created_at, updated_at
        FROM tasks
        WHERE id = $1
    `

    row := r.pool.QueryRow(ctx, query, id)
    found, err := scanTask(row)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, taskdomain.ErrNotFound
        }
        return nil, err
    }

    return found, nil
}

// Update обновляет задачу
func (r *Repository) Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
    const query = `
        UPDATE tasks
        SET title = $1,
            description = $2,
            status = $3,
            updated_at = $4
        WHERE id = $5
        RETURNING id, title, description, status, created_at, updated_at
    `

    row := r.pool.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.UpdatedAt, task.ID)
    updated, err := scanTask(row)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, taskdomain.ErrNotFound
        }
        return nil, err
    }

    return updated, nil
}

// Delete удаляет задачу
func (r *Repository) Delete(ctx context.Context, id int64) error {
    const query = `DELETE FROM tasks WHERE id = $1`

    result, err := r.pool.Exec(ctx, query, id)
    if err != nil {
        return err
    }

    if result.RowsAffected() == 0 {
        return taskdomain.ErrNotFound
    }

    return nil
}

// List возвращает все задачи
func (r *Repository) List(ctx context.Context) ([]taskdomain.Task, error) {
    const query = `
        SELECT id, title, description, status, created_at, updated_at
        FROM tasks
        ORDER BY id DESC
    `

    rows, err := r.pool.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    tasks := make([]taskdomain.Task, 0)
    for rows.Next() {
        task, err := scanTask(rows)
        if err != nil {
            return nil, err
        }
        tasks = append(tasks, *task)
    }

    if err := rows.Err(); err != nil {
        return nil, err
    }

    return tasks, nil
}

// ListActiveTemplates возвращает все активные шаблоны из task_templates
func (r *Repository) ListActiveTemplates(ctx context.Context) ([]taskdomain.RecurrenceConfig, error) {
    const query = `
        SELECT id, title, description, recurrence_type, daily_interval,
               monthly_days, specific_dates, parity_type, start_date,
               end_date, execution_time, status, created_at, updated_at
        FROM task_templates
        WHERE status = 'active'
        ORDER BY id
    `

    rows, err := r.pool.Query(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to query templates: %w", err)
    }
    defer rows.Close()

    var templates []taskdomain.RecurrenceConfig
    for rows.Next() {
        template, err := scanTemplate(rows)
        if err != nil {
            return nil, fmt.Errorf("failed to scan template: %w", err)
        }
        templates = append(templates, *template)
    }

    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("rows iteration error: %w", err)
    }

    return templates, nil
}

// TemplateExistsForDate проверяет, создавалась ли уже задача для шаблона на указанную дату
func (r *Repository) TemplateExistsForDate(ctx context.Context, templateID int64, date time.Time) (bool, error) {
    const query = `
        SELECT EXISTS(
            SELECT 1 FROM task_instances 
            WHERE template_id = $1 
            AND due_date = $2 
            AND is_cancelled = false
        )
    `

    var exists bool
    err := r.pool.QueryRow(ctx, query, templateID, date).Scan(&exists)
    if err != nil {
        return false, fmt.Errorf("failed to check template existence: %w", err)
    }

    return exists, nil
}

// scanTask сканирует строку в Task
func scanTask(scanner interface {
    Scan(dest ...any) error
}) (*taskdomain.Task, error) {
    var (
        task   taskdomain.Task
        status string
    )

    if err := scanner.Scan(
        &task.ID,
        &task.Title,
        &task.Description,
        &status,
        &task.CreatedAt,
        &task.UpdatedAt,
    ); err != nil {
        return nil, err
    }

    task.Status = taskdomain.Status(status)
    return &task, nil
}

// scanTemplate сканирует строку в RecurrenceConfig
func scanTemplate(row pgx.Row) (*taskdomain.RecurrenceConfig, error) {
    var (
        template          taskdomain.RecurrenceConfig
        monthlyDaysJSON   []byte
        specificDatesJSON []byte
        dailyInterval     *int
        parityType        *string
        endDate           *time.Time
        executionTime     *time.Time
        templateID        int64
    )

    err := row.Scan(
        &templateID,
        &template.Title,
        &template.Description,
        &template.RecurrenceType,
        &dailyInterval,
        &monthlyDaysJSON,
        &specificDatesJSON,
        &parityType,
        &template.StartDate,
        &endDate,
        &executionTime,
        &template.Status,
        &template.CreatedAt,
        &template.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }

    template.ID = &templateID

    // Парсим nullable поля
    if dailyInterval != nil {
        template.DailyInterval = dailyInterval
    }

    if parityType != nil {
        pt := taskdomain.ParityType(*parityType)
        template.ParityType = &pt
    }

    template.EndDate = endDate
    template.ExecutionTime = executionTime

    // Парсим JSONB поля monthly_days
    if len(monthlyDaysJSON) > 0 && string(monthlyDaysJSON) != "null" {
        if err := json.Unmarshal(monthlyDaysJSON, &template.MonthlyDays); err != nil {
            return nil, err
        }
    }

    // Парсим JSONB поля specific_dates
    if len(specificDatesJSON) > 0 && string(specificDatesJSON) != "null" {
        var dateStrings []string
        if err := json.Unmarshal(specificDatesJSON, &dateStrings); err != nil {
            return nil, err
        }
        
        template.SpecificDates = make([]time.Time, 0, len(dateStrings))
        for _, ds := range dateStrings {
            parsedDate, err := time.Parse("2006-01-02", ds)
            if err != nil {
                return nil, err
            }
            template.SpecificDates = append(template.SpecificDates, parsedDate)
        }
    }

    return &template, nil
}