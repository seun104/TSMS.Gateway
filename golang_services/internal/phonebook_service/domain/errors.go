package domain

import "errors"

var (
	// ErrNotFound indicates that a requested resource was not found.
	ErrNotFound = errors.New("resource not found")
	// ErrAccessDenied indicates that the user does not have permission to access/modify the resource.
	ErrAccessDenied = errors.New("access denied")
	// ErrDuplicateEntry indicates a unique constraint violation.
	ErrDuplicateEntry = errors.New("duplicate entry")
)
