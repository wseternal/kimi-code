// Package cron implements a cron scheduling system for recurring agent tasks.
// It supports standard 5-field cron expressions, task persistence to disk,
// and provides create/delete/list tools for model invocation.
package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScheduledTask represents a cron-scheduled agent task.
type ScheduledTask struct {
	ID          string    `json:"id"`
	Expression  string    `json:"expression"`
	Prompt      string    `json:"prompt"`
	Model       string    `json:"model,omitempty"`
	WorkDir     string    `json:"work_dir,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	NextRunAt   time.Time `json:"next_run_at"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	RunCount    int       `json:"run_count"`
	Enabled     bool      `json:"enabled"`
}

// CronManager manages scheduled tasks.
type CronManager struct {
	mu       sync.RWMutex
	tasks    map[string]*ScheduledTask
	store    *Store
	nextID   int
	onFire   func(task *ScheduledTask)
}

// NewCronManager creates a cron manager with optional persistence and fire callback.
func NewCronManager(store *Store, onFire func(*ScheduledTask)) *CronManager {
	m := &CronManager{
		tasks:  make(map[string]*ScheduledTask),
		store:  store,
		onFire: onFire,
	}
	if store != nil {
		tasks, err := store.Load()
		if err == nil {
			for _, t := range tasks {
				m.tasks[t.ID] = t
				if id, err := strconv.Atoi(strings.TrimPrefix(t.ID, "cron_")); err == nil && id > m.nextID {
					m.nextID = id
				}
			}
		}
	}
	return m
}

// Create adds a new scheduled task.
func (m *CronManager) Create(expression, prompt, model, workDir string) (*ScheduledTask, error) {
	if _, err := ParseCron(expression); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	m.mu.Lock()
	m.nextID++
	id := fmt.Sprintf("cron_%d", m.nextID)
	task := &ScheduledTask{
		ID:         id,
		Expression: expression,
		Prompt:     prompt,
		Model:      model,
		WorkDir:    workDir,
		CreatedAt:  time.Now(),
		Enabled:    true,
	}
	m.tasks[id] = task
	m.computeNextRun(task)
	m.mu.Unlock()

	m.persist()

	return task, nil
}

// Delete removes a scheduled task.
func (m *CronManager) Delete(id string) bool {
	m.mu.Lock()
	_, ok := m.tasks[id]
	if ok {
		delete(m.tasks, id)
	}
	m.mu.Unlock()
	if ok {
		m.persist()
	}
	return ok
}

// Get returns a task by ID.
func (m *CronManager) Get(id string) (*ScheduledTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	copy := *t
	return &copy, true
}

// List returns all scheduled tasks.
func (m *CronManager) List() []*ScheduledTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*ScheduledTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		copy := *t
		result = append(result, &copy)
	}
	return result
}

// Toggle enables or disables a task.
func (m *CronManager) Toggle(id string, enabled bool) bool {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if ok {
		t.Enabled = enabled
	}
	m.mu.Unlock()
	if ok {
		m.persist()
	}
	return ok
}

// Tick checks for tasks that should fire and triggers them.
// Call this periodically (e.g., every minute).
func (m *CronManager) Tick(now time.Time) []string {
	m.mu.RLock()
	var fired []string
	for _, t := range m.tasks {
		if !t.Enabled {
			continue
		}
		if !t.NextRunAt.IsZero() && !now.Before(t.NextRunAt) {
			fired = append(fired, t.ID)
		}
	}
	m.mu.RUnlock()

	for _, id := range fired {
		m.fireTask(id, now)
	}

	return fired
}

func (m *CronManager) fireTask(id string, now time.Time) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	t.LastRunAt = now
	t.RunCount++
	m.computeNextRun(t)
	task := *t
	m.mu.Unlock()

	m.persist()

	if m.onFire != nil {
		m.onFire(&task)
	}
}

func (m *CronManager) computeNextRun(task *ScheduledTask) {
	sched, err := ParseCron(task.Expression)
	if err != nil {
		return
	}
	now := time.Now()
	if !task.NextRunAt.IsZero() && task.NextRunAt.After(now) {
		now = task.NextRunAt
	}
	task.NextRunAt = sched.Next(now)
}

