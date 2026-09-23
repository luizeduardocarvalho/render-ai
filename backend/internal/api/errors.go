package api

import (
	"errors"
	"fmt"
	"net/http"

	"render-ai/backend/internal/store"
)

// httpError pairs an error message with the HTTP status it should produce.
type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func badRequest(format string, args ...any) *httpError {
	return &httpError{http.StatusBadRequest, fmt.Sprintf(format, args...)}
}

func unauthorized(format string, args ...any) *httpError {
	return &httpError{http.StatusUnauthorized, fmt.Sprintf(format, args...)}
}

func forbidden(format string, args ...any) *httpError {
	return &httpError{http.StatusForbidden, fmt.Sprintf(format, args...)}
}

func notFoundErr(format string, args ...any) *httpError {
	return &httpError{http.StatusNotFound, fmt.Sprintf(format, args...)}
}

func internalErr(format string, args ...any) *httpError {
	return &httpError{http.StatusInternalServerError, fmt.Sprintf(format, args...)}
}

func badGateway(format string, args ...any) *httpError {
	return &httpError{http.StatusBadGateway, fmt.Sprintf(format, args...)}
}

func timeoutErr(format string, args ...any) *httpError {
	return &httpError{http.StatusGatewayTimeout, fmt.Sprintf(format, args...)}
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
