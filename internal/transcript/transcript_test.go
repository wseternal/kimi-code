package transcript

import (
	"testing"
	"time"
)

func TestNewSnapshot(t *testing.T) {
	snap := NewSnapshot()
	if snap == nil {
		t.Fatal("NewSnapshot returned nil")
	}
	if len(snap.Turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(snap.Turns))
	}
}

func TestApplyTurnUpsert(t *testing.T) {
	snap := NewSnapshot()
	turn := TranscriptTurn{
		ID:        NewTurnID(0),
		Index:     0,
		CreatedAt: time.Now(),
		Origin:    &TurnOrigin{Kind: "user", Prompt: "hello"},
	}

	result := ApplyOperation(snap, Operation{Kind: OpTurnUpsert, Turn: &turn})
	if !result.Changed {
		t.Fatal("expected changed")
	}
	if len(result.State.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(result.State.Turns))
	}
	if result.State.Turns[0].Origin.Prompt != "hello" {
		t.Errorf("expected prompt 'hello', got %q", result.State.Turns[0].Origin.Prompt)
	}

	// Update same turn
	updated := turn
	updated.Origin.Prompt = "hello world"
	result2 := ApplyOperation(result.State, Operation{Kind: OpTurnUpsert, Turn: &updated})
	if len(result2.State.Turns) != 1 {
		t.Fatalf("expected 1 turn after upsert, got %d", len(result2.State.Turns))
	}
	if result2.State.Turns[0].Origin.Prompt != "hello world" {
		t.Errorf("expected updated prompt, got %q", result2.State.Turns[0].Origin.Prompt)
	}
}

func TestApplyStepUpsert(t *testing.T) {
	snap := NewSnapshot()
	turn := TranscriptTurn{ID: NewTurnID(0), Index: 0, CreatedAt: time.Now()}
	snap.Turns = append(snap.Turns, turn)

	step := TranscriptStep{
		ID:           NewStepID(turn.ID, 0),
		TurnID:       turn.ID,
		Index:        0,
		FinishReason: "completed",
	}

	result := ApplyOperation(snap, Operation{Kind: OpStepUpsert, Step: &step})
	if !result.Changed {
		t.Fatal("expected changed")
	}
	if len(result.State.Turns[0].Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.State.Turns[0].Steps))
	}
}

func TestApplyFrameUpsertAndAppend(t *testing.T) {
	snap := NewSnapshot()
	turnID := NewTurnID(0)
	stepID := NewStepID(turnID, 0)
	frameID := NewFrameID(stepID, 0)

	frame := Frame{
		ID:     frameID,
		StepID: stepID,
		Index:  0,
		Kind:   FrameText,
		Text:   &TextFrame{Kind: FrameText, Content: "Hello"},
	}

	result := ApplyOperation(snap, Operation{Kind: OpFrameUpsert, Frame: &frame})
	if len(result.State.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(result.State.Frames))
	}

	// Append to frame
	result2 := ApplyOperation(result.State, Operation{
		Kind:    OpAppend,
		FrameID: frameID,
		Content: " World",
	})
	if result2.State.Frames[0].Text.Content != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", result2.State.Frames[0].Text.Content)
	}
}

func TestApplyReset(t *testing.T) {
	snap := NewSnapshot()
	snap.Turns = append(snap.Turns, TranscriptTurn{ID: NewTurnID(0)})
	snap.Frames = append(snap.Frames, Frame{ID: "f1"})

	result := ApplyOperation(snap, Operation{Kind: OpReset})
	if !result.Changed {
		t.Fatal("expected changed")
	}
	if len(result.State.Turns) != 0 || len(result.State.Frames) != 0 {
		t.Error("expected empty state after reset")
	}
}

func TestApplyMetaMerge(t *testing.T) {
	snap := NewSnapshot()

	meta := TranscriptMeta{Model: "kimi-latest"}
	result := ApplyOperation(snap, Operation{Kind: OpMetaMerge, MetaPatch: &meta})
	if result.State.Meta.Model != "kimi-latest" {
		t.Errorf("expected model 'kimi-latest', got %q", result.State.Meta.Model)
	}

	// Merge additional fields
	meta2 := TranscriptMeta{Provider: "kimi", Goal: &GoalMeta{Objective: "test"}}
	result2 := ApplyOperation(result.State, Operation{Kind: OpMetaMerge, MetaPatch: &meta2})
	if result2.State.Meta.Model != "kimi-latest" {
		t.Error("model should be preserved")
	}
	if result2.State.Meta.Provider != "kimi" {
		t.Errorf("expected provider 'kimi', got %q", result2.State.Meta.Provider)
	}
	if result2.State.Meta.Goal == nil || result2.State.Meta.Goal.Objective != "test" {
		t.Error("goal not merged")
	}
}

