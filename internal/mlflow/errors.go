package mlflow

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("mlflow: %s (HTTP %d): %s", e.Code, e.Status, e.Message)
	}
	return fmt.Sprintf("mlflow: HTTP %d: %s", e.Status, e.Message)
}

func newAPIError(status int, body []byte) *APIError {
	var payload struct {
		Code    string `json:"error_code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &payload); err == nil && payload.Code != "" {
		return &APIError{Status: status, Code: payload.Code, Message: payload.Message}
	}
	// when the response is just text
	msg := strings.TrimSpace(string(body))

	if msg == "" {
		msg = http.StatusText(status)
	}

	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200]) + "..."
	}

	return &APIError{Status: status, Message: msg}

}
