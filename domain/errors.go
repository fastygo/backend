package domain

import (
	"errors"
	"fmt"
)

// ErrorCode represents a semantic classification shared across transport layers.
type ErrorCode string

const (
	ErrCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrCodeInvalid      ErrorCode = "INVALID"
	ErrCodeConflict     ErrorCode = "CONFLICT"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeInternal     ErrorCode = "INTERNAL"
)

// Error represents a domain-level error.
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewError builds a domain error.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WrapError wraps an existing error with a domain classification.
func WrapError(code ErrorCode, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Common domain errors.
var (
	ErrUserNotFound    = NewError(ErrCodeNotFound, "user not found")
	ErrTaskNotFound    = NewError(ErrCodeNotFound, "task not found")
	ErrSessionNotFound = NewError(ErrCodeNotFound, "session not found")
	ErrAggregateNotFound = NewError(ErrCodeNotFound, "aggregate not found")
	ErrUnauthorized    = NewError(ErrCodeUnauthorized, "unauthorized")
	ErrInvalidPayload  = NewError(ErrCodeInvalid, "invalid payload")
)

// IsDomainError helps checking error codes.
func IsDomainError(err error, code ErrorCode) bool {
	var dErr *Error
	if errors.As(err, &dErr) {
		return dErr.Code == code
	}
	return false
}
