package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/tools"
	"github.com/visdomtech/kimi-code/internal/agentcore/event"
	"github.com/visdomtech/kimi-code/internal/kosong"
)

// TurnStatus represents the status of a turn.
type TurnStatus string

const (
	TurnPending   TurnStatus = "pending"
	TurnRunning   TurnStatus = "running"
	TurnCompleted TurnStatus = "completed"
	TurnFailed    TurnStatus = "failed"
	TurnAborted   TurnStatus = "aborted"
)

// TurnJob represents a single turn in the agent loop.
type TurnJob struct {
	ID        string
	SessionID string
	Prompt    string
	Status    TurnStatus
	CreatedAt time.Time
	ctx       context.Context
	cancel    context.CancelFunc
}

// StepRequest represents a single LLM step within a turn.
type StepRequest struct {
	TurnID    string
	Seq       int
	Messages  []kosong.Message
	Tools     []kosong.Tool
}

// StepResult is the result of a single step.
type StepResult struct {
	StepSeq      int
	Message      *kosong.Message
	ToolCalls    []kosong.ToolCall
	FinishReason kosong.FinishReason
	Usage        *kosong.TokenUsage
}

// Event is a loop-level event.
type Event struct {
	Type      string    `json:"type"`
	TurnID    string    `json:"turnId"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// Service is the agent loop service that manages turns and steps.
type Service struct {
	provider     kosong.ChatProvider
	toolRegistry *tools.Registry
	eventBus     *event.Bus[Event]

	turnQueue   chan *TurnJob
	stepSeq     atomic.Int64
	maxTurns    int
	maxSteps    int

	mu          sync.RWMutex
	activeTurn  *TurnJob
	running     bool
}

// Config holds loop service configuration.
type Config struct {
	MaxTurns        int
	MaxStepsPerTurn int
}

// NewService creates a new loop service.
func NewService(
	provider kosong.ChatProvider,
	toolRegistry *tools.Registry,
	eventBus *event.Bus[Event],
	cfg Config,
) *Service {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 100
	}
	if cfg.MaxStepsPerTurn <= 0 {
		cfg.MaxStepsPerTurn = 50
	}
	return &Service{
		provider:     provider,
		toolRegistry: toolRegistry,
		eventBus:     eventBus,
		turnQueue:    make(chan *TurnJob, 100),
		maxTurns:     cfg.MaxTurns,
		maxSteps:     cfg.MaxStepsPerTurn,
	}
}

// Start begins processing turns from the queue.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.processLoop(ctx)
}

// Stop stops the loop service.
func (s *Service) Stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// SubmitTurn submits a new turn to the queue.
func (s *Service) SubmitTurn(ctx context.Context, sessionID, prompt string) (*TurnJob, error) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil, errors.New("loop service not running")
	}
	s.mu.Unlock()

	turnCtx, cancel := context.WithCancel(ctx)
	turn := &TurnJob{
		ID:        fmt.Sprintf("turn_%d", time.Now().UnixNano()),
		SessionID: sessionID,
		Prompt:    prompt,
		Status:    TurnPending,
		CreatedAt: time.Now(),
		ctx:       turnCtx,
		cancel:    cancel,
	}

	select {
	case s.turnQueue <- turn:
		return turn, nil
	default:
		cancel()
		return nil, errors.New("turn queue full")
	}
}

// AbortTurn aborts an active turn.
func (s *Service) AbortTurn(turnID string) error {
	s.mu.RLock()
	active := s.activeTurn
	s.mu.RUnlock()

	if active == nil || active.ID != turnID {
		return errors.New("turn not active")
	}
	active.cancel()
	return nil
}

func (s *Service) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case turn := <-s.turnQueue:
			s.mu.Lock()
			if !s.running {
				s.mu.Unlock()
				return
			}
			s.activeTurn = turn
			s.mu.Unlock()

			s.executeTurn(ctx, turn)

			s.mu.Lock()
			s.activeTurn = nil
			s.mu.Unlock()
		}
	}
}

func (s *Service) executeTurn(parentCtx context.Context, turn *TurnJob) {
	turn.Status = TurnRunning

	s.eventBus.Publish(Event{
		Type:      "turn.started",
		TurnID:    turn.ID,
		SessionID: turn.SessionID,
		Timestamp: time.Now(),
		Data:      map[string]any{"prompt": turn.Prompt},
	})

	// Build initial messages
	messages := []kosong.Message{
		kosong.CreateUserMessage(turn.Prompt),
	}

	// Execute steps
	for step := 0; step < s.maxSteps; step++ {
		select {
		case <-turn.ctx.Done():
			turn.Status = TurnAborted
			s.eventBus.Publish(Event{
				Type:      "turn.aborted",
				TurnID:    turn.ID,
				SessionID: turn.SessionID,
				Timestamp: time.Now(),
			})
			return
		case <-parentCtx.Done():
			return
		default:
		}

		result, err := s.executeStep(turn, step, messages)
		if err != nil {
			turn.Status = TurnFailed
			s.eventBus.Publish(Event{
				Type:      "turn.failed",
				TurnID:    turn.ID,
				SessionID: turn.SessionID,
				Timestamp: time.Now(),
				Data:      map[string]any{"error": err.Error()},
			})
			return
		}

		// Add assistant message to history
		if result.Message != nil {
			messages = append(messages, *result.Message)
		}

		// If no tool calls, turn is complete
		if len(result.ToolCalls) == 0 {
			break
		}

		// Execute tool calls
		for _, tc := range result.ToolCalls {
			toolResult, err := s.executeToolCall(turn.ctx, tc)
			if err != nil {
				toolResult = &tools.Result{Output: err.Error(), IsError: true}
			}

			s.eventBus.Publish(Event{
				Type:      "tool_call.completed",
				TurnID:    turn.ID,
				SessionID: turn.SessionID,
				Timestamp: time.Now(),
				Data:      map[string]any{"name": tc.Name, "result": toolResult.Output},
			})

			// Add tool result message
			toolMsg := kosong.CreateToolMessage(tc.ID, toolResult.Output)
			messages = append(messages, toolMsg)
		}
	}

	turn.Status = TurnCompleted
	s.eventBus.Publish(Event{
		Type:      "turn.completed",
		TurnID:    turn.ID,
		SessionID: turn.SessionID,
		Timestamp: time.Now(),
	})
}

func (s *Service) executeStep(turn *TurnJob, step int, messages []kosong.Message) (*StepResult, error) {
	seq := s.stepSeq.Add(1)

	s.eventBus.Publish(Event{
		Type:      "step.started",
		TurnID:    turn.ID,
		SessionID: turn.SessionID,
		Timestamp: time.Now(),
		Data:      map[string]any{"seq": seq},
	})

	// Convert tool definitions
	var kosongTools []kosong.Tool
	for _, def := range s.toolRegistry.Definitions() {
		kosongTools = append(kosongTools, kosong.Tool{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}

	// Call LLM
	stream, err := s.provider.Generate(turn.ctx, "", kosongTools, messages, nil)
	if err != nil {
		return nil, err
	}

	// Consume stream
	msg, err := kosong.Generate(turn.ctx, stream)
	if err != nil {
		return nil, err
	}

	// Extract tool calls from the message
	var toolCalls []kosong.ToolCall
	for _, tc := range msg.ToolCalls {
		toolCalls = append(toolCalls, kosong.ToolCall{
			Type:      tc.Type,
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}

	result := &StepResult{
		StepSeq:   int(seq),
		Message:   msg,
		ToolCalls: toolCalls,
	}

	s.eventBus.Publish(Event{
		Type:      "step.completed",
		TurnID:    turn.ID,
		SessionID: turn.SessionID,
		Timestamp: time.Now(),
		Data:      map[string]any{"seq": seq, "toolCalls": len(toolCalls)},
	})

	return result, nil
}

func (s *Service) executeToolCall(ctx context.Context, tc kosong.ToolCall) (*tools.Result, error) {
	tool, ok := s.toolRegistry.Get(tc.Name)
	if !ok {
		return nil, fmt.Errorf("tool %q not found", tc.Name)
	}

	var input json.RawMessage
	if tc.Arguments != nil {
		input = json.RawMessage(*tc.Arguments)
	} else {
		input = json.RawMessage("{}")
	}

	return tool.Execute(ctx, input, tools.ExecContext{})
}
