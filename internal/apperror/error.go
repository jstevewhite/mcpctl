// Package apperror defines typed application errors and their exit codes.
package apperror

import "fmt"

// Kind classifies an application error.
type Kind int

const (
	KindInternal Kind = iota
	KindUsage
	KindConfig
	KindAuth
	KindConnection
	KindProtocol
	KindToolNotFound
	KindInvalidArgs
	KindToolError
	KindTimeout
	KindInterrupted
)

// Error is a typed application error carrying an exit-code classification.
type Error struct {
	Kind Kind
	Msg  string
	Err  error // optional wrapped cause
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return e.Msg + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// New builds an Error with no wrapped cause.
func New(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// Wrap builds an Error around an existing cause.
func Wrap(kind Kind, err error, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...), Err: err}
}

// Convenience constructors.
func Usage(format string, args ...any) *Error    { return New(KindUsage, format, args...) }
func Config(format string, args ...any) *Error   { return New(KindConfig, format, args...) }
func Internal(format string, args ...any) *Error { return New(KindInternal, format, args...) }
