package plan

import (
	"sync"
	"testing"
)

func TestTracker_SetTasks(t *testing.T) {
	tr := NewTracker()
	tasks := []Task{
		{Title: "Task A", Status: StatusPending},
		{Title: "Task B", Status: StatusActive},
		{Title: "Task C", Status: StatusDone},
	}
	tr.SetTasks(tasks)

	got := tr.Tasks()
	if len(got) != 3 {
		t.Fatalf("Len() = %d, want 3", len(got))
	}
	if got[0].Title != "Task A" || got[0].Status != StatusPending {
		t.Errorf("got[0] = %+v", got[0])
	}
}

func TestTracker_UpdateTask(t *testing.T) {
	tr := NewTracker()
	tr.SetTasks([]Task{{Title: "A", Status: StatusPending}})

	tr.UpdateTask("A", StatusDone)
	tasks := tr.Tasks()
	if tasks[0].Status != StatusDone {
		t.Errorf("expected StatusDone, got %s", tasks[0].Status)
	}

	// Update non-existent title appends
	tr.UpdateTask("B", StatusActive)
	if tr.Len() != 2 {
		t.Errorf("Len() = %d, want 2", tr.Len())
	}
}

func TestTracker_Clear(t *testing.T) {
	tr := NewTracker()
	tr.SetTasks([]Task{{Title: "X"}})
	tr.Clear()
	if tr.Len() != 0 {
		t.Errorf("Len() = %d after Clear()", tr.Len())
	}
}

func TestTracker_Counts(t *testing.T) {
	tr := NewTracker()
	tr.SetTasks([]Task{
		{Title: "P1", Status: StatusPending},
		{Title: "P2", Status: StatusPending},
		{Title: "A1", Status: StatusActive},
		{Title: "D1", Status: StatusDone},
	})
	pending, active, done, failed := tr.Counts()
	if pending != 2 || active != 1 || done != 1 || failed != 0 {
		t.Errorf("Counts() = (%d, %d, %d, %d), want (2, 1, 1, 0)", pending, active, done, failed)
	}
}

func TestTracker_Summary(t *testing.T) {
	tr := NewTracker()
	if tr.Summary() != "No tasks" {
		t.Errorf("empty summary = %q", tr.Summary())
	}

	tr.SetTasks([]Task{
		{Title: "A", Status: StatusDone},
		{Title: "B", Status: StatusActive},
		{Title: "C", Status: StatusPending},
	})
	got := tr.Summary()
	want := "1/3 done, 1 active, 1 pending"
	if got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestTracker_TasksReturnsCopy(t *testing.T) {
	tr := NewTracker()
	tr.SetTasks([]Task{{Title: "Original"}})

	snap := tr.Tasks()
	snap[0].Title = "Modified"

	// Original should be unchanged
	got := tr.Tasks()
	if got[0].Title != "Original" {
		t.Errorf("Tasks() returned reference, not copy: got %q", got[0].Title)
	}
}

func TestTracker_ConcurrentAccess(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			tr.UpdateTask("task", StatusActive)
			tr.UpdateTask("task", StatusDone)
		}
	}()

	// Reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = tr.Tasks()
			_ = tr.Summary()
			tr.Counts()
		}
	}()

	wg.Wait()
}

func TestTracker_UpdateTaskByKeyword(t *testing.T) {
	tests := []struct {
		name           string
		initialTasks   []Task
		keyword        string
		status         TaskStatus
		expectedStatus []TaskStatus
	}{
		{
			name: "matches active task and transitions to done",
			initialTasks: []Task{
				{Title: "Read configuration files", Status: StatusActive},
				{Title: "Write unit tests", Status: StatusPending},
			},
			keyword:        "configuration",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusDone, StatusPending},
		},
		{
			name: "matches active task and transitions to failed",
			initialTasks: []Task{
				{Title: "Build the project", Status: StatusActive},
				{Title: "Deploy to staging", Status: StatusPending},
			},
			keyword:        "build",
			status:         StatusFailed,
			expectedStatus: []TaskStatus{StatusFailed, StatusPending},
		},
		{
			name: "does not transition non-active tasks",
			initialTasks: []Task{
				{Title: "Read config", Status: StatusPending},
				{Title: "Read config", Status: StatusDone},
				{Title: "Read config", Status: StatusFailed},
			},
			keyword:        "read_config",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusPending, StatusDone, StatusFailed},
		},
		{
			name: "empty keyword returns early",
			initialTasks: []Task{
				{Title: "Active task", Status: StatusActive},
			},
			keyword:        "",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusActive},
		},
		{
			name: "short keyword (less than 4 chars) returns early",
			initialTasks: []Task{
				{Title: "Read files", Status: StatusActive},
			},
			keyword:        "abc",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusActive},
		},
		{
			name: "case-insensitive matching",
			initialTasks: []Task{
				{Title: "BUILD THE PROJECT", Status: StatusActive},
			},
			keyword:        "build",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusDone},
		},
		{
			name: "first-match-only behavior",
			initialTasks: []Task{
				{Title: "Build frontend", Status: StatusActive},
				{Title: "Build backend", Status: StatusActive},
				{Title: "Build tests", Status: StatusActive},
			},
			keyword:        "frontend",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusDone, StatusActive, StatusActive},
		},
		{
			name: "no matching keyword leaves tasks unchanged",
			initialTasks: []Task{
				{Title: "Deploy to production", Status: StatusActive},
				{Title: "Run tests", Status: StatusActive},
			},
			keyword:        "documentation",
			status:         StatusDone,
			expectedStatus: []TaskStatus{StatusActive, StatusActive},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := NewTracker()
			tr.SetTasks(tt.initialTasks)

			tr.UpdateTaskByKeyword(tt.keyword, tt.status)

			tasks := tr.Tasks()
			if len(tasks) != len(tt.expectedStatus) {
				t.Fatalf("expected %d tasks, got %d", len(tt.expectedStatus), len(tasks))
			}
			for i, task := range tasks {
				if task.Status != tt.expectedStatus[i] {
					t.Errorf("task[%d] status = %q, want %q", i, task.Status, tt.expectedStatus[i])
				}
			}
		})
	}
}

func TestTracker_UpdateTaskByKeyword_ConcurrentAccess(t *testing.T) {
	tr := NewTracker()
	tr.SetTasks([]Task{
		{Title: "Task one active", Status: StatusActive},
		{Title: "Task two active", Status: StatusActive},
	})

	var wg sync.WaitGroup

	// Concurrent keyword updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			tr.UpdateTaskByKeyword("task_one", StatusDone)
			tr.UpdateTaskByKeyword("task_two", StatusFailed)
		}
	}()

	// Concurrent reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = tr.Tasks()
			_ = tr.Summary()
			tr.Counts()
		}
	}()

	wg.Wait()
}
