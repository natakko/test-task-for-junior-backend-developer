DeepSeek

Запрос:
Я работаю над тестовым заданием на Go 1.23. Мне нужно добавить функционал периодических задач.Сейчас у меня есть такая таблица в БД: CREATE TABLE IF NOT EXISTS tasks (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status);
Задача: добавить поддержку ежедневных (каждые N дней), ежемесячных, на конкретные даты и четных/нечетных дней.Вопрос: Предложи структуру новой таблицы task_templates для хранения этих правил. Как мне связать её с существующей таблицей tasks? Напиши SQL-код для новой миграции 0002_add_recurrence.up.sql.

Ответ:
Отличная задача! Для поддержки периодических задач нужно отделить шаблоны повторения от самих экземпляров задач. Вот оптимальное решение:

SQL-миграция 0002_add_recurrence.up.sql

-- Таблица шаблонов периодических задач
CREATE TABLE IF NOT EXISTS task_templates (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- Тип повторения: 'daily', 'monthly', 'specific_dates', 'parity'
    recurrence_type TEXT NOT NULL,
    -- Интервал в днях для ежедневных задач (каждые N дней)
    daily_interval INT DEFAULT NULL,
    -- Дни месяца для ежемесячных задач (JSON массив чисел: [1,15,30])
    monthly_days JSONB DEFAULT NULL,
    -- Конкретные абсолютные даты (JSON массив строк с датами)
    specific_dates JSONB DEFAULT NULL,
    -- Тип четности: 'even', 'odd', 'none' (для parity-задач)
    parity_type TEXT DEFAULT NULL,
    -- Дата первого выполнения
    start_date DATE NOT NULL,
    -- Опциональная дата окончания повторений
    end_date DATE DEFAULT NULL,
    -- Время выполнения (HH:MM:SS в UTC)
    execution_time TIME DEFAULT '00:00:00',
    -- Статус шаблона: 'active', 'paused', 'archived'
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Валидация типов
    CONSTRAINT valid_recurrence_type CHECK (recurrence_type IN ('daily', 'monthly', 'specific_dates', 'parity')),
    -- Валидация daily_interval
    CONSTRAINT valid_daily_interval CHECK (
        (recurrence_type = 'daily' AND daily_interval > 0) OR
        (recurrence_type != 'daily' AND daily_interval IS NULL)
    ),
    -- Валидация parity_type
    CONSTRAINT valid_parity CHECK (
        (recurrence_type = 'parity' AND parity_type IN ('even', 'odd')) OR
        (recurrence_type != 'parity' AND parity_type IS NULL)
    )
);

-- Таблица для связи шаблонов с конкретными экземплярами задач
CREATE TABLE IF NOT EXISTS task_instances (
    id BIGSERIAL PRIMARY KEY,
    template_id BIGINT NOT NULL REFERENCES task_templates(id) ON DELETE CASCADE,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    due_date DATE NOT NULL, -- Запланированная дата выполнения
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_cancelled BOOLEAN NOT NULL DEFAULT FALSE, -- Ручная отмена конкретного экземпляра
    
    UNIQUE(template_id, task_id, due_date),
    CONSTRAINT unique_template_due_date UNIQUE(template_id, due_date)
);

-- Индексы для быстрого поиска
CREATE INDEX IF NOT EXISTS idx_templates_status_recurrence ON task_templates(status, recurrence_type);
CREATE INDEX IF NOT EXISTS idx_templates_dates ON task_templates(start_date, end_date) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_instances_template_id ON task_instances(template_id);
CREATE INDEX IF NOT EXISTS idx_instances_task_id ON task_instances(task_id);
CREATE INDEX IF NOT EXISTS idx_instances_due_date ON task_instances(due_date) WHERE is_cancelled = false;

Связь с существующей таблицей
tasks - хранит конкретные экземпляры (созданные из шаблонов)
task_templates - хранит правила генерации
task_instances - связывает многие-ко-многим (один шаблон → много задач, одна задача → один шаблон)

Дополнительно: таблица для истории генерации

-- Опционально: журнал генерации (для отладки и аудита)
CREATE TABLE IF NOT EXISTS generation_log (
    id BIGSERIAL PRIMARY KEY,
    template_id BIGINT REFERENCES task_templates(id) ON DELETE CASCADE,
    generated_for_date DATE NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    task_id BIGINT REFERENCES tasks(id) ON DELETE SET NULL,
    status TEXT NOT NULL, -- 'success', 'skipped', 'failed'
    error_message TEXT,
    INDEX idx_gen_log_template_date (template_id, generated_for_date)
);

Преимущества этой схемы:
Нормализация - шаблоны не дублируются

Гибкость - можно менять правила, сохраняя историю задач

Производительность - быстрые запросы "какие задачи нужно сгенерировать на сегодня"

Отмена конкретных экземпляров - через is_cancelled или пропуск генерации

JSONB для сложных правил - легко хранить конкретные даты и дни месяца

Примеры правил в JSONB:

-- Ежемесячные (1 и 15 числа)
monthly_days: '[1, 15]'

