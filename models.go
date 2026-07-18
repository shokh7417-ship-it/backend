package main

import "time"

// User — profile plus the reusable availability that skills should not re-ask.
type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Timezone     string    `json:"timezone"`
	HoursPerWeek int       `json:"hoursPerWeek"`
	Days         []string  `json:"days"`
	CreatedAt    time.Time `json:"createdAt"`
}

// FrameworkAnswers — the universal intake categories, specialized per skill.
type FrameworkAnswers struct {
	CurrentLevel  string   `json:"currentLevel"`
	Target        string   `json:"target"`
	Deadline      string   `json:"deadline"` // YYYY-MM-DD or free text
	HoursPerWeek  int      `json:"hoursPerWeek"`
	Days          []string `json:"days"`
	Budget        string   `json:"budget"`
	Location      string   `json:"location"`
	LearningStyle string   `json:"learningStyle"`
	Motivation    string   `json:"motivation"`
	PivotalChoice string   `json:"pivotalChoice"` // e.g. Academic vs General
}

type Goal struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	RawInput  string    `json:"rawInput"`
	Skill     string    `json:"skill"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

type Message struct {
	Role    string    `json:"role"` // user | assistant
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// IntakeSession — the running state machine for one goal conversation.
type IntakeSession struct {
	ID                  string           `json:"id"`
	UserID              string           `json:"userId"`
	GoalID              string           `json:"goalId"`
	Stage               string           `json:"stage"` // scope_check|disambiguation|intake|plan_ready|out_of_scope
	Lang                string           `json:"lang"`  // en|ru|uz
	Skill               string           `json:"skill"`
	Path                string           `json:"path"`
	Overview            string           `json:"overview"`
	PivotalChoice       string           `json:"pivotalChoice"`
	NeedsDisambiguation bool             `json:"needsDisambiguation"`
	Messages            []Message        `json:"messages"`
	Answers             FrameworkAnswers `json:"answers"`
	AnswerBag           map[string]string `json:"answerBag"` // raw answers keyed by category
	AskedCount          int              `json:"askedCount"`
	PlanID              string           `json:"planId"`
	CreatedAt           time.Time        `json:"createdAt"`
	UpdatedAt           time.Time        `json:"updatedAt"`
}

type Todo struct {
	ID          string     `json:"id"`
	PlanID      string     `json:"planId"`
	Title       string     `json:"title"`
	DurationMin int        `json:"durationMin"`
	Frequency   string     `json:"frequency"` // once|weekly|twice_weekly|thrice_weekly|daily
	Priority    string     `json:"priority"`  // high|medium|low
	Phase       string     `json:"phase"`
	DependsOn   []string   `json:"dependsOn"`
	ResourceRef string     `json:"resourceRef"`
	Status      string     `json:"status"` // pending|done|skipped
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type Milestone struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Phase      string `json:"phase"`
	TargetDate string `json:"targetDate"`
	Done       bool   `json:"done"`
}

type Phase struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	WeekStart int    `json:"weekStart"`
	WeekEnd   int    `json:"weekEnd"`
}

type SetupItem struct {
	ID         string `json:"id"`
	PlanID     string `json:"planId"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Priority   string `json:"priority"`
	PriceRange string `json:"priceRange"`
	Owned      bool   `json:"owned"`
	Rationale  string `json:"rationale"`
}

// Plan — versioned; phases + milestones + assessment, with sized todos underneath.
type Plan struct {
	ID                 string      `json:"id"`
	UserID             string      `json:"userId"`
	GoalID             string      `json:"goalId"`
	Skill              string      `json:"skill"`
	Path               string      `json:"path"`
	Assessment         string      `json:"assessment"`
	Feasibility        string      `json:"feasibility"`
	Phases             []Phase     `json:"phases"`
	Milestones         []Milestone `json:"milestones"`
	Todos              []Todo      `json:"todos"`
	SetupItems         []SetupItem `json:"setupItems"`
	HoursPerWeek       int         `json:"hoursPerWeek"`
	Days               []string    `json:"days"`
	WeeksTotal         int         `json:"weeksTotal"`
	StartDate          string      `json:"startDate"`
	FinishDate         string      `json:"finishDate"`
	OriginalFinishDate string      `json:"originalFinishDate"`
	Version            int         `json:"version"`
	CreatedAt          time.Time   `json:"createdAt"`
	UpdatedAt          time.Time   `json:"updatedAt"`
}

// CalendarEvent — a scheduled todo instance on start.ai's own web calendar.
type CalendarEvent struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	PlanID       string `json:"planId"`
	TodoID       string `json:"todoId"`
	Title        string `json:"title"`
	Date         string `json:"date"` // YYYY-MM-DD
	StartTime    string `json:"startTime"`
	DurationMin  int    `json:"durationMin"`
	ReminderMin  int    `json:"reminderMin"`
	Status       string `json:"status"`       // proposed|scheduled|done|rolled_over
	ExportTarget string `json:"exportTarget"` // web_calendar (app version would be device_calendar)
}

type ProgressLog struct {
	ID     string    `json:"id"`
	UserID string    `json:"userId"`
	PlanID string    `json:"planId"`
	TodoID string    `json:"todoId"`
	Event  string    `json:"event"` // completed|skipped|rolled_over|milestone_hit
	At     time.Time `json:"at"`
	Note   string    `json:"note"`
}
