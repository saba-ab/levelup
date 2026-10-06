// Package domain holds ai's invariants: what a request may ask for, what a
// valid draft of each kind looks like (the exact create-request body of the
// owning module), the template catalogue and usage accounting. No I/O.
package domain

import (
	"slices"
	"strings"
	"unicode/utf8"

	"levelup/internal/modules/ai/contracts"
)

const (
	MaxPromptLen = 2000
	MinCount     = 1
	MaxCount     = 5
	DefaultCount = 3
)

// DraftRequest is a validated drafting request.
type DraftRequest struct {
	Kind    string
	Prompt  string
	Count   int
	Context Context
}

// NewDraftRequest checks kind, prompt and count; count 0 means the default.
func NewDraftRequest(kind, prompt string, count int, c Context) (DraftRequest, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if !slices.Contains(contracts.Kinds, kind) {
		return DraftRequest{}, ErrUnknownKind
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return DraftRequest{}, ErrPromptRequired
	}
	if utf8.RuneCountInString(prompt) > MaxPromptLen {
		return DraftRequest{}, ErrPromptTooLong
	}
	if count == 0 {
		count = DefaultCount
	}
	if count < MinCount || count > MaxCount {
		return DraftRequest{}, ErrBadCount
	}
	if err := c.Validate(); err != nil {
		return DraftRequest{}, err
	}
	return DraftRequest{Kind: kind, Prompt: prompt, Count: count, Context: c}, nil
}
