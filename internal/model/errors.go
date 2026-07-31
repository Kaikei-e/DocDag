package model

import "errors"

// Sentinel errors shared across layers. Callers compare with errors.Is.
var (
	ErrUnknownID       = errors.New("docdag: unknown document id")
	ErrCycle           = errors.New("docdag: cycle in constraint graph")
	ErrNoDocuments     = errors.New("docdag: no documents directory found")
	ErrInvalidConfig   = errors.New("docdag: invalid configuration")
	ErrInvalidDocument = errors.New("docdag: invalid document")
)