-- Конкретные даты
specific_dates: '["2025-01-20", "2025-12-25"]'

Эта схема позволит легко генерировать задачи через планировщик, проверяя каждую ночь, какие экземпляры нужно создать на следующие N дней.

Запрос: 
Я работаю в файле internal/domain/task/recurrence.go. Пакет называется package task.На основе созданной таблицы task_templates, напиши:Структуру RecurrenceConfig со всеми полями (тип, интервал, дни месяца, конкретные даты, четность).Метод (r RecurrenceConfig) ShouldCreateTask(onDate time.Time) bool, который реализует логику проверки для всех типов периодичности.Учти, что пакет — task, и тебе понадобятся импорты time и, возможно, encoding/json (если данные из БД приходят в JSONB).

Ответ: 
Вот реализация для internal/domain/task/recurrence.go:
package task

import (
	"encoding/json"
	"time"
)

// RecurrenceType определяет тип периодичности
type RecurrenceType string

const (
	RecurrenceDaily         RecurrenceType = "daily"
	RecurrenceMonthly       RecurrenceType = "monthly"
	RecurrenceSpecificDates RecurrenceType = "specific_dates"
	RecurrenceParity        RecurrenceType = "parity"
)

// ParityType определяет тип четности
type ParityType string

const (
	ParityEven ParityType = "even"
	ParityOdd  ParityType = "odd"
)

// RecurrenceConfig представляет конфигурацию периодичности задачи
type RecurrenceConfig struct {
	// Тип повторения
	RecurrenceType RecurrenceType `json:"recurrence_type"`
	
	// Интервал в днях для ежедневных задач (каждые N дней)
	DailyInterval *int `json:"daily_interval,omitempty"`
	
	// Дни месяца для ежемесячных задач (числа от 1 до 31)
	MonthlyDays []int `json:"monthly_days,omitempty"`
	
	// Конкретные даты для absolute-типа
	SpecificDates []time.Time `json:"specific_dates,omitempty"`
	
	// Тип четности для parity-задач
	ParityType *ParityType `json:"parity_type,omitempty"`
	
	// Дата первого выполнения
	StartDate time.Time `json:"start_date"`
	
	// Опциональная дата окончания повторений
	EndDate *time.Time `json:"end_date,omitempty"`
	
	// Время выполнения (игнорируется при проверке даты, но хранится для создания задачи)
	ExecutionTime *time.Time `json:"execution_time,omitempty"`
}

// UnmarshalJSON кастомная десериализация для обработки JSONB из БД
func (r *RecurrenceConfig) UnmarshalJSON(data []byte) error {
	// Временная структура для десериализации с сырыми типами
	type Alias struct {
		RecurrenceType RecurrenceType `json:"recurrence_type"`
		DailyInterval  *int           `json:"daily_interval,omitempty"`
		MonthlyDays    json.RawMessage `json:"monthly_days,omitempty"`
		SpecificDates  json.RawMessage `json:"specific_dates,omitempty"`
		ParityType     *ParityType     `json:"parity_type,omitempty"`
		StartDate      time.Time       `json:"start_date"`
		EndDate        *time.Time      `json:"end_date,omitempty"`
		ExecutionTime  *time.Time      `json:"execution_time,omitempty"`
	}
	
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	
	r.RecurrenceType = alias.RecurrenceType
	r.DailyInterval = alias.DailyInterval
	r.ParityType = alias.ParityType
	r.StartDate = alias.StartDate
	r.EndDate = alias.EndDate
	r.ExecutionTime = alias.ExecutionTime
	
	// Обработка MonthlyDays (JSON массив)
	if len(alias.MonthlyDays) > 0 && string(alias.MonthlyDays) != "null" {
		var monthlyDays []int
		if err := json.Unmarshal(alias.MonthlyDays, &monthlyDays); err != nil {
			return err
		}
		r.MonthlyDays = monthlyDays
	}
	
	// Обработка SpecificDates (JSON массив строк с датами)
	if len(alias.SpecificDates) > 0 && string(alias.SpecificDates) != "null" {
		var dateStrings []string
		if err := json.Unmarshal(alias.SpecificDates, &dateStrings); err != nil {
			return err
		}
		
		r.SpecificDates = make([]time.Time, 0, len(dateStrings))
		for _, dateStr := range dateStrings {
			// Парсим дату в формате YYYY-MM-DD
			parsedDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return err
			}
			r.SpecificDates = append(r.SpecificDates, parsedDate)
		}
	}
	
	return nil
}

// ShouldCreateTask определяет, нужно ли создать задачу для указанной даты
func (r RecurrenceConfig) ShouldCreateTask(onDate time.Time) bool {
	// Нормализуем дату до начала дня (без времени)
	dateToCheck := time.Date(onDate.Year(), onDate.Month(), onDate.Day(), 0, 0, 0, 0, onDate.Location())
	
	// Проверяем границы дат
	if dateToCheck.Before(r.StartDate) {
		return false
	}
	
	if r.EndDate != nil {
		endDate := time.Date(r.EndDate.Year(), r.EndDate.Month(), r.EndDate.Day(), 0, 0, 0, 0, r.EndDate.Location())
		if dateToCheck.After(endDate) {
			return false
		}
	}
	
	// Проверяем в зависимости от типа
	switch r.RecurrenceType {
	case RecurrenceDaily:
		return r.shouldCreateDaily(dateToCheck)
		
	case RecurrenceMonthly:
		return r.shouldCreateMonthly(dateToCheck)
		
	case RecurrenceSpecificDates:
		return r.shouldCreateSpecificDates(dateToCheck)
		
	case RecurrenceParity:
		return r.shouldCreateParity(dateToCheck)
		
	default:
		return false
	}
}

