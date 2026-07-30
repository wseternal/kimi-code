package protocol

import "fmt"

// ErrorCode integer constants.
//
// Namespaces:
//
//	0       success
//	4xxxx   client errors (HTTP-4xx analog)
//	5xxxx   daemon internal errors
//	6xxxx   tool runtime
//	7xxxx   LLM provider pass-through (msg = upstream text)
//	8xxxx   MCP server pass-through (msg = upstream text)
//	9xxxx   reserved
const (
	// Success
	ErrorCodeSuccess = 0

	// Validation / parsing
	ErrorCodeValidationFailed  = 40001
	ErrorCodeRequestMalformed  = 40002

	// Auth (401xx)
	ErrorCodeAuthProvisioningRequired = 40110
	ErrorCodeAuthTokenMissing         = 40111
	ErrorCodeAuthTokenUnauthorized    = 40112
	ErrorCodeAuthModelNotResolved     = 40113

	// Not found (404xx)
	ErrorCodeSessionNotFound   = 40401
	ErrorCodePromptNotFound    = 40402
	ErrorCodeMessageNotFound   = 40403
	ErrorCodeApprovalNotFound  = 40404
	ErrorCodeQuestionNotFound  = 40405
	ErrorCodeTaskNotFound      = 40406
	ErrorCodeFileNotFound      = 40407
	ErrorCodeMCPServerNotFound = 40408
	ErrorCodeFSPathNotFound    = 40409
	ErrorCodeWorkspaceNotFound = 40410
	ErrorCodeFSPermissionDenied = 40411
	ErrorCodeProviderNotFound  = 40412
	ErrorCodeModelNotFound     = 40413
	ErrorCodeTerminalNotFound  = 40414
	ErrorCodeSkillNotFound     = 40415
	ErrorCodeToolCallNotFound  = 40416

	// Conflict (409xx)
	ErrorCodeSessionExists            = 40901
	ErrorCodeSessionBusy              = 40921
	ErrorCodeApprovalAlreadyResolved = 40902
	ErrorCodePromptAlreadyCompleted = 40903
	ErrorCodeTaskAlreadyFinished    = 40904
	ErrorCodeMCPAlreadyConnected    = 40905
	ErrorCodeFSIsDirectory          = 40906
	ErrorCodeFSIsBinary             = 40907
	ErrorCodeFSGitUnavailable       = 40908
	ErrorCodeQuestionDismissed      = 40909
	ErrorCodeCompactionUnable       = 40910
	ErrorCodeSessionUndoUnavailable = 40911
	ErrorCodeSkillNotActivatable    = 40912
	ErrorCodeGoalAlreadyExists      = 40913
	ErrorCodeGoalNotFound           = 40914
	ErrorCodeGoalStatusInvalid      = 40915
	ErrorCodeGoalNotResumable       = 40916
	ErrorCodeGoalObjectiveEmpty     = 40917
	ErrorCodeGoalObjectiveTooLong   = 40918
	ErrorCodeFSAlreadyExists        = 40919
	ErrorCodeGoalUnsupportedAgent   = 40920

	// Gone / expired (410xx)
	ErrorCodeApprovalExpired = 41001
	ErrorCodeQuestionExpired = 41002
	ErrorCodeFileExpired     = 41003

	// Payload too large (413xx)
	ErrorCodeFileTooLarge         = 41301
	ErrorCodeFSTooLarge           = 41302
	ErrorCodeFSTooManyResults     = 41303
	ErrorCodeFSPathEscapesSession = 41304
	ErrorCodeFSGrepTimeout        = 41305

	// Rate limit (429xx)
	ErrorCodeFSWatchLimitExceeded = 42902

	// Server errors (5xxxx)
	ErrorCodeInternalError      = 50001
	ErrorCodePersistenceFailure = 50003

	// Tool errors (6xxxx)
	ErrorCodeToolExecutionFailed = 60001
	ErrorCodeToolNotAvailable    = 60002
)

