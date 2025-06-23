package domain

import (
	"context"
)

// PrivateNumberRepository defines the interface for interacting with the storage
// of private numbers, specifically for finding a number to associate with an incoming SMS.
type PrivateNumberRepository interface {
	// FindByNumber searches for a PrivateNumber record matching the given phone number string.
	// It should return (nil, nil) if no number is found (not an error in itself).
	// A database error would be returned as a non-nil error.
	FindByNumber(ctx context.Context, number string) (*PrivateNumber, error)
}
