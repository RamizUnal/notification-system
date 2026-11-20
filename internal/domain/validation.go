package domain

import (
	"fmt"
	"strings"
)

// Stable, machine-readable validation codes. Clients may branch on them; the
// accompanying messages are for humans and may change.
const (
	CodeRequired         = "required"
	CodeInvalidValue     = "invalid_value"
	CodeInvalidFormat    = "invalid_format"
	CodeTooLong          = "too_long"
	CodeOutOfRange       = "out_of_range"
	CodeDuplicate        = "duplicate"
	CodeMissingVariables = "missing_variables"
	CodeChannelMismatch  = "channel_mismatch"
)

// FieldError describes one invalid input field.
type FieldError struct {
	// Field is the JSON path of the offending field, e.g. "notifications[3].recipient".
	Field string
	// Code is a stable machine-readable reason such as "too_long".
	Code string
	// Message explains the problem to a human.
	Message string
}

// ValidationError collects every problem found in an input, so clients can fix
// all of them in one round trip instead of discovering them one by one.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	switch len(e.Fields) {
	case 0:
		return "validation failed"
	case 1:
		f := e.Fields[0]
		return fmt.Sprintf("validation failed: %s: %s", f.Field, f.Message)
	default:
		return fmt.Sprintf("validation failed: %d invalid fields", len(e.Fields))
	}
}

// Add records a problem with field.
func (e *ValidationError) Add(field, code, message string) {
	e.Fields = append(e.Fields, FieldError{Field: field, Code: code, Message: message})
}

// Merge copies the problems of other into e, prefixing each field path with
// prefix (for example "notifications[3]").
func (e *ValidationError) Merge(prefix string, other *ValidationError) {
	if other == nil {
		return
	}
	for _, f := range other.Fields {
		f.Field = joinPath(prefix, f.Field)
		e.Fields = append(e.Fields, f)
	}
}

// Err returns e as an error, or nil when nothing was recorded. Returning the
// result of Err (instead of e itself) avoids the classic typed-nil pitfall
// where a nil *ValidationError becomes a non-nil error interface.
func (e *ValidationError) Err() error {
	if e == nil || len(e.Fields) == 0 {
		return nil
	}
	return e
}

func joinPath(prefix, field string) string {
	switch {
	case prefix == "":
		return field
	case field == "":
		return prefix
	case strings.HasPrefix(field, "["):
		return prefix + field
	default:
		return prefix + "." + field
	}
}