// shouldCreateDaily проверяет ежедневную периодичность (каждые N дней)
func (r RecurrenceConfig) shouldCreateDaily(onDate time.Time) bool {
	if r.DailyInterval == nil || *r.DailyInterval <= 0 {
		return false
	}
	
	// Вычисляем разницу в днях от start_date
	startDate := time.Date(r.StartDate.Year(), r.StartDate.Month(), r.StartDate.Day(), 0, 0, 0, 0, r.StartDate.Location())
	daysDiff := int(onDate.Sub(startDate).Hours() / 24)
	
	// Проверяем, кратен ли интервалу
	return daysDiff%(*r.DailyInterval) == 0 && daysDiff >= 0
}

// shouldCreateMonthly проверяет ежемесячную периодичность (конкретные дни месяца)
func (r RecurrenceConfig) shouldCreateMonthly(onDate time.Time) bool {
	if len(r.MonthlyDays) == 0 {
		return false
	}
	
	dayOfMonth := onDate.Day()
	
	// Проверяем, входит ли день месяца в список
	for _, allowedDay := range r.MonthlyDays {
		if allowedDay == dayOfMonth {
			return true
		}
	}
	
	return false
}

// shouldCreateSpecificDates проверяет, входит ли дата в список конкретных дат
func (r RecurrenceConfig) shouldCreateSpecificDates(onDate time.Time) bool {
	for _, specificDate := range r.SpecificDates {
		if specificDate.Year() == onDate.Year() &&
			specificDate.Month() == onDate.Month() &&
			specificDate.Day() == onDate.Day() {
			return true
		}
	}
	return false
}

// shouldCreateParity проверяет четность дня
func (r RecurrenceConfig) shouldCreateParity(onDate time.Time) bool {
	if r.ParityType == nil {
		return false
	}
	
	dayOfMonth := onDate.Day()
	isEven := dayOfMonth%2 == 0
	
	switch *r.ParityType {
	case ParityEven:
		return isEven
	case ParityOdd:
		return !isEven
	default:
		return false
	}
}

