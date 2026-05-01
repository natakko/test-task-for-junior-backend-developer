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