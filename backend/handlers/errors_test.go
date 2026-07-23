package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/argval/yatko/github"
)

func TestPublicErrorMessage(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&github.APIError{StatusCode: http.StatusNotFound, Message: `{"message":"Not Found"}`}, "not found"},
		{&github.APIError{StatusCode: http.StatusForbidden, Message: "API rate limit exceeded for xxx"}, "forbidden"},
		{&github.APIError{StatusCode: http.StatusTooManyRequests, Message: "GitHub rate limit nearly exhausted"}, "rate limited"},
		{&github.APIError{StatusCode: http.StatusBadGateway, Message: "upstream body"}, "upstream error"},
		{errors.New("performing request: dial tcp: lookup api.github.com"), "upstream error"},
	}
	for _, tc := range cases {
		if got := publicErrorMessage(tc.err); got != tc.want {
			t.Errorf("publicErrorMessage(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
