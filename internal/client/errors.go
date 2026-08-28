package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIError is a non-2xx response from the Rauthy API.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	// Message is Rauthy's `message` field when the body parsed as its error
	// envelope, otherwise the raw body (truncated).
	Message string
	// ErrorCode is Rauthy's `error` field, e.g. "NotFound", when present.
	ErrorCode string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.StatusCode)
	}
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Message)
}

const maxErrBody = 512

func newAPIError(method, path string, status int, raw []byte) *APIError {
	e := &APIError{Method: method, Path: path, StatusCode: status}

	var envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && (envelope.Message != "" || envelope.Error != "") {
		e.ErrorCode = envelope.Error
		e.Message = envelope.Message
		if e.Message == "" {
			e.Message = envelope.Error
		}
		return e
	}

	msg := strings.TrimSpace(string(raw))
	if len(msg) > maxErrBody {
		msg = msg[:maxErrBody] + "…"
	}
	e.Message = msg
	return e
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsForbidden reports whether err is a 403 from the API. For this provider a
// 403 almost always means the API key is missing an access-group right rather
// than that the object is off limits.
func IsForbidden(err error) bool { return hasStatus(err, http.StatusForbidden) }

// IsUnauthorized reports whether err is a 401 from the API.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == status
	}
	return false
}
