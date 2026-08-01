package transcript

import (
	"fmt"
	"time"
)

// TurnGroup represents a group of consecutive turns sharing a common context.
type TurnGroup struct {
	Label     string           `json:"label"`
	Turns     []TranscriptTurn `json:"turns"`
	StartTime time.Time        `json:"startTime"`
	EndTime   time.Time        `json:"endTime"`
}

// GroupTurns groups consecutive turns by time gap. Turns separated by
// more than gapDuration are placed in different groups.
func GroupTurns(turns []TranscriptTurn, gapDuration time.Duration) []TurnGroup {
	if len(turns) == 0 {
		return nil
	}

	var groups []TurnGroup
	current := TurnGroup{
		Turns:     []TranscriptTurn{turns[0]},
		StartTime: turns[0].CreatedAt,
		EndTime:   turnEndTime(turns[0]),
	}

	for i := 1; i < len(turns); i++ {
		prev := turns[i-1]
		curr := turns[i]

		gap := curr.CreatedAt.Sub(turnEndTime(prev))
		if gap > gapDuration {
			// Finalize current group
			current.Label = groupLabel(current)
			groups = append(groups, current)
			// Start new group
			current = TurnGroup{
				Turns:     []TranscriptTurn{curr},
				StartTime: curr.CreatedAt,
				EndTime:   turnEndTime(curr),
			}
		} else {
			current.Turns = append(current.Turns, curr)
			current.EndTime = turnEndTime(curr)
		}
	}

	current.Label = groupLabel(current)
	groups = append(groups, current)
	return groups
}

// FoldFacts extracts key facts from a set of turns for compaction.
// Returns a summary of user prompts and assistant responses.
func FoldFacts(turns []TranscriptTurn, frames []Frame) string {
	if len(turns) == 0 {
		return ""
	}

	var result string
	for _, turn := range turns {
		// Extract user prompt from origin
		if turn.Origin != nil && turn.Origin.Prompt != "" {
			prompt := turn.Origin.Prompt
			if len(prompt) > 200 {
				prompt = prompt[:200] + "..."
			}
			result += "User: " + prompt + "\n"
		}

		// Extract assistant text frames for this turn's steps
		stepIDs := make(map[StepID]bool)
		for _, step := range turn.Steps {
			stepIDs[step.ID] = true
		}
		for _, frame := range frames {
			if !stepIDs[frame.StepID] {
				continue
			}
			if frame.Kind == FrameText && frame.Text != nil {
				text := frame.Text.Content
				if len(text) > 300 {
					text = text[:300] + "..."
				}
				result += "Assistant: " + text + "\n"
			}
		}
	}

	return result
}

// FilterFramesByTurn returns only frames belonging to the specified turn.
func FilterFramesByTurn(frames []Frame, turnID TurnID, steps []TranscriptStep) []Frame {
	stepIDs := make(map[StepID]bool)
	for _, step := range steps {
		if step.TurnID == turnID {
			stepIDs[step.ID] = true
		}
	}
	var result []Frame
	for _, f := range frames {
		if stepIDs[f.StepID] {
			result = append(result, f)
		}
	}
	return result
}

// FilterFramesByStep returns only frames belonging to the specified step.
func FilterFramesByStep(frames []Frame, stepID StepID) []Frame {
	var result []Frame
	for _, f := range frames {
		if f.StepID == stepID {
			result = append(result, f)
		}
	}
	return result
}

// ── Helpers ──

func turnEndTime(turn TranscriptTurn) time.Time {
	if turn.FinishedAt != nil {
		return *turn.FinishedAt
	}
	if len(turn.Steps) > 0 {
		last := turn.Steps[len(turn.Steps)-1]
		if last.Timing != nil && !last.Timing.FinishedAt.IsZero() {
			return last.Timing.FinishedAt
		}
	}
	return turn.CreatedAt
}

func groupLabel(group TurnGroup) string {
	if len(group.Turns) == 1 {
		if group.Turns[0].Origin != nil && group.Turns[0].Origin.Prompt != "" {
			prompt := group.Turns[0].Origin.Prompt
			if len(prompt) > 50 {
				prompt = prompt[:50] + "..."
			}
			return prompt
		}
		return "Single turn"
	}
	return group.Turns[0].CreatedAt.Format("15:04") + " \u2014 " +
		group.Turns[len(group.Turns)-1].CreatedAt.Format("15:04") +
		fmt.Sprintf(" (%d turns)", len(group.Turns))
}
