package tools

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestSteeringTool_EnqueueDrainAll(t *testing.T) {
	st := NewSteeringTool()

	if st.HasMessages() {
		t.Error("new tool should have no messages")
	}
	if st.Len() != 0 {
		t.Errorf("new tool Len() = %d, want 0", st.Len())
	}

	st.Enqueue("first", false)
	st.Enqueue("second", true)

	if !st.HasMessages() {
		t.Error("should have messages after enqueue")
	}
	if st.Len() != 2 {
		t.Errorf("Len() = %d, want 2", st.Len())
	}

	msgs := st.DrainAll()
	if len(msgs) != 2 {
		t.Fatalf("DrainAll returned %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "first" || msgs[0].Priority {
		t.Errorf("msgs[0] = %+v, want first/priority=false", msgs[0])
	}
	if msgs[1].Content != "second" || !msgs[1].Priority {
		t.Errorf("msgs[1] = %+v, want second/priority=true", msgs[1])
	}

	// After drain, should be empty
	if st.HasMessages() {
		t.Error("should have no messages after DrainAll")
	}
	msgs2 := st.DrainAll()
	if len(msgs2) != 0 {
		t.Errorf("second DrainAll returned %d messages, want 0", len(msgs2))
	}
}

func TestSteeringTool_SignalSwap(t *testing.T) {
	st := NewSteeringTool()

	if st.IsSignaled() {
		t.Error("new tool should not be signaled")
	}

	st.Signal()
	if !st.IsSignaled() {
		t.Error("should be signaled after Signal()")
	}
	// Second read should return false (swap clears it)
	if st.IsSignaled() {
		t.Error("second IsSignaled() should return false after swap")
	}
}

func TestSteeringTool_ExecuteEmpty(t *testing.T) {
	st := NewSteeringTool()
	result, err := st.Execute(context.Background(), nil, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "No steering messages." {
		t.Errorf("output = %q, want %q", result.Output, "No steering messages.")
	}
}

func TestSteeringTool_ExecuteWithMessages(t *testing.T) {
	st := NewSteeringTool()
	st.Enqueue("stop using bash", false)
	st.Enqueue("use grep instead", true)

	result, err := st.Execute(context.Background(), nil, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output == "No steering messages." {
		t.Error("should have formatted output")
	}
	if !strings.Contains(result.Output, "stop using bash") {
		t.Error("output should contain first message")
	}
	if !strings.Contains(result.Output, "use grep instead") {
		t.Error("output should contain second message")
	}
}

func TestSteeringTool_ConcurrentAccess(t *testing.T) {
	st := NewSteeringTool()
	const goroutines = 10
	const messagesPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // enqueue, drain, signal goroutines

	// Writers
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < messagesPerGoroutine; i++ {
				st.Enqueue("msg", false)
			}
		}(g)
	}

	// Readers (drain)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < messagesPerGoroutine; i++ {
				st.DrainAll()
			}
		}()
	}

	// Signalers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < messagesPerGoroutine; i++ {
				st.Signal()
				st.IsSignaled()
			}
		}()
	}

	wg.Wait()
	// No race or panic = pass
}

func TestSteeringTool_Definition(t *testing.T) {
	st := NewSteeringTool()
	def := st.Definition()
	if def.Name != "Steering" {
		t.Errorf("Name = %q, want %q", def.Name, "Steering")
	}
	if def.Description == "" {
		t.Error("Description should not be empty")
	}
}


