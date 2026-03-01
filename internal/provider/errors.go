package provider

import (
	"strconv"
	"strings"
)

// ErrorClass categorizes errors for retry decisions.
type ErrorClass string

const (
	ErrorRetryable    ErrorClass = "retryable"     // 5xx, 429, 408, network errors
	ErrorNonRetryable ErrorClass = "non_retryable" // 4xx (not 429/408), auth, model not found
	ErrorContextFull  ErrorClass = "context_full"   // context window exceeded — compact, don't retry
	ErrorRateLimited  ErrorClass = "rate_limited"   // 429 specifically — rotate key + retry
)

// ClassifyError determines how an error should be handled by the retry loop.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorRetryable
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	// Check context window exceeded first (subset of non-retryable,
	// but gets special treatment — signals compaction, not failover)
	if isContextWindowExceeded(lower) {
		return ErrorContextFull
	}

	// Check for rate limiting (429)
	if isRateLimited(msg, lower) {
		// Some 429s are business/quota errors that retries can't fix
		if isNonRetryableRateLimit(lower) {
			return ErrorNonRetryable
		}
		return ErrorRateLimited
	}

	// Check for non-retryable errors (4xx except 429/408, auth failures, model not found)
	if isNonRetryable(msg, lower) {
		return ErrorNonRetryable
	}

	// Everything else (5xx, network errors, timeouts) is retryable
	return ErrorRetryable
}

func isContextWindowExceeded(lower string) bool {
	hints := []string{
		"exceeds the context window",
		"context window of this model",
		"maximum context length",
		"context length exceeded",
		"too many tokens",
		"token limit exceeded",
		"prompt is too long",
		"input is too long",
	}
	for _, h := range hints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func isRateLimited(msg, lower string) bool {
	if !strings.Contains(msg, "429") {
		return false
	}
	return strings.Contains(lower, "too many") ||
		strings.Contains(lower, "rate") ||
		strings.Contains(lower, "limit")
}

func isNonRetryableRateLimit(lower string) bool {
	businessHints := []string{
		"plan does not include",
		"insufficient balance",
		"insufficient quota",
		"quota exhausted",
		"out of credits",
		"package not active",
	}
	for _, h := range businessHints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func isNonRetryable(msg, lower string) bool {
	// Check for HTTP 4xx status codes (not 429, not 408)
	if code := extractHTTPStatus(msg); code > 0 {
		if code >= 400 && code < 500 && code != 429 && code != 408 {
			return true
		}
	}

	// Auth failure keywords
	authHints := []string{
		"invalid api key",
		"incorrect api key",
		"missing api key",
		"api key not set",
		"authentication failed",
		"auth failed",
		"unauthorized",
		"forbidden",
		"permission denied",
		"access denied",
		"invalid token",
	}
	for _, h := range authHints {
		if strings.Contains(lower, h) {
			return true
		}
	}

	// Model not found
	if strings.Contains(lower, "model") {
		modelHints := []string{"not found", "unknown", "unsupported", "does not exist", "invalid"}
		for _, h := range modelHints {
			if strings.Contains(lower, h) {
				return true
			}
		}
	}

	return false
}

// extractHTTPStatus tries to find an HTTP status code in an error string.
func extractHTTPStatus(msg string) int {
	// Look for "HTTP NNN" pattern
	if idx := strings.Index(msg, "HTTP "); idx >= 0 {
		rest := msg[idx+5:]
		if len(rest) >= 3 {
			if code, err := strconv.Atoi(rest[:3]); err == nil {
				return code
			}
		}
	}
	// Look for standalone 3-digit numbers that look like status codes
	for _, word := range strings.Fields(msg) {
		if len(word) == 3 {
			if code, err := strconv.Atoi(word); err == nil && code >= 100 && code < 600 {
				return code
			}
		}
	}
	return 0
}

// ParseRetryAfterMs tries to extract a Retry-After header value from an error message.
// Returns milliseconds to wait, or 0 if not found.
func ParseRetryAfterMs(err error) int64 {
	if err == nil {
		return 0
	}
	lower := strings.ToLower(err.Error())
	for _, prefix := range []string{"retry-after:", "retry_after:", "retry-after ", "retry_after "} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			rest := strings.TrimSpace(lower[idx+len(prefix):])
			// Parse the number (could be float seconds)
			numStr := ""
			for _, c := range rest {
				if c >= '0' && c <= '9' || c == '.' {
					numStr += string(c)
				} else {
					break
				}
			}
			if secs, err := strconv.ParseFloat(numStr, 64); err == nil && secs >= 0 {
				return int64(secs * 1000)
			}
		}
	}
	return 0
}
