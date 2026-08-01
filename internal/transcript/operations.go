package transcript

import "time"

// OpKind identifies the type of transcript operation.
type OpKind string

const (
	OpReset              OpKind = "reset"
	OpTurnUpsert         OpKind = "turn.upsert"
	OpStepUpsert         OpKind = "step.upsert"
	OpFrameUpsert        OpKind = "frame.upsert"
	OpAppend             OpKind = "append"
	OpMarkerUpsert       OpKind = "marker.upsert"
	OpTaskRefUpsert      OpKind = "taskref.upsert"
	OpTaskUpsert         OpKind = "task.upsert"
	OpInteractionUpsert  OpKind = "interaction.upsert"
	OpAttachmentUpsert   OpKind = "attachment.upsert"
	OpTodoUpsert         OpKind = "todo.upsert"
	OpPromptUpsert       OpKind = "prompt.upsert"
	OpMetaMerge          OpKind = "meta.merge"
	OpItemsRemove        OpKind = "items.remove"
	OpInteractionResolve OpKind = "interaction.resolve"
)

// Operation is a transcript mutation.
type Operation struct {
	Kind OpKind `json:"kind"`

	// TurnUpsert
	Turn *TranscriptTurn `json:"turn,omitempty"`

	// StepUpsert
	Step *TranscriptStep `json:"step,omitempty"`

	// FrameUpsert
	Frame *Frame `json:"frame,omitempty"`

	// Append (text to a frame)
	FrameID   FrameID `json:"frameId,omitempty"`
	Content   string  `json:"content,omitempty"`

	// MarkerUpsert / TaskRefUpsert
	Item *TranscriptItem `json:"item,omitempty"`

	// TaskUpsert
	Task *TranscriptTask `json:"task,omitempty"`

	// InteractionUpsert
	Interaction *TranscriptInteraction `json:"interaction,omitempty"`

	// InteractionResolve
	InteractionID InteractionID `json:"interactionId,omitempty"`
	Response      string        `json:"response,omitempty"`

	// AttachmentUpsert
	Attachment *TranscriptAttachment `json:"attachment,omitempty"`

	// TodoUpsert
	Todo *TranscriptTodo `json:"todo,omitempty"`

	// PromptUpsert
	Prompt *TranscriptPrompt `json:"prompt,omitempty"`

	// MetaMerge
	MetaPatch *TranscriptMeta `json:"metaPatch,omitempty"`

	// ItemsRemove
	ItemIndices []int `json:"itemIndices,omitempty"`
}

// ApplyResult is the result of applying an operation.
type ApplyResult struct {
	State   *Snapshot
	Changed bool
}

// ApplyOperation applies a single operation to a snapshot, returning a new
// snapshot (copy-on-write). The original is never mutated.
func ApplyOperation(state *Snapshot, op Operation) ApplyResult {
	if state == nil {
		state = NewSnapshot()
	}

	switch op.Kind {
	case OpReset:
		return ApplyResult{State: NewSnapshot(), Changed: true}

	case OpTurnUpsert:
		return applyTurnUpsert(state, op)

	case OpStepUpsert:
		return applyStepUpsert(state, op)

	case OpFrameUpsert:
		return applyFrameUpsert(state, op)

	case OpAppend:
		return applyAppend(state, op)

	case OpMarkerUpsert, OpTaskRefUpsert:
		return applyItemUpsert(state, op)

	case OpTaskUpsert:
		return applyTaskUpsert(state, op)

	case OpInteractionUpsert:
		return applyInteractionUpsert(state, op)

	case OpInteractionResolve:
		return applyInteractionResolve(state, op)

	case OpAttachmentUpsert:
		return applyAttachmentUpsert(state, op)

	case OpTodoUpsert:
		return ApplyResult{
			State:   copyWith(state, func(s *Snapshot) { s.Todos = op.Todo }),
			Changed: true,
		}

	case OpPromptUpsert:
		return applyPromptUpsert(state, op)

	case OpMetaMerge:
		return applyMetaMerge(state, op)

	case OpItemsRemove:
		return applyItemsRemove(state, op)

	default:
		return ApplyResult{State: state, Changed: false}
	}
}

// ── Apply helpers ──

func applyTurnUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Turn == nil {
		return ApplyResult{State: state, Changed: false}
	}
	turns := make([]TranscriptTurn, len(state.Turns))
	copy(turns, state.Turns)
	found := false
	for i, t := range turns {
		if t.ID == op.Turn.ID {
			turns[i] = *op.Turn
			found = true
			break
		}
	}
	if !found {
		turns = append(turns, *op.Turn)
	}
	s := copySnapshot(state)
	s.Turns = turns
	return ApplyResult{State: s, Changed: true}
}

func applyStepUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Step == nil {
		return ApplyResult{State: state, Changed: false}
	}
	turns := make([]TranscriptTurn, len(state.Turns))
	copy(turns, state.Turns)
	for i, t := range turns {
		if t.ID == op.Step.TurnID {
			steps := make([]TranscriptStep, len(t.Steps))
			copy(steps, t.Steps)
			found := false
			for j, st := range steps {
				if st.ID == op.Step.ID {
					steps[j] = *op.Step
					found = true
					break
				}
			}
			if !found {
				steps = append(steps, *op.Step)
			}
			turns[i].Steps = steps
			break
		}
	}
	s := copySnapshot(state)
	s.Turns = turns
	return ApplyResult{State: s, Changed: true}
}

func applyFrameUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Frame == nil {
		return ApplyResult{State: state, Changed: false}
	}
	frames := make([]Frame, len(state.Frames))
	copy(frames, state.Frames)
	found := false
	for i, f := range frames {
		if f.ID == op.Frame.ID {
			frames[i] = *op.Frame
			found = true
			break
		}
	}
	if !found {
		frames = append(frames, *op.Frame)
	}
	s := copySnapshot(state)
	s.Frames = frames
	return ApplyResult{State: s, Changed: true}
}

func applyAppend(state *Snapshot, op Operation) ApplyResult {
	if op.FrameID == "" || op.Content == "" {
		return ApplyResult{State: state, Changed: false}
	}
	frames := make([]Frame, len(state.Frames))
	copy(frames, state.Frames)
	for i, f := range frames {
		if f.ID == op.FrameID {
			switch f.Kind {
			case FrameText:
				if f.Text != nil {
					t := *f.Text
					t.Content += op.Content
					frames[i].Text = &t
				}
			case FrameThinking:
				if f.Thinking != nil {
					t := *f.Thinking
					t.Content += op.Content
					frames[i].Thinking = &t
				}
			}
			break
		}
	}
	s := copySnapshot(state)
	s.Frames = frames
	return ApplyResult{State: s, Changed: true}
}

func applyItemUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Item == nil {
		return ApplyResult{State: state, Changed: false}
	}
	items := make([]TranscriptItem, len(state.Items))
	copy(items, state.Items)
	items = append(items, *op.Item)
	s := copySnapshot(state)
	s.Items = items
	return ApplyResult{State: s, Changed: true}
}

func applyTaskUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Task == nil {
		return ApplyResult{State: state, Changed: false}
	}
	tasks := make([]TranscriptTask, len(state.Tasks))
	copy(tasks, state.Tasks)
	found := false
	for i, t := range tasks {
		if t.ID == op.Task.ID {
			tasks[i] = *op.Task
			found = true
			break
		}
	}
	if !found {
		tasks = append(tasks, *op.Task)
	}
	s := copySnapshot(state)
	s.Tasks = tasks
	return ApplyResult{State: s, Changed: true}
}

func applyInteractionUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Interaction == nil {
		return ApplyResult{State: state, Changed: false}
	}
	interactions := make([]TranscriptInteraction, len(state.Interactions))
	copy(interactions, state.Interactions)
	found := false
	for i, in := range interactions {
		if in.ID == op.Interaction.ID {
			interactions[i] = *op.Interaction
			found = true
			break
		}
	}
	if !found {
		interactions = append(interactions, *op.Interaction)
	}
	s := copySnapshot(state)
	s.Interactions = interactions
	return ApplyResult{State: s, Changed: true}
}

func applyInteractionResolve(state *Snapshot, op Operation) ApplyResult {
	if op.InteractionID == "" {
		return ApplyResult{State: state, Changed: false}
	}
	interactions := make([]TranscriptInteraction, len(state.Interactions))
	copy(interactions, state.Interactions)
	now := time.Now()
	for i, in := range interactions {
		if in.ID == op.InteractionID {
			interactions[i].Resolved = true
			interactions[i].Response = op.Response
			interactions[i].ResolvedAt = &now
			break
		}
	}
	s := copySnapshot(state)
	s.Interactions = interactions
	return ApplyResult{State: s, Changed: true}
}

func applyAttachmentUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Attachment == nil {
		return ApplyResult{State: state, Changed: false}
	}
	attachments := make([]TranscriptAttachment, len(state.Attachments))
	copy(attachments, state.Attachments)
	found := false
	for i, a := range attachments {
		if a.ID == op.Attachment.ID {
			attachments[i] = *op.Attachment
			found = true
			break
		}
	}
	if !found {
		attachments = append(attachments, *op.Attachment)
	}
	s := copySnapshot(state)
	s.Attachments = attachments
	return ApplyResult{State: s, Changed: true}
}

func applyPromptUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Prompt == nil {
		return ApplyResult{State: state, Changed: false}
	}
	prompts := make([]TranscriptPrompt, len(state.Prompts))
	copy(prompts, state.Prompts)
	found := false
	for i, p := range prompts {
		if p.ID == op.Prompt.ID {
			prompts[i] = *op.Prompt
			found = true
			break
		}
	}
	if !found {
		prompts = append(prompts, *op.Prompt)
	}
	s := copySnapshot(state)
	s.Prompts = prompts
	return ApplyResult{State: s, Changed: true}
}

func applyMetaMerge(state *Snapshot, op Operation) ApplyResult {
	if op.MetaPatch == nil {
		return ApplyResult{State: state, Changed: false}
	}
	meta := state.Meta // copy
	if op.MetaPatch.Goal != nil {
		meta.Goal = op.MetaPatch.Goal
	}
	if op.MetaPatch.Modes != nil {
		meta.Modes = op.MetaPatch.Modes
	}
	if op.MetaPatch.AgentPhase != nil {
		meta.AgentPhase = op.MetaPatch.AgentPhase
	}
	if op.MetaPatch.AgentStatus != nil {
		meta.AgentStatus = op.MetaPatch.AgentStatus
	}
	if op.MetaPatch.Model != "" {
		meta.Model = op.MetaPatch.Model
	}
	if op.MetaPatch.Provider != "" {
		meta.Provider = op.MetaPatch.Provider
	}
	if op.MetaPatch.Custom != nil {
		newCustom := make(map[string]any, len(meta.Custom)+len(op.MetaPatch.Custom))
		for k, v := range meta.Custom {
			newCustom[k] = v
		}
		for k, v := range op.MetaPatch.Custom {
			newCustom[k] = v
		}
		meta.Custom = newCustom
	}
	s := copySnapshot(state)
	s.Meta = meta
	return ApplyResult{State: s, Changed: true}
}

func applyItemsRemove(state *Snapshot, op Operation) ApplyResult {
	if len(op.ItemIndices) == 0 {
		return ApplyResult{State: state, Changed: false}
	}
	removeSet := make(map[int]bool, len(op.ItemIndices))
	for _, idx := range op.ItemIndices {
		removeSet[idx] = true
	}
	items := make([]TranscriptItem, 0, len(state.Items))
	for i, item := range state.Items {
		if !removeSet[i] {
			items = append(items, item)
		}
	}
	s := copySnapshot(state)
	s.Items = items
	return ApplyResult{State: s, Changed: true}
}

// ── Copy helpers ──

func copySnapshot(state *Snapshot) *Snapshot {
	s := &Snapshot{
		Turns:        make([]TranscriptTurn, len(state.Turns)),
		Frames:       make([]Frame, len(state.Frames)),
		Interactions: make([]TranscriptInteraction, len(state.Interactions)),
		Attachments:  make([]TranscriptAttachment, len(state.Attachments)),
		Todos:        state.Todos,
		Tasks:        make([]TranscriptTask, len(state.Tasks)),
		Items:        make([]TranscriptItem, len(state.Items)),
		Prompts:      make([]TranscriptPrompt, len(state.Prompts)),
		Meta:         state.Meta,
	}
	copy(s.Turns, state.Turns)
	// Deep-copy each turn's Steps slice to avoid sharing backing arrays.
	for i := range s.Turns {
		s.Turns[i].Steps = make([]TranscriptStep, len(state.Turns[i].Steps))
		copy(s.Turns[i].Steps, state.Turns[i].Steps)
	}
	copy(s.Frames, state.Frames)
	copy(s.Interactions, state.Interactions)
	copy(s.Attachments, state.Attachments)
	copy(s.Tasks, state.Tasks)
	copy(s.Items, state.Items)
	copy(s.Prompts, state.Prompts)
	return s
}

func copyWith(state *Snapshot, fn func(*Snapshot)) *Snapshot {
	s := copySnapshot(state)
	fn(s)
	return s
}