func (m *CronManager) persist() {
	if m.store == nil {
		return
	}
	m.mu.RLock()
	tasks := make([]*ScheduledTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		copy := *t
		tasks = append(tasks, &copy)
	}
	m.mu.RUnlock()
	_ = m.store.Save(tasks)
}

// ── Store ──

// Store persists scheduled tasks to disk.
type Store struct {
	dir string
}

// NewStore creates a cron store.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) path() string {
	return filepath.Join(s.dir, "cron-tasks.json")
}

// Save persists tasks.
func (s *Store) Save(tasks []*ScheduledTask) error {
	if s.dir == "" {
		return nil
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(s.path(), data, 0644)
}

// Load reads tasks from disk.
func (s *Store) Load() ([]*ScheduledTask, error) {
	if s.dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []*ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// ── Cron Expression Parser ──

// Schedule represents a parsed 5-field cron expression.
type Schedule struct {
	Minutes    []int // 0-59
	Hours      []int // 0-23
	DaysOfMonth []int // 1-31
	Months     []int // 1-12
	DaysOfWeek []int // 0-6 (0=Sunday)
}

// ParseCron parses a standard 5-field cron expression.
func ParseCron(expr string) (*Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day of month: %w", err)
	}
	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day of week: %w", err)
	}

	return &Schedule{
		Minutes:     minutes,
		Hours:       hours,
		DaysOfMonth: dom,
		Months:      months,
		DaysOfWeek:  dow,
	}, nil
}

// Next returns the next time after 'from' that matches the schedule.
func (s *Schedule) Next(from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)

	for i := 0; i < 366*24*60; i++ { // max ~1 year of minutes
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{} // no match found
}

func (s *Schedule) matches(t time.Time) bool {
	return contains(s.Minutes, t.Minute()) &&
		contains(s.Hours, t.Hour()) &&
		contains(s.DaysOfMonth, t.Day()) &&
		contains(s.Months, int(t.Month())) &&
		contains(s.DaysOfWeek, int(t.Weekday()))
}

func contains(vals []int, v int) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}

// parseField parses a single cron field (e.g., "*/5", "1-3", "1,2,3", "*").
func parseField(field string, min, max int) ([]int, error) {
	if field == "*" {
		return makeRange(min, max), nil
	}

	var result []int
	for _, part := range strings.Split(field, ",") {
		if strings.Contains(part, "/") {
			// Step: */5 or 1-10/2
			parts := strings.SplitN(part, "/", 2)
			step, err := strconv.Atoi(parts[1])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step: %s", part)
			}
			var base []int
			if parts[0] == "*" {
				base = makeRange(min, max)
			} else if strings.Contains(parts[0], "-") {
				base, err = parseRange(parts[0])
				if err != nil {
					return nil, err
				}
			} else {
				start, err := strconv.Atoi(parts[0])
				if err != nil {
					return nil, fmt.Errorf("invalid value: %s", part)
				}
				base = makeRange(start, max)
			}
			for i, v := range base {
				if i%step == 0 {
					result = append(result, v)
				}
			}
		} else if strings.Contains(part, "-") {
			r, err := parseRange(part)
			if err != nil {
				return nil, err
			}
			result = append(result, r...)
		} else {
			v, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			if v < min || v > max {
				return nil, fmt.Errorf("value %d out of range [%d-%d]", v, min, max)
			}
			result = append(result, v)
		}
	}
	return result, nil
}

func parseRange(s string) ([]int, error) {
	parts := strings.SplitN(s, "-", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid range: %s", s)
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid range: %s", s)
	}
	if start > end {
		return nil, fmt.Errorf("invalid range: %d-%d", start, end)
	}
	return makeRange(start, end), nil
}

func makeRange(min, max int) []int {
	r := make([]int, 0, max-min+1)
	for i := min; i <= max; i++ {
		r = append(r, i)
	}
	return r
}
