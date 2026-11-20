package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// Problem is an RFC 9457 "problem details" response body.
//
// Type stays "about:blank", so Title is the HTTP status phrase; clients
// branch on Code, a stable machine-readable identifier, and show Detail to
// humans. RequestID echoes the correlation ID so a client can quote it to
// support and an operator can find every log line of that request.
type Problem struct {
	Type      string           `json:"type"`
	Title     string           `json:"title"`
	Status    int              `json:"status"`
	Detail    string           `json:"detail,omitempty"`
	Instance  string           `json:"instance,omitempty"`
	Code      string           `json:"code"`
	RequestID string           `json:"request_id,omitempty"`
	Errors    []FieldProblem   `json:"errors,omitempty"`
	Conflicts []conflictDetail `json:"conflicts,omitempty"`
}

// FieldProblem describes one invalid input field or query parameter.
type FieldProblem struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type conflictDetail struct {
	Index          int    `json:"index"`
	IdempotencyKey string `json:"idempotency_key"`
	NotificationID string `json:"notification_id"`
}

// Stable problem codes.
const (
	codeValidationFailed    = "validation_failed"
	codeInvalidParameters   = "invalid_parameters"
	codeInvalidJSON         = "invalid_json"
	codeUnsupportedMedia    = "unsupported_media_type"
	codePayloadTooLarge     = "payload_too_large"
	codeNotFound            = "not_found"
	codeMethodNotAllowed    = "method_not_allowed"
	codeNotCancellable      = "not_cancellable"
	codeNotRetryable        = "not_retryable"
	codeIdempotencyConflict = "idempotency_conflict"
	codeTemplateNameTaken   = "template_name_taken"
	codeInternal            = "internal_error"
)

// requestError is a problem detected by the transport itself: an
// unreadable body, a malformed query parameter, an unknown route.
type requestError struct {
	status int
	code   string
	detail string
	fields []FieldProblem
}

func (e *requestError) Error() string { return e.detail }

func newRequestError(status int, code, detail string, fields ...FieldProblem) *requestError {
	return &requestError{status: status, code: code, detail: detail, fields: fields}
}

// sentinelProblems maps business errors to responses. Lookup uses
// errors.Is, so wrapped errors map just like bare ones.
var sentinelProblems = []struct {
	err    error
	status int
	code   string
}{
	{domain.ErrNotFound, http.StatusNotFound, codeNotFound},
	{domain.ErrNotCancellable, http.StatusConflict, codeNotCancellable},
	{domain.ErrNotRetryable, http.StatusConflict, codeNotRetryable},
	{domain.ErrIdempotencyConflict, http.StatusConflict, codeIdempotencyConflict},
	{domain.ErrTemplateNameTaken, http.StatusConflict, codeTemplateNameTaken},
}

// problemFor translates err into a response. The second result is false for
// unexpected errors, whose details must be logged but never shown to clients.
func problemFor(err error) (Problem, bool) {
	var (
		reqErr   *requestError
		valErr   *domain.ValidationError
		conflict *domain.IdempotencyConflictError
		notFound *domain.NotFoundError
	)

	switch {
	case errors.As(err, &reqErr):
		return newProblem(reqErr.status, reqErr.code, reqErr.detail, reqErr.fields), true

	case errors.As(err, &valErr):
		fields := make([]FieldProblem, len(valErr.Fields))
		for i, f := range valErr.Fields {
			fields[i] = FieldProblem(f)
		}
		return newProblem(http.StatusUnprocessableEntity, codeValidationFailed,
			"The request contains invalid fields; see errors for details.", fields), true

	case errors.As(err, &conflict):
		p := newProblem(http.StatusConflict, codeIdempotencyConflict,
			"Some notifications reuse idempotency keys of existing notifications. Nothing was created.", nil)
		for _, c := range conflict.Conflicts {
			p.Conflicts = append(p.Conflicts, conflictDetail{
				Index: c.Index, IdempotencyKey: c.Key, NotificationID: c.ExistingID.String(),
			})
		}
		return p, true

	case errors.As(err, &notFound):
		return newProblem(http.StatusNotFound, codeNotFound, notFound.Error(), nil), true
	}

	for _, s := range sentinelProblems {
		if errors.Is(err, s.err) {
			return newProblem(s.status, s.code, err.Error(), nil), true
		}
	}

	return newProblem(http.StatusInternalServerError, codeInternal,
		"Something went wrong on our side. Quote the request_id when reporting it.", nil), false
}

func newProblem(status int, code, detail string, fields []FieldProblem) Problem {
	return Problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Code:   code,
		Errors: fields,
	}
}

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
