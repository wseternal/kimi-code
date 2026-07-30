package protocol

// PromptOriginVariant identifies the source of a prompt submission.
type PromptOriginVariant string

const (
	PromptOriginUser             PromptOriginVariant = "user"
	PromptOriginSkillActivation  PromptOriginVariant = "skill_activation"
	PromptOriginPluginCommand    PromptOriginVariant = "plugin_command"
	PromptOriginInjection        PromptOriginVariant = "injection"
	PromptOriginShellCommand     PromptOriginVariant = "shell_command"
	PromptOriginGoalContinuation PromptOriginVariant = "goal_continuation"
	PromptOriginBackgroundTask   PromptOriginVariant = "background_task"
	PromptOriginSubagent         PromptOriginVariant = "subagent"
	PromptOriginAPI              PromptOriginVariant = "api"
	PromptOriginCron             PromptOriginVariant = "cron"
	PromptOriginSteering         PromptOriginVariant = "steering"
	PromptOriginSystemTrigger    PromptOriginVariant = "system_trigger"
	PromptOriginCompaction       PromptOriginVariant = "compaction"
)

// PromptOrigin is a discriminated union tagging each prompt with its origin.
type PromptOrigin struct {
	Variant PromptOriginVariant `json:"variant"`

	// Skill activation context
	SkillName    string `json:"skill_name,omitempty"`
	SkillVersion string `json:"skill_version,omitempty"`

	// Plugin context
	PluginID string `json:"plugin_id,omitempty"`

	// Shell command context
	ShellCommand string `json:"shell_command,omitempty"`

	// Goal context
	GoalID string `json:"goal_id,omitempty"`

	// Background task context
	TaskID   string `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`

	// Sub-agent context
	ParentAgentID string `json:"parent_agent_id,omitempty"`

	// API context
	APIEndpoint string `json:"api_endpoint,omitempty"`

	// Cron context
	CronExpression string `json:"cron_expression,omitempty"`

	// System trigger context
	TriggerType string `json:"trigger_type,omitempty"`
}

// IsAutomated reports whether the prompt was generated automatically
// (not directly by the user).
func (o *PromptOrigin) IsAutomated() bool {
	if o == nil {
		return false
	}
	switch o.Variant {
	case PromptOriginUser, PromptOriginAPI:
		return false
	default:
		return true
	}
}
