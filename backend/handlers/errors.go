package handlers

import (
	"errors"
	"net/http"

	"github.com/argval/yoink/github"
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
