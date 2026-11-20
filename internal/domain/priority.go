package domain

import "fmt"

// Priority decides the order in which waiting notifications are delivered.
type Priority string

// Supported priorities, from most to least urgent.
const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

// Priorities returns every priority from most to least urgent.
func Priorities() []Priority {
	return []Priority{PriorityHigh, PriorityNormal, PriorityLow}
}

// Valid reports whether p is a supported priority.
func (p Priority) Valid() bool {
	switch p {
	case PriorityHigh, PriorityNormal, PriorityLow:
		return true
	default:
		return false
	}
}

// Rank orders priorities for dequeueing: lower ranks are delivered first.
// The storage layer persists the same ranking so the queue index can sort on it.
func (p Priority) Rank() int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityNormal:
		return 1
	default:
		return 2
	}
}

// ParsePriority converts s into a Priority, rejecting unknown values.
func ParsePriority(s string) (Priority, error) {
	p := Priority(s)
	if !p.Valid() {
		return "", fmt.Errorf("unknown priority %q", s)
	}
	return p, nil
}
