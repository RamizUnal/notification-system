package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	templateNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	variablePattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Template is a reusable message body with {{variable}} placeholders.
// Templates are rendered when a notification is created, so editing a
// template never changes notifications that are already queued.
type Template struct {
	ID        uuid.UUID
	Name      string
	Channel   Channel
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TemplateDraft is a request to create or replace a template.
type TemplateDraft struct {
	Name    string
	Channel Channel
	Body    string
}

// Validate checks the draft and returns every problem as a *ValidationError.
func (d TemplateDraft) Validate() error {
	var verr ValidationError

	if !templateNamePattern.MatchString(d.Name) {
		verr.Add("name", CodeInvalidFormat,
			"name must be 2-64 lowercase letters, digits, '-' or '_', starting with a letter or digit")
	}

	policy, known := PolicyFor(d.Channel)
	if !known {
		verr.Add("channel", CodeInvalidValue, fmt.Sprintf("channel must be one of %s", joinValues(Channels())))
	}

	switch {
	case strings.TrimSpace(d.Body) == "":
		verr.Add("body", CodeRequired, "body is required")
	case known && utf8.RuneCountInString(d.Body) > policy.MaxContentLength:
		verr.Add("body", CodeTooLong, fmt.Sprintf("body may be at most %d characters for %s", policy.MaxContentLength, d.Channel))
	default:
		if _, err := parseTemplate(d.Body); err != nil {
			verr.Add("body", CodeInvalidFormat, err.Error())
		}
	}
	return verr.Err()
}

// Variables lists the placeholder names used in the body, in order of first
// appearance and without duplicates.
func (t Template) Variables() []string {
	segments, err := parseTemplate(t.Body)
	if err != nil {
		return nil
	}
	var names []string
	for _, s := range segments {
		if s.variable != "" && !slices.Contains(names, s.variable) {
			names = append(names, s.variable)
		}
	}
	return names
}

// MissingVariablesError reports placeholders that had no value when rendering.
type MissingVariablesError struct {
	Names []string
}

func (e *MissingVariablesError) Error() string {
	return "missing values for variables: " + strings.Join(e.Names, ", ")
}

// Render substitutes vars into the body. Every placeholder needs a value;
// extra variables are ignored so callers can share one variable map across
// templates.
func (t Template) Render(vars map[string]string) (string, error) {
	segments, err := parseTemplate(t.Body)
	if err != nil {
		return "", fmt.Errorf("template %s is malformed: %w", t.Name, err)
	}

	var (
		out     strings.Builder
		missing []string
	)
	for _, s := range segments {
		if s.variable == "" {
			out.WriteString(s.literal)
			continue
		}
		value, ok := vars[s.variable]
		if !ok {
			if !slices.Contains(missing, s.variable) {
				missing = append(missing, s.variable)
			}
			continue
		}
		out.WriteString(value)
	}
	if len(missing) > 0 {
		return "", &MissingVariablesError{Names: missing}
	}
	return out.String(), nil
}

// segment is either literal text or a reference to a variable.
type segment struct {
	literal  string
	variable string
}

// parseTemplate splits body into literal text and {{ variable }} references.
// Whitespace inside the braces is ignored. A "{{" without a matching "}}" or
// with an invalid name inside is an error; a lone "}}" is plain text.
func parseTemplate(body string) ([]segment, error) {
	var segments []segment
	rest := body
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			if rest != "" {
				segments = append(segments, segment{literal: rest})
			}
			return segments, nil
		}
		if open > 0 {
			segments = append(segments, segment{literal: rest[:open]})
		}

		inner := rest[open+2:]
		end := strings.Index(inner, "}}")
		if end < 0 {
			return nil, fmt.Errorf("placeholder %q is never closed with }}", truncate(rest[open:], 20))
		}
		name := strings.TrimSpace(inner[:end])
		if !variablePattern.MatchString(name) {
			return nil, fmt.Errorf("placeholder {{%s}} must name a variable using letters, digits and underscores", inner[:end])
		}
		segments = append(segments, segment{variable: name})
		rest = inner[end+2:]
	}
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes]) + "…"
}
