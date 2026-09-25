package api

import (
	"errors"
	"fmt"
	"net/http"

	"render-ai/backend/internal/store"
)

// httpError pairs an error message with the HTTP status it should produce.
// code, when set, is echoed as an extra "code" field alongside "error" in the
// JSON body (see writeErr) - machine-readable, for the one error the frontend
// must branch on specifically: 402 insufficient credits.
type httpError struct {
	status  int
	message string
	code    string
}

func (e *httpError) Error() string { return e.message }

func badRequest(format string, args ...any) *httpError {
	return &httpError{status: http.StatusBadRequest, message: fmt.Sprintf(format, args...)}
}

func unauthorized(format string, args ...any) *httpError {
	return &httpError{status: http.StatusUnauthorized, message: fmt.Sprintf(format, args...)}
}

func forbidden(format string, args ...any) *httpError {
	return &httpError{status: http.StatusForbidden, message: fmt.Sprintf(format, args...)}
}

func notFoundErr(format string, args ...any) *httpError {
	return &httpError{status: http.StatusNotFound, message: fmt.Sprintf(format, args...)}
}

func internalErr(format string, args ...any) *httpError {
	return &httpError{status: http.StatusInternalServerError, message: fmt.Sprintf(format, args...)}
}

func badGateway(format string, args ...any) *httpError {
	return &httpError{status: http.StatusBadGateway, message: fmt.Sprintf(format, args...)}
}

func timeoutErr(format string, args ...any) *httpError {
	return &httpError{status: http.StatusGatewayTimeout, message: fmt.Sprintf(format, args...)}
}

// insufficientCreditsErr is the 402 a render (or inventory generation) that
// can't be paid for returns - see API_CONTRACT.md's credits section. The
// frontend branches on the "code" field, not the message text.
func insufficientCreditsErr() *httpError {
	return &httpError{status: http.StatusPaymentRequired, message: "insufficient credits", code: "insufficient_credits"}
}

// rateLimitedErr is the 429 returned when the AI provider's quota is used up
// and retrying did not help. The frontend branches on the "code" field.
func rateLimitedErr() *httpError {
	return &httpError{status: http.StatusTooManyRequests, message: "the AI service is busy, try again in a minute", code: "rate_limited"}
}

// mapStoreErr turns store.ErrNotFound into a 404 with a friendlier message,
// and passes any other error through unchanged.
func mapStoreErr(err error, format string, args ...any) error {
	if errors.Is(err, store.ErrNotFound) {
		return notFoundErr(format, args...)
	}
	return err
}

// statusAndMessage resolves the HTTP status and message for any error
// returned by a handler.
func statusAndMessage(err error) (int, string) {
	var he *httpError
	if errors.As(err, &he) {
		return he.status, he.message
	}
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound, "not found"
	}
	return http.StatusInternalServerError, err.Error()
}
