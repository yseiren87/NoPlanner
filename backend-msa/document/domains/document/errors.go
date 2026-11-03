package document

import "errors"

var (
	ErrInvalid  = errors.New("invalid document input")
	ErrNotFound = errors.New("document version not found")
)
