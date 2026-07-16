package apperror

import (
	"context"
	"errors"
)

// ExitCode maps any error to a stable process exit code.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	var ae *Error
	if errors.As(err, &ae) {
		switch ae.Kind {
		case KindUsage:
			return 2
		case KindConfig:
			return 3
		case KindAuth:
			return 4
		case KindConnection:
			return 5
		case KindProtocol:
			return 6
		case KindToolNotFound:
			return 7
		case KindInvalidArgs:
			return 8
		case KindToolError:
			return 9
		case KindTimeout:
			return 10
		case KindInterrupted:
			return 130
		default:
			return 1
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return 10
	case errors.Is(err, context.Canceled):
		return 130
	default:
		return 1
	}
}
