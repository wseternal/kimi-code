// Package audit provides a BadgerDB-backed append-only audit trail and
// session persistence layer. Every LLM event and user action is recorded
// with timestamps, enabling faithful session replay and full traceability.
//
// The package is standalone — it does not depend on the existing session
// or persistence packages, and can be wired in as a drop-in replacement
// in a future cycle.
package audit
