package kosong

// Tool represents a tool that the model may invoke during generation.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Deferred    bool                   `json:"deferred,omitempty"`
}