// errorCodeReason maps every allocated ErrorCode to its dot-separated reason string.
var errorCodeReason = map[int]string{
	ErrorCodeSuccess:                 "success",
	ErrorCodeValidationFailed:        "validation.failed",
	ErrorCodeRequestMalformed:        "request.malformed",
	ErrorCodeAuthProvisioningRequired: "auth.provisioning_required",
	ErrorCodeAuthTokenMissing:         "auth.token_missing",
	ErrorCodeAuthTokenUnauthorized:    "auth.token_unauthorized",
	ErrorCodeAuthModelNotResolved:     "auth.model_not_resolved",
	ErrorCodeSessionNotFound:          "session.not_found",
	ErrorCodePromptNotFound:           "prompt.not_found",
	ErrorCodeMessageNotFound:          "message.not_found",
	ErrorCodeApprovalNotFound:         "approval.not_found",
	ErrorCodeQuestionNotFound:         "question.not_found",
	ErrorCodeTaskNotFound:             "task.not_found",
	ErrorCodeFileNotFound:             "file.not_found",
	ErrorCodeMCPServerNotFound:        "mcp.server_not_found",
	ErrorCodeFSPathNotFound:           "fs.path_not_found",
	ErrorCodeWorkspaceNotFound:        "workspace.not_found",
	ErrorCodeFSPermissionDenied:       "fs.permission_denied",
	ErrorCodeProviderNotFound:         "provider.not_found",
	ErrorCodeModelNotFound:            "model.not_found",
	ErrorCodeTerminalNotFound:         "terminal.not_found",
	ErrorCodeSkillNotFound:            "skill.not_found",
	ErrorCodeToolCallNotFound:         "tool_call.not_found",
	ErrorCodeSessionExists:            "session.exists",
	ErrorCodeSessionBusy:              "session.busy",
	ErrorCodeApprovalAlreadyResolved:  "approval.already_resolved",
	ErrorCodePromptAlreadyCompleted:   "prompt.already_completed",
	ErrorCodeTaskAlreadyFinished:      "task.already_finished",
	ErrorCodeMCPAlreadyConnected:      "mcp.already_connected",
	ErrorCodeFSIsDirectory:            "fs.is_directory",
	ErrorCodeFSIsBinary:               "fs.is_binary",
	ErrorCodeFSGitUnavailable:         "fs.git_unavailable",
	ErrorCodeQuestionDismissed:        "question.dismissed",
	ErrorCodeCompactionUnable:         "compaction.unable",
	ErrorCodeSessionUndoUnavailable:   "session.undo_unavailable",
	ErrorCodeSkillNotActivatable:      "skill.not_activatable",
	ErrorCodeGoalAlreadyExists:        "goal.already_exists",
	ErrorCodeGoalNotFound:             "goal.not_found",
	ErrorCodeGoalStatusInvalid:        "goal.status_invalid",
	ErrorCodeGoalNotResumable:         "goal.not_resumable",
	ErrorCodeGoalObjectiveEmpty:       "goal.objective_empty",
	ErrorCodeGoalObjectiveTooLong:     "goal.objective_too_long",
	ErrorCodeFSAlreadyExists:          "fs.already_exists",
	ErrorCodeGoalUnsupportedAgent:     "goal.unsupported_agent",
	ErrorCodeApprovalExpired:          "approval.expired",
	ErrorCodeQuestionExpired:          "question.expired",
	ErrorCodeFileExpired:              "file.expired",
	ErrorCodeFileTooLarge:             "file.too_large",
	ErrorCodeFSTooLarge:               "fs.too_large",
	ErrorCodeFSTooManyResults:         "fs.too_many_results",
	ErrorCodeFSPathEscapesSession:     "fs.path_escapes_session",
	ErrorCodeFSGrepTimeout:            "fs.grep_timeout",
	ErrorCodeFSWatchLimitExceeded:     "fs.watch_limit_exceeded",
	ErrorCodeInternalError:            "internal.error",
	ErrorCodePersistenceFailure:       "persistence.failure",
	ErrorCodeToolExecutionFailed:      "tool.execution_failed",
	ErrorCodeToolNotAvailable:         "tool.not_available",
}

// ErrorCodeReason returns the human-readable reason string for an error code.
// Returns "unknown" for unallocated codes.
func ErrorCodeReason(code int) string {
	if r, ok := errorCodeReason[code]; ok {
		return r
	}
	return "unknown"
}

// IsClientError reports whether the code is in the 4xxxx client-error range.
func IsClientError(code int) bool { return code >= 40000 && code < 50000 }

// IsServerError reports whether the code is in the 5xxxx server-error range.
func IsServerError(code int) bool { return code >= 50000 && code < 60000 }

// IsToolError reports whether the code is in the 6xxxx tool-error range.
func IsToolError(code int) bool { return code >= 60000 && code < 70000 }

// APIError is a typed Go error wrapping an error code.
// Implements the error interface so it can be used with errors.Is/As.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d %s] %s", e.Code, ErrorCodeReason(e.Code), e.Message)
}

