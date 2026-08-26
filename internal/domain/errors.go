package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("revision conflict")
	ErrInvalidState = errors.New("invalid state")
	ErrIdempotent   = errors.New("idempotent replay")
)
