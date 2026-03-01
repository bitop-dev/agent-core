package agent

import (
	"regexp"
	"strings"
)

// scrubCredentials replaces known credential patterns in text with
// redacted placeholders. It preserves a small prefix (first 4 chars)
// for context so the LLM can understand what type of value was present.
//
// Matches patterns like:
//   - token = "sk-abc123..."       → token = "sk-a*[REDACTED]"
//   - API_KEY: ghp_xyzLongValue    → API_KEY: ghp_*[REDACTED]
//   - "password": "hunter2abc"     → "password": "hunt*[REDACTED]"
//   - BEARER=eyJhbGciOi...        → BEARER=eyJh*[REDACTED]
//
// Also matches standalone high-entropy secrets that look like API keys
// even without a labeled key name.
func scrubCredentials(input string) string {
	result := sensitiveKVRegex.ReplaceAllStringFunc(input, replaceKV)
	result = standaloneBearerRegex.ReplaceAllStringFunc(result, replaceStandaloneBearer)
	return result
}

// sensitiveKVRegex matches key=value or key: value patterns where the key
// contains a sensitive word (token, api_key, password, secret, etc.)
// and the value is at least 8 chars (to avoid false positives on short values).
var sensitiveKVRegex = regexp.MustCompile(
	`(?i)(token|api[_-]?key|password|passwd|secret|user[_-]?key|bearer|credential|auth|private[_-]?key|access[_-]?key)` +
		`(["']?\s*[:=]\s*)` +
		`(?:` +
		`"([^"]{8,})"` + // double-quoted value
		`|'([^']{8,})'` + // single-quoted value
		`|([a-zA-Z0-9_\-\.\/\+]{8,})` + // unquoted value
		`)`,
)

// standaloneBearerRegex matches "Bearer <token>" in Authorization headers.
var standaloneBearerRegex = regexp.MustCompile(
	`(?i)(Bearer\s+)([a-zA-Z0-9_\-\.\/\+]{8,})`,
)

// replaceKV handles key=value and key: value replacement.
func replaceKV(match string) string {
	subs := sensitiveKVRegex.FindStringSubmatch(match)
	if subs == nil {
		return match
	}

	key := subs[1]
	sep := subs[2]

	// Find which capture group got the value
	var val string
	var quoted string
	switch {
	case subs[3] != "":
		val = subs[3]
		quoted = `"`
	case subs[4] != "":
		val = subs[4]
		quoted = `'`
	case subs[5] != "":
		val = subs[5]
		quoted = ""
	default:
		return match
	}

	prefix := val
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}

	return key + sep + quoted + prefix + "*[REDACTED]" + quoted
}

// replaceStandaloneBearer handles "Bearer <token>" patterns.
func replaceStandaloneBearer(match string) string {
	subs := standaloneBearerRegex.FindStringSubmatch(match)
	if subs == nil {
		return match
	}
	prefix := subs[2]
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	return subs[1] + prefix + "*[REDACTED]"
}

// envKeySensitive returns true if an env var name likely contains a secret.
// Used to decide which env vars to scrub from tool output.
var sensitiveEnvKeyPatterns = []string{
	"token", "api_key", "apikey", "password", "passwd",
	"secret", "private_key", "access_key", "auth",
	"credential", "bearer",
}

// IsSensitiveEnvKey returns true if the key name suggests it holds a secret.
func IsSensitiveEnvKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pat := range sensitiveEnvKeyPatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}
