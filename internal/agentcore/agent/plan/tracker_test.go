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
