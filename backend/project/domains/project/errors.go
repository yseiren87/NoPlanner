package project

import "errors"

var (
	ErrInvalid   = errors.New("invalid project input")
	ErrNotFound  = errors.New("project not found")
	ErrForbidden = errors.New("project access forbidden")
	ErrLastOwner = errors.New("project must retain an owner")
)
