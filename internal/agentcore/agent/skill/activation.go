package skill

import (
	"fmt"
	"time"
)

// ActivationEvent records when a skill is activated.
type ActivationEvent struct {
	SkillName string    `json:"skill_name"`
	Source    string    `json:"source"` // "user_slash", "model_tool", "system"
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// ActivationManager tracks skill activations and provides lifecycle management.
type ActivationManager struct {
	catalog    *Catalog
	history    []ActivationEvent
	maxHistory int
	onActivate func(event ActivationEvent)
}

// NewActivationManager creates an activation manager with a catalog.
func NewActivationManager(catalog *Catalog, onActivate func(ActivationEvent)) *ActivationManager {
	return &ActivationManager{
		catalog:    catalog,
		maxHistory: 1000,
		onActivate: onActivate,
	}
}

// Activate records a skill activation event and returns the skill.
func (m *ActivationManager) Activate(name, source string) (*Skill, error) {
	if m.catalog == nil {
		return nil, fmt.Errorf("no skill catalog available")
	}

	sk := m.catalog.Get(name)
	if sk == nil {
		event := ActivationEvent{
			SkillName: name,
			Source:    source,
			Timestamp: time.Now(),
			Success:   false,
			Error:     "skill not found",
		}
		m.record(event)
		return nil, fmt.Errorf("skill not found: %s", name)
	}

	event := ActivationEvent{
		SkillName: name,
		Source:    source,
		Timestamp: time.Now(),
		Success:   true,
	}
	m.record(event)

	return sk, nil
}

// History returns recent activation events.
func (m *ActivationManager) History(limit int) []ActivationEvent {
	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}
	start := len(m.history) - limit
	return m.history[start:]
}

// LastActivation returns the most recent activation for a skill.
func (m *ActivationManager) LastActivation(name string) *ActivationEvent {
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].SkillName == name {
			event := m.history[i]
			return &event
		}
	}
	return nil
}

// ActivationCount returns how many times a skill has been activated.
func (m *ActivationManager) ActivationCount(name string) int {
	count := 0
	for _, e := range m.history {
		if e.SkillName == name && e.Success {
			count++
		}
	}
	return count
}

func (m *ActivationManager) record(event ActivationEvent) {
	if len(m.history) >= m.maxHistory {
		m.history = m.history[1:]
	}
	m.history = append(m.history, event)

	if m.onActivate != nil {
		m.onActivate(event)
	}
}

// Catalog returns the underlying catalog.
func (m *ActivationManager) Catalog() *Catalog {
	return m.catalog
}

// Refresh re-discovers skills from the project root.
func (m *ActivationManager) Refresh(projectRoot string) error {
	catalog, err := Discover(projectRoot)
	if err != nil {
		return err
	}
	m.catalog = catalog
	return nil
}

// AvailableSkills returns skill names that can be model-invoked.
func (m *ActivationManager) AvailableSkills() []string {
	if m.catalog == nil {
		return nil
	}
	var result []string
	for _, sk := range m.catalog.List() {
		if !sk.DisableModelInvocation {
			result = append(result, sk.Name)
		}
	}
	return result
}

// SkillSummary returns a summary of all skills for system prompt injection.
func (m *ActivationManager) SkillSummary() string {
	if m.catalog == nil || m.catalog.Len() == 0 {
		return ""
	}

	var sb []string
	for _, sk := range m.catalog.List() {
		if sk.IsUserActivatable() {
			desc := sk.Description
			if sk.WhenToUse != "" {
				desc = sk.WhenToUse
			}
			sb = append(sb, fmt.Sprintf("- **%s**: %s", sk.SlashName(), desc))
		}
	}

	if len(sb) == 0 {
		return ""
	}

	return "Available skills:\n" + joinLines(sb)
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}
