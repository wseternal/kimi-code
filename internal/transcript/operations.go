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
	// Use copySnapshot then modify in place to avoid double-copying (N1 fix).
	s := copySnapshot(state)
	found := false
	for i, t := range s.Turns {
		if t.ID == op.Turn.ID {
			s.Turns[i] = *op.Turn
			found = true
			break
		}
	}
	if !found {
		s.Turns = append(s.Turns, *op.Turn)
	}
	return ApplyResult{State: s, Changed: true}
}

func applyStepUpsert(state *Snapshot, op Operation) ApplyResult {
	if op.Step == nil {
		return ApplyResult{State: state, Changed: false}
	}
	turns := make([]TranscriptTurn, len(state.Turns))
	copy(turns, state.Turns)
	found := false
	for i, t := range turns {
		if t.ID == op.Step.TurnID {
			steps := make([]TranscriptStep, len(t.Steps))
			copy(steps, t.Steps)
			for j, st := range steps {
				if st.ID == op.Step.ID {
					steps[j] = *op.Step
					found = true
					break
				}
			}
			if !found {
				steps = append(steps, *op.Step)
				found = true
			}
			turns[i].Steps = steps
			break
		}
	}
	if !found {
		return ApplyResult{State: state, Changed: false}
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
	found := false
	for i, f := range frames {
		if f.ID == op.FrameID {
			found = true
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
	if !found {
		return ApplyResult{State: state, Changed: false}
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
	// W1 fix: find-and-replace by identity (TurnID + Marker/TaskRef match) before appending.
	found := false
	for i, item := range items {
		if item.TurnID == op.Item.TurnID &&
			((item.Marker != nil && op.Item.Marker != nil && item.Marker.Kind == op.Item.Marker.Kind) ||
				(item.TaskRef != nil && op.Item.TaskRef != nil && item.TaskRef.TaskID == op.Item.TaskRef.TaskID)) {
			items[i] = *op.Item
			found = true
			break
		}
	}
	if !found {
		items = append(items, *op.Item)
	}
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
	meta := state.Meta // shallow copy of struct
	// Deep-copy pointer fields to avoid aliasing the operation input.
	if op.MetaPatch.Goal != nil {
		g := *op.MetaPatch.Goal
		meta.Goal = &g
	}
	if op.MetaPatch.Modes != nil {
		m := *op.MetaPatch.Modes
		meta.Modes = &m
	}
	if op.MetaPatch.AgentPhase != nil {
		p := *op.MetaPatch.AgentPhase
		meta.AgentPhase = &p
	}
	if op.MetaPatch.AgentStatus != nil {
		a := *op.MetaPatch.AgentStatus
		meta.AgentStatus = &a
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
		Tasks:        make([]TranscriptTask, len(state.Tasks)),
		Items:        make([]TranscriptItem, len(state.Items)),
		Prompts:      make([]TranscriptPrompt, len(state.Prompts)),
		Meta:         state.Meta,
	}
	// Deep-copy Todos pointer to avoid sharing with original.
	if state.Todos != nil {
		todos := *state.Todos
		if state.Todos.Items != nil {
			todos.Items = make([]TodoItem, len(state.Todos.Items))
			copy(todos.Items, state.Todos.Items)
		}
		s.Todos = &todos
	}
	// Deep-copy Meta.Custom map to avoid sharing with original.
	if state.Meta.Custom != nil {
		custom := make(map[string]any, len(state.Meta.Custom))
		for k, v := range state.Meta.Custom {
			custom[k] = v
		}
		s.Meta.Custom = custom
	}
	// Deep-copy Meta pointer fields.
	if state.Meta.Goal != nil {
		g := *state.Meta.Goal
		s.Meta.Goal = &g
	}
	if state.Meta.Modes != nil {
		m := *state.Meta.Modes
		s.Meta.Modes = &m
	}
	if state.Meta.AgentPhase != nil {
		p := *state.Meta.AgentPhase
		s.Meta.AgentPhase = &p
	}
	if state.Meta.AgentStatus != nil {
		a := *state.Meta.AgentStatus
		s.Meta.AgentStatus = &a
	}
	// Deep-copy Turns with all pointer fields (C3 fix).
	copy(s.Turns, state.Turns)
	for i := range s.Turns {
		// Deep-copy Steps slice and each step's pointer fields.
		if state.Turns[i].Steps != nil {
			s.Turns[i].Steps = make([]TranscriptStep, len(state.Turns[i].Steps))
			copy(s.Turns[i].Steps, state.Turns[i].Steps)
			for j := range s.Turns[i].Steps {
				st := &s.Turns[i].Steps[j]
				if st.Usage != nil {
					u := *st.Usage
					st.Usage = &u
				}
				if st.Timing != nil {
					t := *st.Timing
					st.Timing = &t
				}
				if st.Retry != nil {
					r := *st.Retry
					st.Retry = &r
				}
			}
		}
		// Deep-copy Turn pointer fields.
		if state.Turns[i].Origin != nil {
			o := *state.Turns[i].Origin
			s.Turns[i].Origin = &o
		}
		if state.Turns[i].FinishedAt != nil {
			t := *state.Turns[i].FinishedAt
			s.Turns[i].FinishedAt = &t
		}
	}
	// Deep-copy Frames with all pointer fields (C3 fix).
	copy(s.Frames, state.Frames)
	for i := range s.Frames {
		f := &s.Frames[i]
		if f.Text != nil {
			t := *f.Text
			f.Text = &t
		}
		if f.Thinking != nil {
			t := *f.Thinking
			f.Thinking = &t
		}
		if f.ToolCall != nil {
			t := *f.ToolCall
			f.ToolCall = &t
		}
		if f.Notice != nil {
			n := *f.Notice
			f.Notice = &n
		}
	}
	// Deep-copy Interactions pointer fields.
	copy(s.Interactions, state.Interactions)
	for i := range s.Interactions {
		if state.Interactions[i].ResolvedAt != nil {
			t := *state.Interactions[i].ResolvedAt
			s.Interactions[i].ResolvedAt = &t
		}
	}
	copy(s.Attachments, state.Attachments)
	// Deep-copy Tasks pointer fields.
	copy(s.Tasks, state.Tasks)
	for i := range s.Tasks {
		if state.Tasks[i].EndedAt != nil {
			t := *state.Tasks[i].EndedAt
			s.Tasks[i].EndedAt = &t
		}
	}
	// Deep-copy Items pointer fields.
	copy(s.Items, state.Items)
	for i := range s.Items {
		if s.Items[i].Marker != nil {
			m := *s.Items[i].Marker
			s.Items[i].Marker = &m
		}
		if s.Items[i].TaskRef != nil {
			tr := *s.Items[i].TaskRef
			s.Items[i].TaskRef = &tr
		}
	}
	copy(s.Prompts, state.Prompts)
	return s
}

func copyWith(state *Snapshot, fn func(*Snapshot)) *Snapshot {
	s := copySnapshot(state)
	fn(s)
	return s
}
