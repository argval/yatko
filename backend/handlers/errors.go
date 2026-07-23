package handlers

import (
	"errors"
	"net/http"

	"github.com/argval/yatko/github"
)

// httpStatusFromError maps a GitHub APIError to an appropriate HTTP status code.
// Falls back to 502 Bad Gateway for unexpected errors.
func httpStatusFromError(err error) int {
	var apiErr *github.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusNotFound:
			return http.StatusNotFound
		case http.StatusForbidden, http.StatusUnauthorized:
			return http.StatusForbidden
		case http.StatusTooManyRequests:
			return http.StatusTooManyRequests
		}
	}
	return http.StatusBadGateway
}

// publicErrorMessage returns a fixed, status-appropriate client message.
// Callers should log err.Error() server-side; never echo upstream bodies or
// internal Go strings to unauthenticated clients.
func publicErrorMessage(err error) string {
	switch httpStatusFromError(err) {
	case http.StatusNotFound:
		return "not found"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusTooManyRequests:
		return "rate limited"
	default:
		return "upstream error"
	}
}