// ParseRecurrenceFromDBRow вспомогательная функция для парсинга из строки БД
// Используется, если ты получаешь JSONB как string из драйвера
func ParseRecurrenceFromDBRow(jsonData string) (*RecurrenceConfig, error) {
	var config RecurrenceConfig
	if err := json.Unmarshal([]byte(jsonData), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

Пример использования:
package main

import (
	"fmt"
	"time"
	"your-project/internal/domain/task"
)

func main() {
	// Пример 1: Ежедневная задача каждые 3 дня
	dailyConfig := task.RecurrenceConfig{
		RecurrenceType: task.RecurrenceDaily,
		DailyInterval:  intPtr(3),
		StartDate:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	
	// Проверяем 4 января 2025
	date1 := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	fmt.Println(dailyConfig.ShouldCreateTask(date1)) // true (4-1 = 3 дня)
	
	// Пример 2: Ежемесячная задача 1 и 15 числа
	monthlyConfig := task.RecurrenceConfig{
		RecurrenceType: task.RecurrenceMonthly,
		MonthlyDays:    []int{1, 15},
		StartDate:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	
	date2 := time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC)
	fmt.Println(monthlyConfig.ShouldCreateTask(date2)) // true
	
	// Пример 3: Четные дни
	parityConfig := task.RecurrenceConfig{
		RecurrenceType: task.RecurrenceParity,
		ParityType:     task.ParityEvenPtr(),
		StartDate:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	
	date3 := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	fmt.Println(parityConfig.ShouldCreateTask(date3)) // true (4 - четное)
}

func intPtr(i int) *int { return &i }

Важно: Используй эту структуру при сканировании из БД, передавая JSONB поле как []byte или string в ParseRecurrenceFromDBRow.

Запрос: 
Логика shouldCreateMonthly сейчас просто проверяет совпадение чисел. Но если задача назначена на 31-е число, а в текущем месяце всего 30 дней (или 28 в феврале), задача будет пропущена. Перепиши метод shouldCreateMonthly так, чтобы: Если указанное число (например, 31) больше, чем количество дней в текущем месяце, задача создавалась в последний доступный день этого месяца. Добавь вспомогательную функцию lastDayOfMonth(t time.Time) int.

Ответ:
Отличное замечание! Вот исправленная реализация:
// lastDayOfMonth возвращает количество дней в месяце для указанной даты
func lastDayOfMonth(t time.Time) int {
    // Переходим к первому дню следующего месяца и отнимаем один день
    firstDayOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
    return firstDayOfNextMonth.AddDate(0, 0, -1).Day()
}

// shouldCreateMonthly проверяет ежемесячную периодичность с умной коррекцией
// для дат, которых нет в месяце (например, 31 число в феврале)
func (r RecurrenceConfig) shouldCreateMonthly(onDate time.Time) bool {
    if len(r.MonthlyDays) == 0 {
        return false
    }
    
    currentDay := onDate.Day()
    lastDay := lastDayOfMonth(onDate)
    
    for _, targetDay := range r.MonthlyDays {
        // Если целевой день существует в текущем месяце
        if targetDay <= lastDay {
            if currentDay == targetDay {
                return true
            }
        } else {
            // Если целевой день превышает количество дней в месяце,
            // проверяем, является ли текущий день последним днем месяца
            if currentDay == lastDay {
                return true
            }
        }
    }
    
    return false
}

Запрос:
Логика проверки готова. Теперь нужно обновить слой Repository.Вот мой текущий метод Create из internal/repository/postgres/task.go: package task

import "time"

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Task struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s Status) Valid() bool {
	switch s {
	case StatusNew, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}
 Задача: Обнови метод Create так, чтобы он поддерживал транзакцию (db.Begin).Если у задачи (task.Task) есть настройки периодичности, сначала нужно сохранить запись в таблицу task_templates, а затем создать саму задачу в таблице tasks.Не забудь добавить связь в таблицу task_instances (если мы решили её использовать).Напиши обновленный код метода и объясни, как лучше передавать настройки периодичности в структуру Task.

 Ответ:
 Отличная задача! Давай обновим структуру Task и реализуем метод Create с поддержкой транзакций.

 Обновленная структура Task
 package task

import (
    "database/sql"
    "encoding/json"
    "time"
)

type Status string

const (
    StatusNew        Status = "new"
    StatusInProgress Status = "in_progress"
    StatusDone       Status = "done"
)

// RecurrenceType и другие константы из recurrence.go
type RecurrenceType string

const (
    RecurrenceDaily         RecurrenceType = "daily"
    RecurrenceMonthly       RecurrenceType = "monthly"
    RecurrenceSpecificDates RecurrenceType = "specific_dates"
    RecurrenceParity        RecurrenceType = "parity"
)

type ParityType string

const (
    ParityEven ParityType = "even"
    ParityOdd  ParityType = "odd"
)

// RecurrenceConfig структура для хранения настроек периодичности
type RecurrenceConfig struct {
    ID             *int64          `json:"id,omitempty"`
    RecurrenceType RecurrenceType  `json:"recurrence_type"`
    DailyInterval  *int            `json:"daily_interval,omitempty"`
    MonthlyDays    []int           `json:"monthly_days,omitempty"`
    SpecificDates  []time.Time     `json:"specific_dates,omitempty"`
    ParityType     *ParityType     `json:"parity_type,omitempty"`
    StartDate      time.Time       `json:"start_date"`
    EndDate        *time.Time      `json:"end_date,omitempty"`
    ExecutionTime  *time.Time      `json:"execution_time,omitempty"`
    Status         string          `json:"status"` // active, paused, archived
    CreatedAt      time.Time       `json:"created_at"`
    UpdatedAt      time.Time       `json:"updated_at"`
}

// Task основная структура задачи
type Task struct {
    ID          int64             `json:"id"`
    Title       string            `json:"title"`
    Description string            `json:"description"`
    Status      Status            `json:"status"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
    
    // Опционально: настройки периодичности (если задача создана из шаблона)
    Recurrence  *RecurrenceConfig `json:"recurrence,omitempty"`
    TemplateID  *int64            `json:"template_id,omitempty"` // ID шаблона, если задача создана из шаблона
}

// TaskCreateRequest структура для создания задачи с возможной периодичностью
type TaskCreateRequest struct {
    Title       string            `json:"title"`
    Description string            `json:"description"`
    Status      Status            `json:"status"`
    Recurrence  *RecurrenceConfig `json:"recurrence,omitempty"`
}

func (s Status) Valid() bool {
    switch s {
    case StatusNew, StatusInProgress, StatusDone:
        return true
    default:
        return false
    }
}

Обновленный метод Create в репозитории
package postgres

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "time"
    
    "your-project/internal/domain/task"
    "github.com/lib/pq" // для работы с PostgreSQL массивами/JSONB
)

type TaskRepository struct {
    db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
    return &TaskRepository{db: db}
}

// Create создает новую задачу и опционально настройки периодичности
func (r *TaskRepository) Create(ctx context.Context, req *task.TaskCreateRequest) (*task.Task, error) {
    // Начинаем транзакцию
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback() // Откат в случае ошибки
    
    var templateID *int64
    var recurrenceConfig *task.RecurrenceConfig
    
    // Если есть настройки периодичности, сначала сохраняем шаблон
    if req.Recurrence != nil {
        templateID, recurrenceConfig, err = r.createTemplate(ctx, tx, req)
        if err != nil {
            return nil, fmt.Errorf("failed to create template: %w", err)
        }
    }
    
    // Создаем задачу в таблице tasks
    createdTask, err := r.createTask(ctx, tx, req, templateID)
    if err != nil {
        return nil, fmt.Errorf("failed to create task: %w", err)
    }
    
    // Если есть шаблон, создаем связь в task_instances
    if templateID != nil {
        if err := r.createTaskInstance(ctx, tx, *templateID, createdTask.ID, createdTask.CreatedAt); err != nil {
            return nil, fmt.Errorf("failed to create task instance: %w", err)
        }
        createdTask.Recurrence = recurrenceConfig
        createdTask.TemplateID = templateID
    }
    
    // Фиксируем транзакцию
    if err := tx.Commit(); err != nil {
        return nil, fmt.Errorf("failed to commit transaction: %w", err)
    }
    
    return createdTask, nil
}

// createTemplate создает запись в task_templates
func (r *TaskRepository) createTemplate(ctx context.Context, tx *sql.Tx, req *task.TaskCreateRequest) (*int64, *task.RecurrenceConfig, error) {
    cfg := req.Recurrence
    
    // Подготавливаем JSONB поля
    var monthlyDaysJSON, specificDatesJSON []byte
    var err error
    
    if len(cfg.MonthlyDays) > 0 {
        monthlyDaysJSON, err = json.Marshal(cfg.MonthlyDays)
        if err != nil {
            return nil, nil, fmt.Errorf("failed to marshal monthly_days: %w", err)
        }
    }
    
    if len(cfg.SpecificDates) > 0 {
        // Преобразуем time.Time в строки для JSON
        dateStrings := make([]string, len(cfg.SpecificDates))
        for i, d := range cfg.SpecificDates {
            dateStrings[i] = d.Format("2006-01-02")
        }
        specificDatesJSON, err = json.Marshal(dateStrings)
        if err != nil {
            return nil, nil, fmt.Errorf("failed to marshal specific_dates: %w", err)
        }
    }
    
    // Устанавливаем статус шаблона
    templateStatus := "active"
    if cfg.Status != "" {
        templateStatus = cfg.Status
    }
    
    // SQL запрос для вставки шаблона
    query := `
        INSERT INTO task_templates (
            title, description, recurrence_type, daily_interval, 
            monthly_days, specific_dates, parity_type, start_date, 
            end_date, execution_time, status, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        RETURNING id, created_at, updated_at
    `
    
    var templateID int64
    var createdAt, updatedAt time.Time
    
    err = tx.QueryRowContext(ctx, query,
        req.Title,
        req.Description,
        cfg.RecurrenceType,
        cfg.DailyInterval,
        monthlyDaysJSON,
        specificDatesJSON,
        cfg.ParityType,
        cfg.StartDate,
        cfg.EndDate,
        cfg.ExecutionTime,
        templateStatus,
        time.Now(),
        time.Now(),
    ).Scan(&templateID, &createdAt, &updatedAt)
    
    if err != nil {
        return nil, nil, fmt.Errorf("failed to insert template: %w", err)
    }
    
    // Обновляем конфиг с данными из БД
    cfg.ID = &templateID
    cfg.CreatedAt = createdAt
    cfg.UpdatedAt = updatedAt
    cfg.Status = templateStatus
    
    return &templateID, cfg, nil
}

// createTask создает задачу в таблице tasks
func (r *TaskRepository) createTask(ctx context.Context, tx *sql.Tx, req *task.TaskCreateRequest, templateID *int64) (*task.Task, error) {
    query := `
        INSERT INTO tasks (title, description, status, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, created_at, updated_at
    `
    
    now := time.Now()
    var taskID int64
    var createdAt, updatedAt time.Time
    
    err := tx.QueryRowContext(ctx, query,
        req.Title,
        req.Description,
        req.Status,
        now,
        now,
    ).Scan(&taskID, &createdAt, &updatedAt)
    
    if err != nil {
        return nil, fmt.Errorf("failed to insert task: %w", err)
    }
    
    return &task.Task{
        ID:          taskID,
        Title:       req.Title,
        Description: req.Description,
        Status:      req.Status,
        CreatedAt:   createdAt,
        UpdatedAt:   updatedAt,
        TemplateID:  templateID,
    }, nil
}

// createTaskInstance создает связь между шаблоном и задачей
func (r *TaskRepository) createTaskInstance(ctx context.Context, tx *sql.Tx, templateID, taskID int64, dueDate time.Time) error {
    query := `
        INSERT INTO task_instances (template_id, task_id, due_date, generated_at, is_cancelled)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (template_id, due_date) DO NOTHING
    `
    
    _, err := tx.ExecContext(ctx, query,
        templateID,
        taskID,
        dueDate,
        time.Now(),
        false,
    )
    
    if err != nil {
        return fmt.Errorf("failed to create task instance: %w", err)
    }
    
    return nil
}

Вспомогательные методы для работы с периодичностью
// GetTasksForDate получает все задачи (включая сгенерированные из шаблонов) для указанной даты
func (r *TaskRepository) GetTasksForDate(ctx context.Context, date time.Time) ([]*task.Task, error) {
    // 1. Получаем все активные шаблоны
    templates, err := r.getActiveTemplates(ctx)
    if err != nil {
        return nil, err
    }
    
    // 2. Для каждого шаблона проверяем, нужно ли создать задачу
    var tasks []*task.Task
    
    for _, tmpl := range templates {
        if tmpl.ShouldCreateTask(date) {
            // Проверяем, не создана ли уже задача на эту дату
            exists, err := r.taskInstanceExists(ctx, tmpl.ID, date)
            if err != nil {
                return nil, err
            }
            
            if !exists {
                // Создаем новую задачу из шаблона
                newTask, err := r.createTaskFromTemplate(ctx, tmpl, date)
                if err != nil {
                    return nil, err
                }
                tasks = append(tasks, newTask)
            }
        }
    }
    
    return tasks, nil
}

// getActiveTemplates получает все активные шаблоны
func (r *TaskRepository) getActiveTemplates(ctx context.Context) ([]*task.RecurrenceConfig, error) {
    query := `
        SELECT id, title, description, recurrence_type, daily_interval, 
               monthly_days, specific_dates, parity_type, start_date, 
               end_date, execution_time, status, created_at, updated_at
        FROM task_templates
        WHERE status = 'active'
    `
    
    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var templates []*task.RecurrenceConfig
    for rows.Next() {
        var tmpl task.RecurrenceConfig
        var monthlyDaysJSON, specificDatesJSON []byte
        var dailyInterval, parityType sql.NullString
        
        err := rows.Scan(
            &tmpl.ID, &tmpl.Title, &tmpl.Description, &tmpl.RecurrenceType,
            &tmpl.DailyInterval, &monthlyDaysJSON, &specificDatesJSON, &parityType,
            &tmpl.StartDate, &tmpl.EndDate, &tmpl.ExecutionTime,
            &tmpl.Status, &tmpl.CreatedAt, &tmpl.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        
        // Парсим JSONB поля
        if len(monthlyDaysJSON) > 0 {
            json.Unmarshal(monthlyDaysJSON, &tmpl.MonthlyDays)
        }
        if len(specificDatesJSON) > 0 {
            var dateStrings []string
            json.Unmarshal(specificDatesJSON, &dateStrings)
            for _, ds := range dateStrings {
                d, _ := time.Parse("2006-01-02", ds)
                tmpl.SpecificDates = append(tmpl.SpecificDates, d)
            }
        }
        if parityType.Valid {
            pt := task.ParityType(parityType.String)
            tmpl.ParityType = &pt
        }
        
        templates = append(templates, &tmpl)
    }
    
    return templates, nil
}

// taskInstanceExists проверяет, существует ли уже экземпляр задачи для шаблона на указанную дату
func (r *TaskRepository) taskInstanceExists(ctx context.Context, templateID *int64, date time.Time) (bool, error) {
    query := `
        SELECT EXISTS(
            SELECT 1 FROM task_instances 
            WHERE template_id = $1 AND due_date = $2
        )
    `
    var exists bool
    err := r.db.QueryRowContext(ctx, query, templateID, date).Scan(&exists)
    return exists, err
}

// createTaskFromTemplate создает задачу на основе шаблона
func (r *TaskRepository) createTaskFromTemplate(ctx context.Context, template *task.RecurrenceConfig, dueDate time.Time) (*task.Task, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback()
    
    // Создаем задачу
    taskReq := &task.TaskCreateRequest{
        Title:       template.Title,
        Description: template.Description,
        Status:      task.StatusNew,
    }
    
    newTask, err := r.createTask(ctx, tx, taskReq, template.ID)
    if err != nil {
        return nil, err
    }
    
    // Создаем связь
    err = r.createTaskInstance(ctx, tx, *template.ID, newTask.ID, dueDate)
    if err != nil {
        return nil, err
    }
    
    if err := tx.Commit(); err != nil {
        return nil, err
    }
    
    return newTask, nil
}

Ключевые моменты:
Транзакционность: Все операции либо успешны, либо полностью откатываются

Гибкость: Можно создавать как обычные задачи, так и задачи с периодичностью

Связность: Через task_instances отслеживается, какие задачи из каких шаблонов созданы

Безопасность: Используются подготовленные запросы и контекст

Использование:
// Создание обычной задачи
req := &task.TaskCreateRequest{
    Title:       "Обычная задача",
    Description: "Без повторений",
    Status:      task.StatusNew,
}
task, err := repo.Create(ctx, req)

// Создание задачи с ежемесячным повторением
monthlyReq := &task.TaskCreateRequest{
    Title:       "Ежемесячный отчет",
    Description: "Сдавать отчет каждый месяц 31 числа",
    Status:      task.StatusNew,
    Recurrence: &task.RecurrenceConfig{
        RecurrenceType: task.RecurrenceMonthly,
        MonthlyDays:    []int{31},
        StartDate:      time.Now(),
        Status:         "active",
    },
}
monthlyTask, err := repo.Create(ctx, monthlyReq)

Запрос: 
У меня есть файл task_repository.go, использующий pgxpool.Pool. Мне нужно обновить метод Create, чтобы он поддерживал периодичность задач. Вот мой текущий код репозитория: package postgres

import (
	"context"
	"errors"

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

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (*taskdomain.Task, error) {
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
 Что нужно сделать: Обнови метод Create. Если у task.Recurrence не nil, нужно: Начать транзакцию через r.pool.Begin(ctx).Сохранить настройки в таблицу task_templates (используй json.Marshal для полей JSONB).Сохранить саму задачу в tasks.Создать запись в task_instances.Завершить транзакцию (Commit).Добавь новый метод ListActiveTemplates(ctx context.Context) ([]taskdomain.RecurrenceConfig, error) — он понадобится для воркера. Используй псевдоним taskdomain, как в моем коде. Учти, что pgx возвращает ошибку err из Begin, и транзакция закрывается через defer tx.Rollback(ctx).

 Ответ: 
 Вот обновленный код репозитория с учетом pgxpool.Pool и taskdomain псевдонима:

 package postgres

import (
	"context"
	"encoding/json"
	"errors"
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

// Create создает новую задачу и опционально настройки периодичности
func (r *Repository) Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	// Если нет периодичности, используем простую вставку без транзакции
	if task.Recurrence == nil {
		return r.createSimpleTask(ctx, task)
	}

	// Если есть периодичность, используем транзакцию
	return r.createTaskWithRecurrence(ctx, task)
}

// createSimpleTask создает задачу без периодичности
func (r *Repository) createSimpleTask(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
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

// createTaskWithRecurrence создает задачу с периодичностью в транзакции
func (r *Repository) createTaskWithRecurrence(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error) {
	// Начинаем транзакцию
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Сохраняем шаблон периодичности
	templateID, err := r.createTemplate(ctx, tx, task)
	if err != nil {
		return nil, err
	}

	// 2. Сохраняем задачу
	createdTask, err := r.createTaskInTx(ctx, tx, task)
	if err != nil {
		return nil, err
	}

	// 3. Создаем связь в task_instances
	err = r.createTaskInstance(ctx, tx, templateID, createdTask.ID, createdTask.CreatedAt)
	if err != nil {
		return nil, err
	}

	// 4. Фиксируем транзакцию
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Обновляем поля задачи с данными из БД
	createdTask.Recurrence = task.Recurrence
	createdTask.TemplateID = &templateID

	return createdTask, nil
}

// createTemplate создает запись в task_templates
func (r *Repository) createTemplate(ctx context.Context, tx pgx.Tx, task *taskdomain.Task) (int64, error) {
	cfg := task.Recurrence

	// Подготавливаем JSONB поля
	var monthlyDaysJSON, specificDatesJSON []byte
	var err error

	if len(cfg.MonthlyDays) > 0 {
		monthlyDaysJSON, err = json.Marshal(cfg.MonthlyDays)
		if err != nil {
			return 0, err
		}
	}

	if len(cfg.SpecificDates) > 0 {
		// Преобразуем time.Time в строки для JSON
		dateStrings := make([]string, len(cfg.SpecificDates))
		for i, d := range cfg.SpecificDates {
			dateStrings[i] = d.Format("2006-01-02")
		}
		specificDatesJSON, err = json.Marshal(dateStrings)
		if err != nil {
			return 0, err
		}
	}

	// Устанавливаем статус шаблона
	templateStatus := "active"
	if cfg.Status != "" {
		templateStatus = cfg.Status
	}

	now := time.Now()

	query := `
		INSERT INTO task_templates (
			title, description, recurrence_type, daily_interval, 
			monthly_days, specific_dates, parity_type, start_date, 
			end_date, execution_time, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`

	var templateID int64
	err = tx.QueryRow(ctx, query,
		task.Title,
		task.Description,
		cfg.RecurrenceType,
		cfg.DailyInterval,
		monthlyDaysJSON,
		specificDatesJSON,
		cfg.ParityType,
		cfg.StartDate,
		cfg.EndDate,
		cfg.ExecutionTime,
		templateStatus,
		now,
		now,
	).Scan(&templateID)

	if err != nil {
		return 0, err
	}

	return templateID, nil
}

// createTaskInTx создает задачу в рамках транзакции
func (r *Repository) createTaskInTx(ctx context.Context, tx pgx.Tx, task *taskdomain.Task) (*taskdomain.Task, error) {
	const query = `
		INSERT INTO tasks (title, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, title, description, status, created_at, updated_at
	`

	row := tx.QueryRow(ctx, query, task.Title, task.Description, task.Status, task.CreatedAt, task.UpdatedAt)
	created, err := scanTask(row)
	if err != nil {
		return nil, err
	}

	return created, nil
}

// createTaskInstance создает связь между шаблоном и задачей
func (r *Repository) createTaskInstance(ctx context.Context, tx pgx.Tx, templateID, taskID int64, dueDate time.Time) error {
	query := `
		INSERT INTO task_instances (template_id, task_id, due_date, generated_at, is_cancelled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (template_id, due_date) DO NOTHING
	`

	_, err := tx.Exec(ctx, query,
		templateID,
		taskID,
		dueDate,
		time.Now(),
		false,
	)

	return err
}

// ListActiveTemplates возвращает все активные шаблоны задач
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
		return nil, err
	}
	defer rows.Close()

	var templates []taskdomain.RecurrenceConfig
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *template)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return templates, nil
}

// GetTemplatesForDate возвращает шаблоны, которые должны создать задачу на указанную дату
func (r *Repository) GetTemplatesForDate(ctx context.Context, date time.Time) ([]taskdomain.RecurrenceConfig, error) {
	const query = `
		SELECT id, title, description, recurrence_type, daily_interval, 
		       monthly_days, specific_dates, parity_type, start_date, 
		       end_date, execution_time, status, created_at, updated_at
		FROM task_templates
		WHERE status = 'active'
		AND start_date <= $1
		AND (end_date IS NULL OR end_date >= $1)
	`

	rows, err := r.pool.Query(ctx, query, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []taskdomain.RecurrenceConfig
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *template)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return templates, nil
}

// TemplateExistsForDate проверяет, был ли уже создан экземпляр задачи для шаблона на указанную дату
func (r *Repository) TemplateExistsForDate(ctx context.Context, templateID int64, date time.Time) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM task_instances 
			WHERE template_id = $1 AND due_date = $2 AND is_cancelled = false
		)
	`

	var exists bool
	err := r.pool.QueryRow(ctx, query, templateID, date).Scan(&exists)
	return exists, err
}

// scanTemplate сканирует строку из БД в RecurrenceConfig
func scanTemplate(row pgx.Row) (*taskdomain.RecurrenceConfig, error) {
	var (
		template           taskdomain.RecurrenceConfig
		monthlyDaysJSON    []byte
		specificDatesJSON  []byte
		dailyInterval      *int
		parityType         *string
		endDate            *time.Time
		executionTime      *time.Time
	)

	err := row.Scan(
		&template.ID,
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

	// Парсим JSONB поля
	if len(monthlyDaysJSON) > 0 && string(monthlyDaysJSON) != "null" {
		if err := json.Unmarshal(monthlyDaysJSON, &template.MonthlyDays); err != nil {
			return nil, err
		}
	}

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

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (*taskdomain.Task, error) {
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

Ключевые изменения:
Разделение логики: CreateSimpleTask для задач без периодичности, CreateTaskWithRecurrence для задач с периодичностью.

Транзакции с pgx: Используем tx.Commit(ctx) и tx.Rollback(ctx) с передачей контекста.

Новые методы:

ListActiveTemplates() - возвращает все активные шаблоны

GetTemplatesForDate(date) - возвращает шаблоны для конкретной даты

TemplateExistsForDate() - проверяет, создана ли уже задача

ScanTemplate: отдельная функция для сканирования шаблонов из БД с правильной обработкой JSONB и nullable полей.

Обработка ошибок: корректная обработка pgx.ErrNoRows и других ошибок.

Использование в воркере:
// Пример воркера для генерации задач
func (w *Worker) GenerateTasksForDate(ctx context.Context, date time.Time) error {
    templates, err := w.repo.GetTemplatesForDate(ctx, date)
    if err != nil {
        return err
    }
    
    for _, template := range templates {
        if template.ShouldCreateTask(date) {
            exists, err := w.repo.TemplateExistsForDate(ctx, *template.ID, date)
            if err != nil {
                return err
            }
            
            if !exists {
                // Создаем задачу из шаблона
                newTask := &taskdomain.Task{
                    Title:       template.Title,
                    Description: template.Description,
                    Status:      taskdomain.StatusNew,
                    CreatedAt:   time.Now(),
                    UpdatedAt:   time.Now(),
                }
                
                _, err := w.repo.Create(ctx, newTask)
                if err != nil {
                    return err
                }
            }
        }
    }
    
    return nil
}

Запрос: 
Допиши для моего репозитория task_repository.go на pgxpool два метода: ListActiveTemplates(ctx context.Context) ([]taskdomain.RecurrenceConfig, error) — возвращает все шаблоны из task_templates со статусом 'active'. TemplateExistsForDate(ctx context.Context, templateID int64, date time.Time) (bool, error) — проверяет в таблице task_instances, создавалась ли уже задача для этого шаблона на эту дату.

Ответ:
Вот дополненные методы для вашего task_repository.go:

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
		return nil, err
	}
	defer rows.Close()

	var templates []taskdomain.RecurrenceConfig
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *template)
	}

	if err := rows.Err(); err != nil {
		return nil, err
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
		return false, err
	}

	return exists, nil
}

// scanTemplate вспомогательная функция для сканирования строки в RecurrenceConfig
func scanTemplate(row pgx.Row) (*taskdomain.RecurrenceConfig, error) {
	var (
		template          taskdomain.RecurrenceConfig
		monthlyDaysJSON   []byte
		specificDatesJSON []byte
		dailyInterval     *int
		parityType        *string
		endDate           *time.Time
		executionTime     *time.Time
	)

	err := row.Scan(
		&template.ID,
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


