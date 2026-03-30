package kiro

import (
	"net/http"
	"time"
)

const defaultRequestTimeout = 5 * time.Minute

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &http.Client{Timeout: timeout}
}
