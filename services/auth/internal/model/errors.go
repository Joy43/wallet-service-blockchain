package model

import "errors"

// ErrorCode is transport-agnostic. The transport layer maps it to
// HTTP status codes (or gRPC codes) at the edge.
type ErrorCode string

const (
	ErrInvalid         ErrorCode = "INVALID_ARGUMENT"
	ErrNotFound        ErrorCode = "NOT_FOUND"
	ErrConflict        ErrorCode = "CONFLICT"
	ErrUnauthenticated ErrorCode = "UNAUTHENTICATED"
	ErrForbidden       ErrorCode = "FORBIDDEN"
	ErrRateLimited     ErrorCode = "RATE_LIMITED"
	ErrUnavailable     ErrorCode = "UNAVAILABLE"
	ErrInternal        ErrorCode = "INTERNAL"
)

type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func NewError(code ErrorCode, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// CodeOf finds the domain error code anywhere in a wrapped error chain.
func CodeOf(err error) ErrorCode {
	var de *Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ErrInternal
}

// ------ MessageOf returns a client-safe message. Internal errors are hidden.-------
func MessageOf(err error) string {
	var de *Error
	if errors.As(err, &de) && de.Code != ErrInternal {
		return de.Message
	}
	return "internal error"
}
