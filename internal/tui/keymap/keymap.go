// Package keymap provides configurable keybindings for the TUI.
package keymap

// Action identifies a TUI action that can be bound to a key.
type Action string

const (
	ActionSend        Action = "send"
	ActionCancel      Action = "cancel"
	ActionNewSession  Action = "new_session"
	ActionSessionList Action = "session_list"
	ActionScrollUp    Action = "scroll_up"
	ActionScrollDown  Action = "scroll_down"
	ActionPageUp      Action = "page_up"
	ActionPageDown    Action = "page_down"
	ActionCopy        Action = "copy"
	ActionPaste       Action = "paste"
	ActionTogglePlan  Action = "toggle_plan"
	ActionToggleMode  Action = "toggle_mode"
	ActionAbort       Action = "abort"
	ActionUndo        Action = "undo"
	ActionCompact     Action = "compact"
	ActionExport      Action = "export"
	ActionHelp        Action = "help"
	ActionQuit        Action = "quit"
	ActionTab         Action = "tab"
	ActionEscape      Action = "escape"
	ActionEnter       Action = "enter"
)

// Binding maps a key to an action with a description.
type Binding struct {
	Key         string `json:"key"`
	Action      Action `json:"action"`
	Description string `json:"description"`
}

// Keymap holds the complete set of keybindings.
type Keymap struct {
	bindings map[Action]Binding
	byKey    map[string]Action
}

// DefaultKeymap returns the default keybinding configuration.
func DefaultKeymap() *Keymap {
	bindings := []Binding{
		{Key: "enter", Action: ActionSend, Description: "Send message"},
		{Key: "ctrl+c", Action: ActionCancel, Description: "Cancel current operation"},
		{Key: "ctrl+n", Action: ActionNewSession, Description: "New session"},
		{Key: "ctrl+p", Action: ActionSessionList, Description: "Session picker"},
		{Key: "up", Action: ActionScrollUp, Description: "Scroll up"},
		{Key: "down", Action: ActionScrollDown, Description: "Scroll down"},
		{Key: "pgup", Action: ActionPageUp, Description: "Page up"},
		{Key: "pgdown", Action: ActionPageDown, Description: "Page down"},
		{Key: "ctrl+y", Action: ActionCopy, Description: "Copy selection"},
		{Key: "ctrl+shift+plan", Action: ActionTogglePlan, Description: "Toggle plan mode"},
		{Key: "ctrl+shift+mode", Action: ActionToggleMode, Description: "Toggle permission mode"},
		{Key: "ctrl+a", Action: ActionAbort, Description: "Abort running task"},
		{Key: "ctrl+z", Action: ActionUndo, Description: "Undo last message"},
		{Key: "ctrl+k", Action: ActionCompact, Description: "Compact context"},
		{Key: "ctrl+e", Action: ActionExport, Description: "Export session"},
		{Key: "ctrl+h", Action: ActionHelp, Description: "Show help"},
		{Key: "ctrl+q", Action: ActionQuit, Description: "Quit"},
		{Key: "tab", Action: ActionTab, Description: "Tab complete"},
		{Key: "esc", Action: ActionEscape, Description: "Escape/clear"},
	}

	km := &Keymap{
		bindings: make(map[Action]Binding),
		byKey:    make(map[string]Action),
	}
	for _, b := range bindings {
		km.bindings[b.Action] = b
		km.byKey[b.Key] = b.Action
	}
	return km
}

// Get returns the binding for an action.
func (k *Keymap) Get(action Action) (Binding, bool) {
	b, ok := k.bindings[action]
	return b, ok
}

// Lookup returns the action for a key.
func (k *Keymap) Lookup(key string) (Action, bool) {
	a, ok := k.byKey[key]
	return a, ok
}

// Set overrides a keybinding.
func (k *Keymap) Set(binding Binding) {
	// Remove old key mapping
	if old, ok := k.bindings[binding.Action]; ok {
		delete(k.byKey, old.Key)
	}
	k.bindings[binding.Action] = binding
	k.byKey[binding.Key] = binding.Action
}

// Bindings returns all bindings.
func (k *Keymap) Bindings() []Binding {
	result := make([]Binding, 0, len(k.bindings))
	for _, b := range k.bindings {
		result = append(result, b)
	}
	return result
}

// MatchKey checks if a key string matches an action's bound key.
func (k *Keymap) MatchKey(key string, action Action) bool {
	b, ok := k.bindings[action]
	if !ok {
		return false
	}
	return key == b.Key
}
