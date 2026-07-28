package cli

import (
	"fmt"
	"sort"
	"strings"
)

// SlashCommandDef defines a slash command with metadata.
type SlashCommandDef struct {
	Name        string
	Desc        string
	Usage       string // e.g. "/title <name>"
	Group       string // "session", "agent", "config", "info", "utility"
	Handler     func(m *tuiModel, args string) (string, bool) // returns message, handled
}

// CommandRegistry holds all registered slash commands.
type CommandRegistry struct {
	commands []SlashCommandDef
	byName   map[string]*SlashCommandDef
}

// NewCommandRegistry creates a registry with all built-in commands.
func NewCommandRegistry() *CommandRegistry {
	r := &CommandRegistry{
		byName: make(map[string]*SlashCommandDef),
	}
	r.registerDefaults()
	return r
}

// Register adds a command to the registry.
func (r *CommandRegistry) Register(cmd SlashCommandDef) {
	r.commands = append(r.commands, cmd)
	r.byName[cmd.Name] = &r.commands[len(r.commands)-1]
}

// Get returns a command by name.
func (r *CommandRegistry) Get(name string) *SlashCommandDef {
	return r.byName[name]
}

// All returns all registered commands.
func (r *CommandRegistry) All() []SlashCommandDef {
	return r.commands
}

// Match returns commands matching the given prefix.
func (r *CommandRegistry) Match(prefix string) []SlashCommandDef {
	prefix = strings.ToLower(prefix)
	var matches []SlashCommandDef
	for _, cmd := range r.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// Groups returns commands organized by group.
func (r *CommandRegistry) Groups() map[string][]SlashCommandDef {
	groups := make(map[string][]SlashCommandDef)
	for _, cmd := range r.commands {
		g := cmd.Group
		if g == "" {
			g = "other"
		}
		groups[g] = append(groups[g], cmd)
	}
	// Sort each group
	for _, cmds := range groups {
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name < cmds[j].Name
		})
	}
	return groups
}

// toSlashCommands converts registry commands to the legacy slashCommand format.
func (r *CommandRegistry) toSlashCommands() []slashCommand {
	var out []slashCommand
	for _, cmd := range r.commands {
		out = append(out, slashCommand{name: cmd.Name, desc: cmd.Desc})
	}
	return out
}

