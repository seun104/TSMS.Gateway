package domain

import "errors"

var (
	// ErrNotFound indicates that a requested resource was not found.
	ErrNotFound = errors.New("resource not found")
	// ErrJobAlreadyLocked indicates that a job could not be acquired because it was already locked.
	ErrJobAlreadyLocked = errors.New("job already locked")
	// ErrNoDueJobs indicates that no jobs are currently due for processing.
	ErrNoDueJobs = errors.New("no due jobs found")
)
