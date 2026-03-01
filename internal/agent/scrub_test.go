package agent

import (
	"strings"
	"testing"
)

func TestScrubCredentials_KeyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // what the output MUST contain
		excludes string // what the output must NOT contain
	}{
		{
			name:     "env var equals",
			input:    `API_KEY=sk-3NI1--VUSGnbEEgikDkVsQ`,
			contains: "sk-3*[REDACTED]",
			excludes: "VUSGnbEEgikDkVsQ",
		},
		{
			name:     "json double-quoted",
			input:    `"token": "ghp_xyzABCDEFGH123456789"`,
			contains: `ghp_*[REDACTED]"`,
			excludes: "ABCDEFGH",
		},
		{
			name:     "yaml colon",
			input:    `password: hunter2abcdef`,
			contains: `hunt*[REDACTED]`,
			excludes: "abcdef",
		},
		{
			name:     "single-quoted",
			input:    `secret = 'mySuper$ecret123!'`,
			excludes: "ecret123",
		},
		{
			name:     "bearer token header",
			input:    `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9`,
			contains: "eyJh*[REDACTED]",
			excludes: "JIUzI1NiI",
		},
		{
			name:     "private key",
			input:    `PRIVATE_KEY=MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgw`,
			contains: "MIIE*[REDACTED]",
			excludes: "BADANBgk",
		},
		{
			name:     "access key",
			input:    `access_key: AKIAIOSFODNN7EXAMPLE`,
			contains: "AKIA*[REDACTED]",
			excludes: "OSFODNN7",
		},
		{
			name:     "short value not scrubbed",
			input:    `token = abc`,
			contains: "abc", // too short (< 8 chars), should not be redacted
		},
		{
			name:     "no sensitive key",
			input:    `name = "not-a-secret-at-all"`,
			contains: "not-a-secret-at-all", // "name" is not a sensitive key
		},
		{
			name:     "mixed text survives",
			input:    `Config loaded. api_key=sk-long-secret-value-here. Starting server.`,
			contains: "Config loaded",
			excludes: "secret-value-here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubCredentials(tt.input)
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected output to contain %q, got: %s", tt.contains, result)
			}
			if tt.excludes != "" && strings.Contains(result, tt.excludes) {
				t.Errorf("expected output to NOT contain %q, got: %s", tt.excludes, result)
			}
		})
	}
}

func TestScrubCredentials_MultipleInOneLine(t *testing.T) {
	input := `token=sk-abc12345678 and password="hunter2longpassword"`
	result := scrubCredentials(input)
	if strings.Contains(result, "abc12345678") {
		t.Errorf("token not scrubbed: %s", result)
	}
	if strings.Contains(result, "longpassword") {
		t.Errorf("password not scrubbed: %s", result)
	}
}

func TestScrubCredentials_PreservesNonSensitiveContent(t *testing.T) {
	input := `File: main.go
Line 42: fmt.Println("hello world")
Status: 200 OK
Total: 15 items`
	result := scrubCredentials(input)
	if result != input {
		t.Errorf("non-sensitive content was modified:\ninput:  %s\noutput: %s", input, result)
	}
}

func TestScrubCredentials_EmptyInput(t *testing.T) {
	result := scrubCredentials("")
	if result != "" {
		t.Errorf("expected empty, got: %s", result)
	}
}

func TestIsSensitiveEnvKey(t *testing.T) {
	sensitive := []string{
		"API_KEY", "OPENAI_API_KEY", "GITHUB_TOKEN", "DB_PASSWORD",
		"SECRET_KEY", "AWS_ACCESS_KEY", "AUTH_TOKEN", "PRIVATE_KEY",
	}
	for _, key := range sensitive {
		if !IsSensitiveEnvKey(key) {
			t.Errorf("expected %q to be sensitive", key)
		}
	}

	notSensitive := []string{
		"HOME", "PATH", "LANG", "EDITOR", "GOPATH", "USER", "SHELL",
	}
	for _, key := range notSensitive {
		if IsSensitiveEnvKey(key) {
			t.Errorf("expected %q to NOT be sensitive", key)
		}
	}
}

func TestScrubCredentials_StandaloneBearer(t *testing.T) {
	input := `curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig" https://api.example.com`
	result := scrubCredentials(input)
	if strings.Contains(result, "payload") {
		t.Errorf("bearer token not scrubbed: %s", result)
	}
	if !strings.Contains(result, "eyJh*[REDACTED]") {
		t.Errorf("expected redacted bearer, got: %s", result)
	}
}