// NewAPIError constructs an APIError.
func NewAPIError(code int, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

// KimiErrorCode is a typed semantic error code string.
// Maps to the TS string enum of ~75 codes.
type KimiErrorCode string

const (
	// Session errors
	KimiErrSessionNotFound      KimiErrorCode = "session.not_found"
	KimiErrSessionBusy          KimiErrorCode = "session.busy"
	KimiErrSessionForkActiveTurn KimiErrorCode = "session.fork_active_turn"
	KimiErrSessionUndoUnavailable KimiErrorCode = "session.undo_unavailable"

	// Provider errors
	KimiErrProviderRateLimit    KimiErrorCode = "provider.rate_limit"
	KimiErrProviderQuota        KimiErrorCode = "provider.quota_exhausted"
	KimiErrProviderTimeout      KimiErrorCode = "provider.timeout"
	KimiErrProviderConnection   KimiErrorCode = "provider.connection"
	KimiErrProviderContextOverflow KimiErrorCode = "provider.context_overflow"
	KimiErrProviderEmptyResponse KimiErrorCode = "provider.empty_response"
	KimiErrProviderFiltered     KimiErrorCode = "provider.filtered"
	KimiErrProviderBadRequest   KimiErrorCode = "provider.bad_request"

	// Auth errors
	KimiErrAuthTokenMissing     KimiErrorCode = "auth.token_missing"
	KimiErrAuthTokenUnauthorized KimiErrorCode = "auth.token_unauthorized"
	KimiErrAuthProvisioning     KimiErrorCode = "auth.provisioning_required"
	KimiErrAuthModelNotResolved KimiErrorCode = "auth.model_not_resolved"

	// Goal errors
	KimiErrGoalAlreadyExists    KimiErrorCode = "goal.already_exists"
	KimiErrGoalNotFound         KimiErrorCode = "goal.not_found"
	KimiErrGoalStatusInvalid    KimiErrorCode = "goal.status_invalid"
	KimiErrGoalBudgetExhausted  KimiErrorCode = "goal.budget_exhausted"

	// Tool errors
	KimiErrToolExecutionFailed  KimiErrorCode = "tool.execution_failed"
	KimiErrToolNotAvailable     KimiErrorCode = "tool.not_available"
	KimiErrToolPermissionDenied KimiErrorCode = "tool.permission_denied"

	// Compaction errors
	KimiErrCompactionUnable     KimiErrorCode = "compaction.unable"
	KimiErrCompactionOverflow   KimiErrorCode = "compaction.overflow"

	// Validation errors
	KimiErrValidationFailed     KimiErrorCode = "validation.failed"
	KimiErrRequestMalformed     KimiErrorCode = "request.malformed"

	// Internal errors
	KimiErrInternal             KimiErrorCode = "internal.error"
	KimiErrPersistenceFailure   KimiErrorCode = "persistence.failure"
)

// IsRetryable reports whether the error is typically retryable.
func (c KimiErrorCode) IsRetryable() bool {
	switch c {
	case KimiErrProviderRateLimit, KimiErrProviderTimeout,
		KimiErrProviderConnection, KimiErrProviderEmptyResponse:
		return true
	}
	return false
}

// KimiErrorPayload is the structured wire error format.
type KimiErrorPayload struct {
	Code       KimiErrorCode `json:"code"`
	Message    string        `json:"message"`
	Retryable  bool          `json:"retryable,omitempty"`
	StatusCode int           `json:"status_code,omitempty"`
	RequestID  string        `json:"request_id,omitempty"`
	Cause      *KimiErrorPayload `json:"cause,omitempty"`
}

// NewKimiError creates a KimiErrorPayload.
func NewKimiError(code KimiErrorCode, message string) *KimiErrorPayload {
	return &KimiErrorPayload{
		Code:      code,
		Message:   message,
		Retryable: code.IsRetryable(),
	}
}

// WithCause attaches a causal error chain.
func (e *KimiErrorPayload) WithCause(cause *KimiErrorPayload) *KimiErrorPayload {
	e.Cause = cause
	return e
}

// ErrorChain returns the full causal chain as a slice.
func (e *KimiErrorPayload) ErrorChain() []*KimiErrorPayload {
	chain := []*KimiErrorPayload{e}
	for c := e.Cause; c != nil; c = c.Cause {
		chain = append(chain, c)
	}
	return chain
}
