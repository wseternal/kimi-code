package loop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentctx "github.com/visdomtech/kimi-code/internal/agentcore/agent/context"
	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
	"github.com/visdomtech/kimi-code/internal/agentcore/event"
	"github.com/visdomtech/kimi-code/internal/kosong"
)

// mockProvider records the messages it receives on each Generate call,
// and returns a simple text-only response.
type mockProvider struct {
	mu       sync.Mutex
	calls    [][]kosong.Message // each entry is the history slice passed to Generate
	response string             // text the mock assistant replies with
}

func (m *mockProvider) Name() string                                  { return "mock" }
func (m *mockProvider) ModelName() string                             { return "mock-model" }
func (m *mockProvider) ThinkingEffort() kosong.ThinkingEffort         { return kosong.ThinkingOff }
func (m *mockProvider) MaxCompletionTokens() int                      { return 0 }
func (m *mockProvider) WithThinking(kosong.ThinkingEffort) kosong.ChatProvider { return m }
func (m *mockProvider) WithMaxCompletionTokens(int, *kosong.MaxCompletionTokensOptions) kosong.ChatProvider {
	return m
}
func (m *mockProvider) UploadVideo(context.Context, interface{}, *kosong.GenerateOptions) (*kosong.VideoURLPart, error) {
	return nil, nil
}

func (m *mockProvider) Generate(
	_ context.Context,
	_ string,
	_ []kosong.Tool,
	history []kosong.Message,
	_ *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	m.mu.Lock()
	// Deep-copy the history so later mutations don't affect the snapshot.
	snapshot := make([]kosong.Message, len(history))
	copy(snapshot, history)
	m.calls = append(m.calls, snapshot)
	m.mu.Unlock()

	// Return a simple text response with no tool calls.
	ch := make(chan kosong.StreamedMessagePart, 1)
	ch <- kosong.StreamedMessagePart{Type: "text", Text: m.response}
	close(ch)
	return &kosong.StreamedMessage{Parts: ch}, nil
}