func TestApplyInteractionUpsertAndResolve(t *testing.T) {
	snap := NewSnapshot()
	interaction := TranscriptInteraction{
		ID:        NewInteractionID("i1"),
		Kind:      InteractionApproval,
		Prompt:    "Allow file write?",
		CreatedAt: time.Now(),
	}

	result := ApplyOperation(snap, Operation{Kind: OpInteractionUpsert, Interaction: &interaction})
	if len(result.State.Interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(result.State.Interactions))
	}

	result2 := ApplyOperation(result.State, Operation{
		Kind:          OpInteractionResolve,
		InteractionID: "i1",
		Response:      "approved",
	})
	if !result2.State.Interactions[0].Resolved {
		t.Error("expected resolved")
	}
	if result2.State.Interactions[0].Response != "approved" {
		t.Errorf("expected 'approved', got %q", result2.State.Interactions[0].Response)
	}
}

func TestApplyTaskUpsert(t *testing.T) {
	snap := NewSnapshot()
	task := TranscriptTask{
		ID:        NewTaskID("t1"),
		Kind:      TaskShell,
		Label:     "ls -la",
		Status:    "running",
		StartedAt: time.Now(),
	}

	result := ApplyOperation(snap, Operation{Kind: OpTaskUpsert, Task: &task})
	if len(result.State.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.State.Tasks))
	}

	// Update same task
	now := time.Now()
	completed := task
	completed.Status = "completed"
	completed.EndedAt = &now
	result2 := ApplyOperation(result.State, Operation{Kind: OpTaskUpsert, Task: &completed})
	if len(result2.State.Tasks) != 1 {
		t.Fatal("should upsert, not append")
	}
	if result2.State.Tasks[0].Status != "completed" {
		t.Errorf("expected 'completed', got %q", result2.State.Tasks[0].Status)
	}
}

func TestApplyItemsRemove(t *testing.T) {
	snap := NewSnapshot()
	snap.Items = []TranscriptItem{
		{TurnID: "t0", Marker: &TranscriptMarker{Kind: "divider", Label: "a"}},
		{TurnID: "t0", Marker: &TranscriptMarker{Kind: "divider", Label: "b"}},
		{TurnID: "t0", Marker: &TranscriptMarker{Kind: "divider", Label: "c"}},
	}

	result := ApplyOperation(snap, Operation{Kind: OpItemsRemove, ItemIndices: []int{1}})
	if len(result.State.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.State.Items))
	}
	if result.State.Items[0].Marker.Label != "a" || result.State.Items[1].Marker.Label != "c" {
		t.Error("wrong items remaining")
	}
}

func TestStoreApplyAndState(t *testing.T) {
	store := NewStore("", "")

	turn := TranscriptTurn{ID: NewTurnID(0), Index: 0, CreatedAt: time.Now()}
	store.Apply(Operation{Kind: OpTurnUpsert, Turn: &turn})

	state := store.State()
	if len(state.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(state.Turns))
	}

	ops := store.Operations()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
}

func TestPaginate(t *testing.T) {
	snap := NewSnapshot()
	for i := 0; i < 10; i++ {
		snap.Turns = append(snap.Turns, TranscriptTurn{
			ID:        NewTurnID(i),
			Index:     i,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}

	page := Paginate(snap, 0, 3)
	if page.Total != 10 {
		t.Errorf("expected total 10, got %d", page.Total)
	}
	if len(page.Turns) != 3 {
		t.Errorf("expected 3 turns, got %d", len(page.Turns))
	}
	if !page.HasMore {
		t.Error("expected hasMore=true")
	}

	// Last page should have 1 turn
	lastPage := Paginate(snap, 3, 3)
	if len(lastPage.Turns) != 1 {
		t.Errorf("expected 1 turn on last page, got %d", len(lastPage.Turns))
	}
	if lastPage.HasMore {
		t.Error("expected hasMore=false on last page")
	}
}

func TestGroupTurns(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	turns := []TranscriptTurn{
		{ID: "t0", CreatedAt: base},
		{ID: "t1", CreatedAt: base.Add(1 * time.Minute)},
		{ID: "t2", CreatedAt: base.Add(2 * time.Minute)},
		// Gap of 1 hour
		{ID: "t3", CreatedAt: base.Add(62 * time.Minute)},
		{ID: "t4", CreatedAt: base.Add(63 * time.Minute)},
	}

	groups := GroupTurns(turns, 30*time.Minute)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0].Turns) != 3 {
		t.Errorf("expected 3 turns in first group, got %d", len(groups[0].Turns))
	}
	if len(groups[1].Turns) != 2 {
		t.Errorf("expected 2 turns in second group, got %d", len(groups[1].Turns))
	}
}

func TestCopyOnWrite(t *testing.T) {
	snap := NewSnapshot()
	turn := TranscriptTurn{ID: NewTurnID(0), Index: 0, CreatedAt: time.Now()}
	snap.Turns = append(snap.Turns, turn)

	result := ApplyOperation(snap, Operation{
		Kind:  OpTurnUpsert,
		Turn:  &TranscriptTurn{ID: NewTurnID(1), Index: 1, CreatedAt: time.Now()},
	})

	// Original should be unchanged
	if len(snap.Turns) != 1 {
		t.Errorf("original mutated: expected 1 turn, got %d", len(snap.Turns))
	}
	if len(result.State.Turns) != 2 {
		t.Errorf("result should have 2 turns, got %d", len(result.State.Turns))
	}
}