func (r *CommandRegistry) registerDefaults() {
	// Session commands
	r.Register(SlashCommandDef{Name: "sessions", Desc: "Browse and resume sessions", Group: "session"})
	r.Register(SlashCommandDef{Name: "fork", Desc: "Fork current session", Group: "session"})
	r.Register(SlashCommandDef{Name: "title", Desc: "Set session title", Usage: "/title <name>", Group: "session"})
	r.Register(SlashCommandDef{Name: "new", Desc: "Create a new session", Group: "session"})
	r.Register(SlashCommandDef{Name: "clear", Desc: "Clear conversation display", Group: "session"})
	r.Register(SlashCommandDef{Name: "init", Desc: "Reset session to clean state", Group: "session"})
	r.Register(SlashCommandDef{Name: "undo", Desc: "Remove last N turns", Usage: "/undo [N]", Group: "session"})
	r.Register(SlashCommandDef{Name: "compact", Desc: "Compact conversation history", Group: "session"})
	r.Register(SlashCommandDef{Name: "export-md", Desc: "Export conversation as markdown", Group: "session"})

	// Agent commands
	r.Register(SlashCommandDef{Name: "goal", Desc: "Set autonomous goal", Usage: "/goal <text>", Group: "agent"})
	r.Register(SlashCommandDef{Name: "swarm", Desc: "Toggle swarm mode", Group: "agent"})
	r.Register(SlashCommandDef{Name: "btw", Desc: "Side query without context impact", Usage: "/btw <prompt>", Group: "agent"})

	// Config commands
	r.Register(SlashCommandDef{Name: "provider", Desc: "Show/switch provider", Usage: "/provider [name]", Group: "config"})
	r.Register(SlashCommandDef{Name: "model", Desc: "Switch LLM model", Group: "config"})
	r.Register(SlashCommandDef{Name: "experiments", Desc: "List experimental flags", Group: "config"})
	r.Register(SlashCommandDef{Name: "reload", Desc: "Reload config.toml", Group: "config"})
	r.Register(SlashCommandDef{Name: "auto", Desc: "Toggle YOLO mode", Group: "config"})
	r.Register(SlashCommandDef{Name: "yolo", Desc: "Toggle YOLO mode (alias)", Group: "config"})
	r.Register(SlashCommandDef{Name: "editor", Desc: "Toggle external editor mode", Group: "config"})
	r.Register(SlashCommandDef{Name: "theme", Desc: "Switch/list themes", Usage: "/theme [name]", Group: "config"})
	r.Register(SlashCommandDef{Name: "effort", Desc: "Set thinking effort level", Usage: "/effort [low|medium|high]", Group: "config"})
	r.Register(SlashCommandDef{Name: "permission", Desc: "Select permission mode", Group: "config"})
	r.Register(SlashCommandDef{Name: "plan", Desc: "Toggle plan mode", Group: "config"})
	r.Register(SlashCommandDef{Name: "settings", Desc: "Show settings info", Group: "config"})

	// Info commands
	r.Register(SlashCommandDef{Name: "status", Desc: "Show session info", Group: "info"})
	r.Register(SlashCommandDef{Name: "usage", Desc: "Show token usage", Group: "info"})
	r.Register(SlashCommandDef{Name: "mcp", Desc: "List MCP connections", Group: "info"})
	r.Register(SlashCommandDef{Name: "plugins", Desc: "List loaded plugins", Group: "info"})
	r.Register(SlashCommandDef{Name: "feedback", Desc: "Open feedback URL", Group: "info"})
	r.Register(SlashCommandDef{Name: "version", Desc: "Show version info", Group: "info"})
	r.Register(SlashCommandDef{Name: "help", Desc: "Show help information", Group: "info"})

	// Utility commands
	r.Register(SlashCommandDef{Name: "copy", Desc: "Copy last response to clipboard", Group: "utility"})
	r.Register(SlashCommandDef{Name: "web", Desc: "Quick web search", Usage: "/web <query>", Group: "utility"})
	r.Register(SlashCommandDef{Name: "add-dir", Desc: "Add working directory", Usage: "/add-dir <path>", Group: "utility"})
	r.Register(SlashCommandDef{Name: "tasks", Desc: "List background tasks", Group: "utility"})
	r.Register(SlashCommandDef{Name: "doctor", Desc: "Run diagnostics", Group: "utility"})
	r.Register(SlashCommandDef{Name: "login", Desc: "Set up API key", Group: "utility"})
	r.Register(SlashCommandDef{Name: "logout", Desc: "Remove API key", Group: "utility"})
}

// renderHelp returns a formatted help string for all commands.
func (r *CommandRegistry) renderHelp() string {
	groups := r.Groups()
	var b strings.Builder
	b.WriteString("Available commands:\n\n")

	groupOrder := []string{"session", "agent", "config", "info", "utility"}
	groupLabels := map[string]string{
		"session": "Session",
		"agent":   "Agent",
		"config":  "Config",
		"info":    "Info",
		"utility": "Utility",
	}

	for _, g := range groupOrder {
		cmds, ok := groups[g]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("  %s:\n", groupLabels[g]))
		maxName := 0
		for _, cmd := range cmds {
			label := cmd.Name
			if cmd.Usage != "" {
				label = strings.TrimPrefix(cmd.Usage, "/")
			}
			if len(label) > maxName {
				maxName = len(label)
			}
		}
		for _, cmd := range cmds {
			label := cmd.Name
			if cmd.Usage != "" {
				label = strings.TrimPrefix(cmd.Usage, "/")
			}
			pad := strings.Repeat(" ", maxName-len(label)+2)
			b.WriteString(fmt.Sprintf("    /%s%s%s\n", label, pad, cmd.Desc))
		}
		b.WriteString("\n")
	}

	b.WriteString("Type / to see available commands. Skills: $skill-name or /skill:skill-name.")
	return b.String()
}
