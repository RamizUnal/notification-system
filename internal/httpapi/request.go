package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// Request body limits.
const (
	maxSmallBody = 1 << 20  // single notifications and templates
	maxBatchBody = 16 << 20 // up to 1,000 notifications
)

// decodeJSON reads exactly one JSON value into dst. It rejects other media
// types, bodies over limit, unknown fields and trailing data, and turns
// every decoding failure into a precise client error.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" {
			return newRequestError(http.StatusUnsupportedMediaType, codeUnsupportedMedia,
				"Send the body as JSON with the header Content-Type: application/json.")
		}
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return describeDecodeError(err, limit)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return newRequestError(http.StatusBadRequest, codeInvalidJSON, "The body must contain a single JSON object.")
	}
	return nil
}

func describeDecodeError(err error, limit int64) error {
	var (
		syntaxErr  *json.SyntaxError
		typeErr    *json.UnmarshalTypeError
		maxSizeErr *http.MaxBytesError
	)
	switch {
	case errors.Is(err, io.EOF):
		return newRequestError(http.StatusBadRequest, codeInvalidJSON, "The request body is empty.")
	case errors.As(err, &maxSizeErr):
		return newRequestError(http.StatusRequestEntityTooLarge, codePayloadTooLarge,
			fmt.Sprintf("The request body must not exceed %d bytes.", limit))
	case errors.As(err, &syntaxErr):
		return newRequestError(http.StatusBadRequest, codeInvalidJSON,
			fmt.Sprintf("The body is not valid JSON (error at byte %d).", syntaxErr.Offset))
	case errors.Is(err, io.ErrUnexpectedEOF):
		return newRequestError(http.StatusBadRequest, codeInvalidJSON, "The body ends in the middle of a JSON value.")
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "body"
		}
		return newRequestError(http.StatusBadRequest, codeInvalidJSON,
			fmt.Sprintf("Field %q must be a JSON %s.", field, jsonKind(typeErr.Type.Kind().String())),
			FieldProblem{Field: field, Code: domain.CodeInvalidValue, Message: "has the wrong JSON type"})
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		name := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return newRequestError(http.StatusBadRequest, codeInvalidJSON,
			fmt.Sprintf("Unknown field %q.", name),
			FieldProblem{Field: name, Code: "unknown_field", Message: "is not a recognized field"})
	default:
		return newRequestError(http.StatusBadRequest, codeInvalidJSON, "The body could not be decoded as JSON.")
	}
}

func jsonKind(goKind string) string {
	switch goKind {
	case "string":
		return "string"
	case "map", "struct":
		return "object"
	case "slice", "array":
		return "array"
	case "bool":
		return "boolean"
	default:
		return "number"
	}
}

// pathID parses the {id} path parameter. A malformed ID cannot name an
// existing resource, so it is reported as not found.
func pathID(r *http.Request, resource string) (uuid.UUID, error) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.NotFound(resource, raw)
	}
	return id, nil
}

// queryErrors collects problems with query parameters.
type queryErrors []FieldProblem

func (q *queryErrors) add(param, message string) {
	*q = append(*q, FieldProblem{Field: param, Code: domain.CodeInvalidValue, Message: message})
}

func (q queryErrors) err() error {
	if len(q) == 0 {
		return nil
	}
	return newRequestError(http.StatusBadRequest, codeInvalidParameters,
		"Some query parameters are invalid; see errors for details.", q...)
}

// parseListFilter reads the listing parameters. Multi-valued parameters
// accept both repetition and commas: ?status=queued&status=failed or
// ?status=queued,failed.
func parseListFilter(q url.Values) (service.ListFilter, error) {
	var (
		f    service.ListFilter
		errs queryErrors
	)

	for _, v := range multiValues(q, "status") {
		s, err := domain.ParseStatus(v)
		if err != nil {
			errs.add("status", fmt.Sprintf("%q is not a status", v))
			continue
		}
		f.Statuses = append(f.Statuses, s)
	}
	for _, v := range multiValues(q, "channel") {
		c, err := domain.ParseChannel(v)
		if err != nil {
			errs.add("channel", fmt.Sprintf("%q is not a channel", v))
			continue
		}
		f.Channels = append(f.Channels, c)
	}
	for _, v := range multiValues(q, "priority") {
		p, err := domain.ParsePriority(v)
		if err != nil {
			errs.add("priority", fmt.Sprintf("%q is not a priority", v))
			continue
		}
		f.Priorities = append(f.Priorities, p)
	}

	if v := q.Get("batch_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			errs.add("batch_id", "must be a UUID")
		} else {
			f.BatchID = &id
		}
	}
	f.Recipient = strings.TrimSpace(q.Get("recipient"))
	f.CreatedFrom = parseTimeParam(q, "created_from", &errs)
	f.CreatedTo = parseTimeParam(q, "created_to", &errs)

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs.add("limit", "must be an integer")
		} else {
			f.Limit = n
		}
	}
	if v := q.Get("cursor"); v != "" {
		c, err := decodeCursor(v)
		if err != nil {
			errs.add("cursor", "is not a cursor returned by this API")
		} else {
			f.After = &c
		}
	}
	return f, errs.err()
}

func multiValues(q url.Values, key string) []string {
	var out []string
	for _, raw := range q[key] {
		for part := range strings.SplitSeq(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func parseTimeParam(q url.Values, key string, errs *queryErrors) *time.Time {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		errs.add(key, "must be an RFC 3339 timestamp such as 2026-11-20T09:30:00Z")
		return nil
	}
	return &t
}

// cursorPayload is the content of an opaque pagination cursor.
type cursorPayload struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"id"`
}

func encodeCursor(c *service.Cursor) *string {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(cursorPayload{CreatedAt: c.CreatedAt, ID: c.ID})
	s := base64.RawURLEncoding.EncodeToString(b)
	return &s
}

func decodeCursor(s string) (service.Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return service.Cursor{}, err
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return service.Cursor{}, err
	}
	if p.ID == uuid.Nil || p.CreatedAt.IsZero() {
		return service.Cursor{}, errors.New("incomplete cursor")
	}
	return service.Cursor{CreatedAt: p.CreatedAt, ID: p.ID}, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}
