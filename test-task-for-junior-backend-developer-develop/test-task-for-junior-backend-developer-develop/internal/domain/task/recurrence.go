package task

import "time"

// RecurrenceType тип периодичности
type RecurrenceType string

const (
	RecurrenceDaily         RecurrenceType = "daily"
	RecurrenceMonthly       RecurrenceType = "monthly"
	RecurrenceSpecificDates RecurrenceType = "specific_dates"
	RecurrenceParity        RecurrenceType = "parity"
)

// ParityType тип четности
type ParityType string

const (
	ParityEven ParityType = "even"
	ParityOdd  ParityType = "odd"
)

// RecurrenceConfig настройки периодичности задачи
type RecurrenceConfig struct {
	ID             *int64          `json:"id,omitempty"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	RecurrenceType RecurrenceType  `json:"recurrence_type"`
	DailyInterval  *int            `json:"daily_interval,omitempty"`
	MonthlyDays    []int           `json:"monthly_days,omitempty"`
	SpecificDates  []time.Time     `json:"specific_dates,omitempty"`
	ParityType     *ParityType     `json:"parity_type,omitempty"`
	StartDate      time.Time       `json:"start_date"`
	EndDate        *time.Time      `json:"end_date,omitempty"`
	ExecutionTime  *time.Time      `json:"execution_time,omitempty"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ShouldCreateTask определяет, нужно ли создать задачу для указанной даты
func (r RecurrenceConfig) ShouldCreateTask(onDate time.Time) bool {
	dateToCheck := time.Date(onDate.Year(), onDate.Month(), onDate.Day(), 0, 0, 0, 0, onDate.Location())

	if dateToCheck.Before(r.StartDate) {
		return false
	}

	if r.EndDate != nil {
		endDate := time.Date(r.EndDate.Year(), r.EndDate.Month(), r.EndDate.Day(), 0, 0, 0, 0, r.EndDate.Location())
		if dateToCheck.After(endDate) {
			return false
		}
	}

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

func (r RecurrenceConfig) shouldCreateDaily(onDate time.Time) bool {
	if r.DailyInterval == nil || *r.DailyInterval <= 0 {
		return false
	}

	startDate := time.Date(r.StartDate.Year(), r.StartDate.Month(), r.StartDate.Day(), 0, 0, 0, 0, r.StartDate.Location())
	daysDiff := int(onDate.Sub(startDate).Hours() / 24)

	return daysDiff%(*r.DailyInterval) == 0 && daysDiff >= 0
}

func (r RecurrenceConfig) shouldCreateMonthly(onDate time.Time) bool {
	if len(r.MonthlyDays) == 0 {
		return false
	}

	currentDay := onDate.Day()
	lastDay := lastDayOfMonth(onDate)

	for _, targetDay := range r.MonthlyDays {
		effectiveDay := targetDay
		if effectiveDay > lastDay {
			effectiveDay = lastDay
		}

		if currentDay == effectiveDay {
			return true
		}
	}

	return false
}

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

func lastDayOfMonth(t time.Time) int {
	firstDayOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
	return firstDayOfNextMonth.AddDate(0, 0, -1).Day()
}