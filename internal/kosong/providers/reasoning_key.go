// Package providers implements LLM provider adapters.
// reasoning_key.go provides reasoning key dialect detection.
package providers

import "sync"

// Known reasoning keys in priority order.
var knownReasoningKeys = []string{
	"reasoning_content",
	"reasoning_details",
	"reasoning",
}

// DefaultReasoningKey is the fallback when no dialect is detected.
const DefaultReasoningKey = "reasoning_content"

// ReasoningKeyDialect detects which reasoning key a provider uses
// and echoes it on outbound requests. The dialect is learned from
// inbound responses and steers the next request.
type ReasoningKeyDialect struct {
	mu          sync.RWMutex
	explicitKey string // pinned key (disables detection)
	detected    string // last observed key
}

// NewReasoningKeyDialect creates a dialect detector. If explicitKey is
// non-empty, detection is disabled and that key is always used.
func NewReasoningKeyDialect(explicitKey string) *ReasoningKeyDialect {
	return &ReasoningKeyDialect{explicitKey: explicitKey}
}

// Observe scans a source map for the first known reasoning key with a
// string value. It remembers which key was found and returns the
// reasoning text and whether it was found.
func (d *ReasoningKeyDialect) Observe(source map[string]interface{}) (string, bool) {
	for _, key := range knownReasoningKeys {
		if v, ok := source[key]; ok {
			if s, isStr := v.(string); isStr && s != "" {
				d.mu.Lock()
				d.detected = key
				d.mu.Unlock()
				return s, true
			}
		}
	}
	return "", false
}

// ObserveDelta is like Observe but works with delta objects that may
// only contain one reasoning key at a time.
func (d *ReasoningKeyDialect) ObserveDelta(delta map[string]interface{}) (string, bool) {
	return d.Observe(delta)
}

// OutboundKey returns the key to use on outbound requests.
// Priority: explicit > detected > default.
func (d *ReasoningKeyDialect) OutboundKey() string {
	if d.explicitKey != "" {
		return d.explicitKey
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.detected != "" {
		return d.detected
	}
	return DefaultReasoningKey
}

// Detected returns the last detected key, or empty if none.
func (d *ReasoningKeyDialect) Detected() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.detected
}

// Reset clears the detected key.
func (d *ReasoningKeyDialect) Reset() {
	d.mu.Lock()
	d.detected = ""
	d.mu.Unlock()
}

// ExtractReasoning scans an object for the first known reasoning key
// with a string value. Returns the key and value, or empty strings.
func ExtractReasoning(source map[string]interface{}) (key, value string) {
	for _, k := range knownReasoningKeys {
		if v, ok := source[k]; ok {
			if s, isStr := v.(string); isStr && s != "" {
				return k, s
			}
		}
	}
	return "", ""
}
