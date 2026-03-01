package provider

import (
	"fmt"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err      string
		expected ErrorClass
	}{
		// Retryable
		{"500 Internal Server Error", ErrorRetryable},
		{"502 Bad Gateway", ErrorRetryable},
		{"503 Service Unavailable", ErrorRetryable},
		{"connection reset", ErrorRetryable},
		{"timeout", ErrorRetryable},

		// Non-retryable
		{"HTTP 400 Bad Request", ErrorNonRetryable},
		{"HTTP 401 Unauthorized", ErrorNonRetryable},
		{"HTTP 403 Forbidden", ErrorNonRetryable},
		{"404 Not Found", ErrorNonRetryable},
		{"invalid api key provided", ErrorNonRetryable},
		{"authentication failed", ErrorNonRetryable},
		{"model gpt-99 not found", ErrorNonRetryable},
		{"unsupported model: glm-4.7", ErrorNonRetryable},
		{"permission denied", ErrorNonRetryable},

		// Rate limited (retryable with key rotation)
		{"429 Too Many Requests rate limit", ErrorRateLimited},

		// Rate limited but non-retryable (business/quota)
		{"429 Too Many Requests: plan does not include this model", ErrorNonRetryable},
		{"429 rate limit: insufficient balance", ErrorNonRetryable},

		// Context full
		{"maximum context length exceeded", ErrorContextFull},
		{"your input exceeds the context window of this model", ErrorContextFull},
		{"too many tokens in the request", ErrorContextFull},
		{"prompt is too long", ErrorContextFull},

		// 429 and 408 are NOT non-retryable
		{"408 Request Timeout", ErrorRetryable},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			got := ClassifyError(fmt.Errorf("%s", tt.err))
			if got != tt.expected {
				t.Errorf("ClassifyError(%q) = %s, want %s", tt.err, got, tt.expected)
			}
		})
	}
}

func TestParseRetryAfterMs(t *testing.T) {
	tests := []struct {
		err      string
		expected int64
	}{
		{"Retry-After: 5", 5000},
		{"retry-after: 2.5", 2500},
		{"retry_after: 10", 10000},
		{"some error with retry-after: 1 second", 1000},
		{"no retry info here", 0},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			got := ParseRetryAfterMs(fmt.Errorf("%s", tt.err))
			if got != tt.expected {
				t.Errorf("ParseRetryAfterMs(%q) = %d, want %d", tt.err, got, tt.expected)
			}
		})
	}
}