// TestExecuteTurn_IncludesConversationHistory verifies that the second turn
// in the same session includes messages from the first turn.
// This is the Prove-It test for the bug: "agent forgets previous messages".
func TestExecuteTurn_IncludesConversationHistory(t *testing.T) {
	provider := &mockProvider{response: "I remember!"}
	toolReg := tools.NewRegistry()
	eventBus := event.NewBus[Event]()

	svc := NewService(provider, toolReg, eventBus, Config{
		MaxTurns:        10,
		MaxStepsPerTurn: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.Start(ctx)
	defer svc.Stop()

	sessionID := "test-session"

	// Turn 1: user says "how are you"
	turn1, err := svc.SubmitTurn(ctx, sessionID, "how are you")
	if err != nil {
		t.Fatalf("SubmitTurn 1 failed: %v", err)
	}

	// Wait for turn 1 to complete
	waitForTurn(t, turn1, 3*time.Second)

	// Turn 2: user says "what did I asked just now?"
	turn2, err := svc.SubmitTurn(ctx, sessionID, "what did I asked just now?")
	if err != nil {
		t.Fatalf("SubmitTurn 2 failed: %v", err)
	}

	waitForTurn(t, turn2, 3*time.Second)

	// Verify: the provider should have been called at least twice
	provider.mu.Lock()
	defer provider.mu.Unlock()

	if len(provider.calls) < 2 {
		t.Fatalf("expected at least 2 Generate calls, got %d", len(provider.calls))
	}

	// Turn 1 call: should have 1 user message ("how are you")
	turn1Messages := provider.calls[0]
	if len(turn1Messages) < 1 {
		t.Fatalf("turn 1: expected at least 1 message, got %d", len(turn1Messages))
	}
	if turn1Messages[0].Role != kosong.RoleUser {
		t.Errorf("turn 1 first message role = %q, want %q", turn1Messages[0].Role, kosong.RoleUser)
	}

	// Turn 2 call: MUST include the previous conversation (user + assistant + new user)
	turn2Messages := provider.calls[1]
	if len(turn2Messages) < 3 {
		t.Fatalf("turn 2: expected at least 3 messages (prev user + prev assistant + new user), got %d", len(turn2Messages))
	}

	// First message should be the original user prompt from turn 1
	if turn2Messages[0].Role != kosong.RoleUser {
		t.Errorf("turn 2 msg[0] role = %q, want user", turn2Messages[0].Role)
	}
	firstText := extractTextFromMessage(turn2Messages[0])
	if firstText != "how are you" {
		t.Errorf("turn 2 msg[0] text = %q, want %q", firstText, "how are you")
	}

	// Second message should be the assistant response from turn 1
	if turn2Messages[1].Role != kosong.RoleAssistant {
		t.Errorf("turn 2 msg[1] role = %q, want assistant", turn2Messages[1].Role)
	}

	// Third message should be the new user prompt
	if turn2Messages[2].Role != kosong.RoleUser {
		t.Errorf("turn 2 msg[2] role = %q, want user", turn2Messages[2].Role)
	}
	thirdText := extractTextFromMessage(turn2Messages[2])
	if thirdText != "what did I asked just now?" {
		t.Errorf("turn 2 msg[2] text = %q, want %q", thirdText, "what did I asked just now?")
	}
}

// TestExecuteTurn_DifferentSessionsHaveIndependentHistory verifies that
// conversation history is isolated per session.
func TestExecuteTurn_DifferentSessionsHaveIndependentHistory(t *testing.T) {
	provider := &mockProvider{response: "OK"}
	toolReg := tools.NewRegistry()
	eventBus := event.NewBus[Event]()

	svc := NewService(provider, toolReg, eventBus, Config{
		MaxTurns:        10,
		MaxStepsPerTurn: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.Start(ctx)
	defer svc.Stop()

	// Session A: turn 1
	turnA1, err := svc.SubmitTurn(ctx, "session-A", "Hello from A")
	if err != nil {
		t.Fatalf("SubmitTurn A1 failed: %v", err)
	}
	waitForTurn(t, turnA1, 3*time.Second)

	// Session B: turn 1
	turnB1, err := svc.SubmitTurn(ctx, "session-B", "Hello from B")
	if err != nil {
		t.Fatalf("SubmitTurn B1 failed: %v", err)
	}
	waitForTurn(t, turnB1, 3*time.Second)

	// Session A: turn 2
	turnA2, err := svc.SubmitTurn(ctx, "session-A", "Second message from A")
	if err != nil {
		t.Fatalf("SubmitTurn A2 failed: %v", err)
	}
	waitForTurn(t, turnA2, 3*time.Second)

	provider.mu.Lock()
	defer provider.mu.Unlock()

	if len(provider.calls) < 3 {
		t.Fatalf("expected at least 3 Generate calls, got %d", len(provider.calls))
	}

	// Session A turn 2 (call index 2): should contain A's history, NOT B's
	sessionA2Messages := provider.calls[2]
	for _, msg := range sessionA2Messages {
		text := extractTextFromMessage(msg)
		if text == "Hello from B" {
			t.Error("session A turn 2 should NOT contain messages from session B")
		}
	}

	// Should contain A's first message
	foundA := false
	for _, msg := range sessionA2Messages {
		if extractTextFromMessage(msg) == "Hello from A" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Error("session A turn 2 should contain A's first message")
	}
}

// waitForTurn blocks until the turn reaches a terminal state or times out.
func waitForTurn(t *testing.T, turn *TurnJob, timeout time.Duration) {
	t.Helper()
	select {
	case <-turn.Done():
		return
	case <-time.After(timeout):
		t.Fatalf("turn %s did not complete within %v (status: %s)", turn.ID, timeout, turn.TurnStatus())
	}
}

// extractTextFromMessage concatenates text content parts from a message.
func extractTextFromMessage(msg kosong.Message) string {
	var result string
	for _, part := range msg.Content {
		if part.Type == "text" {
			result += part.Text
		}
	}
	return result
}

// TestStreamingEventsEmitted verifies that text.delta and step events are
// emitted during a turn via the event bus.
func TestStreamingEventsEmitted(t *testing.T) {
	provider := &mockProvider{response: "Hello streaming!"}
	toolReg := tools.NewRegistry()
	eventBus := event.NewBus[Event]()

	// Collect events
	var mu sync.Mutex
	var events []Event
	eventBus.Subscribe(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	svc := NewService(provider, toolReg, eventBus, Config{
		MaxTurns:        10,
		MaxStepsPerTurn: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.Start(ctx)
	defer svc.Stop()

	turn, err := svc.SubmitTurn(ctx, "test-session", "hi")
	if err != nil {
		t.Fatalf("SubmitTurn failed: %v", err)
	}
	waitForTurn(t, turn, 3*time.Second)

	// Give subscriber a moment to drain
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	hasTurnStarted := false
	hasStepStarted := false
	hasTextDelta := false
	hasStepCompleted := false
	hasTurnCompleted := false
	for _, ev := range events {
		switch ev.Type {
		case "turn.started":
			hasTurnStarted = true
		case "step.started":
			hasStepStarted = true
		case "text.delta":
			hasTextDelta = true
		case "step.completed":
			hasStepCompleted = true
		case "turn.completed":
			hasTurnCompleted = true
		}
	}

	if !hasTurnStarted {
		t.Error("missing turn.started event")
	}
	if !hasStepStarted {
		t.Error("missing step.started event")
	}
	if !hasTextDelta {
		t.Error("missing text.delta event")
	}
	if !hasStepCompleted {
		t.Error("missing step.completed event")
	}
	if !hasTurnCompleted {
		t.Error("missing turn.completed event")
	}
}

// TestAutoCompactionTrigger verifies that auto-compaction is triggered
// when token usage exceeds the threshold.
func TestAutoCompactionTrigger(t *testing.T) {
	provider := &mockProvider{response: "compacted reply"}
	toolReg := tools.NewRegistry()
	eventBus := event.NewBus[Event]()

	// Track compaction calls
	var compactMu sync.Mutex
	compactCalled := 0
	compactFn := func(msgs []kosong.Message) ([]kosong.Message, error) {
		compactMu.Lock()
		compactCalled++
		compactMu.Unlock()
		// Return only the last 2 messages (simulate compaction)
		if len(msgs) > 2 {
			return msgs[len(msgs)-2:], nil
		}
		return msgs, nil
	}

	// Create a strategy with very low trigger ratio
	strategy := agentctx.NewCompactionStrategy(agentctx.CompactionConfig{
		TriggerRatio:         0.01, // trigger on any usage
		BlockRatio:           0.99,
		ReservedContextSize:  0,
		MaxCompactionPerTurn: 3,
		MaxOverflowAttempts:  3,
	})
	cm := agentctx.NewContextManager(100, 0.01, 0) // 100 token window

	svc := NewService(provider, toolReg, eventBus, Config{
		MaxTurns:        10,
		MaxStepsPerTurn: 5,
	}, WithCompaction(compactFn, strategy, cm))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.Start(ctx)
	defer svc.Stop()

	turn, err := svc.SubmitTurn(ctx, "test-session", "hello world this is a long message")
	if err != nil {
		t.Fatalf("SubmitTurn failed: %v", err)
	}
	waitForTurn(t, turn, 3*time.Second)

	compactMu.Lock()
	calls := compactCalled
	compactMu.Unlock()

	if calls == 0 {
		t.Error("expected compaction to be called at least once")
	}
}

// TestOverflowRecovery verifies that context overflow errors trigger
// compaction and retry.
func TestOverflowRecovery(t *testing.T) {
	// Provider that fails once with context overflow, then succeeds
	callCount := 0
	provider := &overflowMockProvider{
		firstErr: kosong.NewAPIContextOverflowError(400, "context too long", nil, nil, nil),
		response: "recovered reply",
		onCall: func() {
			callCount++
		},
	}
	toolReg := tools.NewRegistry()
	eventBus := event.NewBus[Event]()

	compactFn := func(msgs []kosong.Message) ([]kosong.Message, error) {
		if len(msgs) > 1 {
			return msgs[len(msgs)-1:], nil
		}
		return msgs, nil
	}

	strategy := agentctx.NewCompactionStrategy(agentctx.DefaultCompactionConfig())
	cm := agentctx.NewContextManager(1000, 0.85, 0)

	svc := NewService(provider, toolReg, eventBus, Config{
		MaxTurns:        10,
		MaxStepsPerTurn: 5,
	}, WithCompaction(compactFn, strategy, cm))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.Start(ctx)
	defer svc.Stop()

	turn, err := svc.SubmitTurn(ctx, "test-session", "hello")
	if err != nil {
		t.Fatalf("SubmitTurn failed: %v", err)
	}
	waitForTurn(t, turn, 3*time.Second)

	// Provider should have been called at least twice (first fail + retry)
	if callCount < 2 {
		t.Errorf("expected at least 2 provider calls (overflow + retry), got %d", callCount)
	}
}

// overflowMockProvider fails with context overflow on first call, then succeeds.
type overflowMockProvider struct {
	firstErr error
	response string
	onCall   func()
	called   bool
}

func (m *overflowMockProvider) Name() string                                      { return "mock" }
func (m *overflowMockProvider) ModelName() string                                 { return "mock-model" }
func (m *overflowMockProvider) ThinkingEffort() kosong.ThinkingEffort             { return kosong.ThinkingOff }
func (m *overflowMockProvider) MaxCompletionTokens() int                          { return 0 }
func (m *overflowMockProvider) WithThinking(kosong.ThinkingEffort) kosong.ChatProvider { return m }
func (m *overflowMockProvider) WithMaxCompletionTokens(int, *kosong.MaxCompletionTokensOptions) kosong.ChatProvider {
	return m
}
func (m *overflowMockProvider) UploadVideo(context.Context, interface{}, *kosong.GenerateOptions) (*kosong.VideoURLPart, error) {
	return nil, nil
}

func (m *overflowMockProvider) Generate(
	ctx context.Context,
	systemPrompt string,
	tools []kosong.Tool,
	history []kosong.Message,
	opts *kosong.GenerateOptions,
) (*kosong.StreamedMessage, error) {
	m.onCall()
	if !m.called {
		m.called = true
		return nil, m.firstErr
	}
	ch := make(chan kosong.StreamedMessagePart, 1)
	ch <- kosong.StreamedMessagePart{Type: "text", Text: m.response}
	close(ch)
	return &kosong.StreamedMessage{Parts: ch}, nil
}

// isContextOverflowError verifies our error detection works
func TestIsContextOverflowError(t *testing.T) {
	err := kosong.NewAPIContextOverflowError(400, "too long", nil, nil, nil)
	svc := &Service{}
	if !svc.isContextOverflow(err) {
		t.Error("expected isContextOverflow to return true")
	}
	if svc.isContextOverflow(errors.New("random error")) {
		t.Error("expected isContextOverflow to return false for generic error")
	}
}
