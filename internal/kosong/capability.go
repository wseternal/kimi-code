package kosong

// ModelCapability describes the capabilities of a specific model.
// MaxContextTokens of 0 means "unknown".
type ModelCapability struct {
	ImageIn                  bool `json:"imageIn"`
	VideoIn                  bool `json:"videoIn"`
	AudioIn                  bool `json:"audioIn"`
	Thinking                 bool `json:"thinking"`
	ToolUse                  bool `json:"toolUse"`
	MaxContextTokens         int  `json:"maxContextTokens"`
	MaxInputTokens           int  `json:"maxInputTokens,omitempty"`
	DynamicallyLoadedTools   bool `json:"dynamicallyLoadedTools,omitempty"`
}

// UnknownCapability is the default returned when a model is not catalogued.
var UnknownCapability = ModelCapability{}

// IsUnknownCapability returns true if the capability has no declared features.
func IsUnknownCapability(mc ModelCapability) bool {
	return !mc.ImageIn && !mc.VideoIn && !mc.AudioIn &&
		!mc.Thinking && !mc.ToolUse && !mc.DynamicallyLoadedTools &&
		mc.MaxContextTokens == 0
}
