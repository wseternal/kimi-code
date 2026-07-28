// Package permission implements a policy chain for tool call approval.
//
// Each Policy evaluates a tool call and returns a Decision: Allow, Deny,
// or Ask (defer to the next policy or the user). The Chain evaluates
// policies in order and returns the first definitive (non-Ask) answer,
// falling back to the final Ask if no policy decides.
package permission

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Decision is the outcome of a policy evaluation.
type Decision int

const (
	DecisionAsk   Decision = iota // no opinion — defer to next policy or user
	DecisionAllow                 // explicitly allowed
	DecisionDeny                  // explicitly denied
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	default:
		return "ask"
	}
}

// Result is the outcome of a policy evaluation with metadata.
type Result struct {
	Decision Decision
	Reason   string
	Policy   string
}

// Policy evaluates whether a tool call is permitted.
type Policy interface {
	// Name returns the policy's identifier.
	Name() string
	// Evaluate returns a decision for the given tool call.
	Evaluate(toolName string, input json.RawMessage) Result
}

// Chain evaluates policies in order. The first non-Ask decision wins.
// If all policies return Ask, the chain returns the final Ask.
type Chain struct {
	policies []Policy
}

// NewChain creates a policy chain from the given policies, evaluated in order.
func NewChain(policies ...Policy) *Chain {
	return &Chain{policies: policies}
}

// Evaluate runs the chain and returns the first definitive decision.
func (c *Chain) Evaluate(toolName string, input json.RawMessage) Result {
	var lastAsk Result
	for _, p := range c.policies {
		r := p.Evaluate(toolName, input)
		if r.Decision != DecisionAsk {
			return r
		}
		lastAsk = r
	}
	if lastAsk.Policy == "" {
		return Result{Decision: DecisionAllow} // empty chain = permissive
	}
	return lastAsk
}

// DefaultChain returns a policy chain for normal (non-yolo) mode.
// Sensitive files are denied, everything else asks the user.
func DefaultChain() *Chain {
	return NewChain(
		NewSensitiveFilePolicy(),
		NewFallbackAskPolicy(),
	)
}

// YoloChain returns a policy chain that auto-approves everything.
func YoloChain() *Chain {
	return NewChain(NewAutoApprovePolicy())
}

// ── Built-in Policies ────────────────────────────────────────────────

// AutoApprovePolicy allows everything.
type AutoApprovePolicy struct{}

func NewAutoApprovePolicy() *AutoApprovePolicy { return &AutoApprovePolicy{} }

func (p *AutoApprovePolicy) Name() string { return "auto-approve" }

func (p *AutoApprovePolicy) Evaluate(_ string, _ json.RawMessage) Result {
	return Result{Decision: DecisionAllow, Reason: "auto-approved", Policy: p.Name()}
}

// AutoDenyPolicy denies everything with a fixed reason.
type AutoDenyPolicy struct {
	reason string
}

func NewAutoDenyPolicy(reason string) *AutoDenyPolicy {
	return &AutoDenyPolicy{reason: reason}
}

func (p *AutoDenyPolicy) Name() string { return "auto-deny" }

func (p *AutoDenyPolicy) Evaluate(_ string, _ json.RawMessage) Result {
	return Result{Decision: DecisionDeny, Reason: p.reason, Policy: p.Name()}
}

// FallbackAskPolicy always returns Ask, used as the last policy in a chain.
type FallbackAskPolicy struct{}

func NewFallbackAskPolicy() *FallbackAskPolicy { return &FallbackAskPolicy{} }

func (p *FallbackAskPolicy) Name() string { return "fallback-ask" }

func (p *FallbackAskPolicy) Evaluate(_ string, _ json.RawMessage) Result {
	return Result{Decision: DecisionAsk, Reason: "user approval required", Policy: p.Name()}
}

// SensitiveFilePolicy denies Read/Write/Edit operations on sensitive files.
// Non-file tools and non-sensitive paths return Allow (pass-through).
type SensitiveFilePolicy struct{}

func NewSensitiveFilePolicy() *SensitiveFilePolicy { return &SensitiveFilePolicy{} }

func (p *SensitiveFilePolicy) Name() string { return "sensitive-file" }

// fileTools maps tool names to the JSON key holding the file path.
var fileToolPathKeys = map[string]string{
	"Read":  "path",
	"Write": "path",
	"Edit":  "path",
}

func (p *SensitiveFilePolicy) Evaluate(toolName string, input json.RawMessage) Result {
	pathKey, isFileTool := fileToolPathKeys[toolName]
	if !isFileTool {
		// Not a file-access tool — no opinion, defer to next policy
		return Result{Decision: DecisionAsk, Policy: p.Name()}
	}

	var args map[string]json.RawMessage
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{Decision: DecisionAllow, Policy: p.Name()}
	}

	rawPath, ok := args[pathKey]
	if !ok {
		return Result{Decision: DecisionAllow, Policy: p.Name()}
	}

	var path string
	if err := json.Unmarshal(rawPath, &path); err != nil {
		return Result{Decision: DecisionAllow, Policy: p.Name()}
	}

	if IsSensitiveFile(path) {
		return Result{
			Decision: DecisionDeny,
			Reason:   "access to sensitive file is blocked: " + path,
			Policy:   p.Name(),
		}
	}

	return Result{Decision: DecisionAllow, Policy: p.Name()}
}

// ── Sensitive File Detection ─────────────────────────────────────────

var sensitiveBasenames = map[string]bool{
	".env":        true,
	"id_rsa":      true,
	"id_ed25519":  true,
	"id_ecdsa":    true,
	"credentials": true,
}

var envExemptions = map[string]bool{
	".env.example":  true,
	".env.sample":   true,
	".env.template": true,
}

var publicKeyBasenames = map[string]bool{
	"id_rsa.pub":     true,
	"id_ed25519.pub": true,
	"id_ecdsa.pub":   true,
}

var sensitivePrefixes = []string{"id_rsa", "id_ed25519", "id_ecdsa", "credentials"}

var sensitiveDotSuffixes = map[string]bool{
	".bak": true, ".backup": true, ".copy": true, ".disabled": true,
	".key": true, ".old": true, ".orig": true, ".pem": true,
	".save": true, ".tmp": true,
}

var sensitivePathSuffixes = []string{
	".aws/credentials",
	".gcp/credentials",
}

// IsSensitiveFile reports whether the given path refers to a sensitive file
// (credentials, private keys, env secrets).
func IsSensitiveFile(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	lowerPath := strings.ToLower(path)

	// Exemptions
	if envExemptions[lower] {
		return false
	}
	if publicKeyBasenames[lower] {
		return false
	}

	// Exact matches
	if sensitiveBasenames[lower] {
		return true
	}

	// .env.* variants
	if strings.HasPrefix(lower, ".env.") {
		return true
	}

	// Prefix-based matches (id_rsa.bak, credentials.old, etc.)
	for _, prefix := range sensitivePrefixes {
		if lower == prefix {
			return true
		}
		if len(lower) > len(prefix) && strings.HasPrefix(lower, prefix) {
			suffix := lower[len(prefix):]
			sep := suffix[0]
			if sep == '-' || sep == '_' {
				return true
			}
			if sep == '.' && sensitiveDotSuffixes[suffix] {
				return true
			}
		}
	}

	// Path suffix matches
	for _, suf := range sensitivePathSuffixes {
		if strings.HasSuffix(lowerPath, "/"+suf) || strings.Contains(lowerPath, "/"+suf+"/") {
			return true
		}
	}

	return false
}
