package provider

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider is a test provider that fails a configurable number of times.
type mockProvider struct {
	name           string
	callCount      atomic.Int32
	failUntil      int // fail the first N calls
	failError      string
	responseText   string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Capabilities() Capabilities {
	return Capabilities{NativeToolCalling: true, Streaming: true}
}

func (m *mockProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error) {
	attempt := int(m.callCount.Add(1))
	if attempt <= m.failUntil {
		return nil, fmt.Errorf("%s", m.failError)
	}

	ch := make(chan CompletionEvent, 4)
	go func() {
		defer close(ch)
		ch <- CompletionEvent{Type: EventTextDelta, Text: m.responseText}
		ch <- CompletionEvent{Type: EventUsage, Usage: &Usage{InputTokens: 10, OutputTokens: 5}}
		ch <- CompletionEvent{Type: EventDone, StopReason: "stop"}
	}()
	return ch, nil
}

// drain reads all events from a completion channel and returns them.
func drain(ch <-chan CompletionEvent) []CompletionEvent {
	var events []CompletionEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestReliable_SucceedsWithoutRetry(t *testing.T) {
	mock := &mockProvider{name: "primary", responseText: "ok"}
	rp := NewReliable(mock, DefaultReliableConfig())

	ch, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Text != "ok" {
		t.Errorf("expected text 'ok', got %q", events[0].Text)
	}
	if mock.callCount.Load() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount.Load())
	}
}

func TestReliable_RetriesThenRecovers(t *testing.T) {
	mock := &mockProvider{
		name:         "primary",
		failUntil:    2,
		failError:    "503 Service Unavailable",
		responseText: "recovered",
	}
	cfg := DefaultReliableConfig()
	cfg.BaseBackoff = 1 * time.Millisecond // fast tests
	rp := NewReliable(mock, cfg)

	ch, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch)
	if events[0].Text != "recovered" {
		t.Errorf("expected 'recovered', got %q", events[0].Text)
	}
	if mock.callCount.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", mock.callCount.Load())
	}
}

func TestReliable_NonRetryableSkipsRetries(t *testing.T) {
	primary := &mockProvider{
		name:      "primary",
		failUntil: 999,
		failError: "HTTP 401 Unauthorized",
	}
	fallback := &mockProvider{
		name:         "fallback",
		responseText: "from fallback",
	}
	cfg := DefaultReliableConfig()
	cfg.BaseBackoff = 1 * time.Millisecond
	rp := NewReliableMulti([]Provider{primary, fallback}, cfg)

	ch, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch)
	if events[0].Text != "from fallback" {
		t.Errorf("expected 'from fallback', got %q", events[0].Text)
	}
	// Primary should have been called only once (no retries for non-retryable)
	if primary.callCount.Load() != 1 {
		t.Errorf("expected 1 primary call, got %d", primary.callCount.Load())
	}
}

func TestReliable_ContextExceededAbortsImmediately(t *testing.T) {
	mock := &mockProvider{
		name:      "primary",
		failUntil: 999,
		failError: "maximum context length exceeded",
	}
	fallback := &mockProvider{
		name:         "fallback",
		responseText: "should not reach",
	}
	cfg := DefaultReliableConfig()
	cfg.FallbackModels = []string{"fallback-model"}
	rp := NewReliableMulti([]Provider{mock, fallback}, cfg)

	_, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error for context exceeded")
	}
	if mock.callCount.Load() != 1 {
		t.Errorf("expected 1 call (no retries), got %d", mock.callCount.Load())
	}
	if fallback.callCount.Load() != 0 {
		t.Errorf("expected 0 fallback calls, got %d", fallback.callCount.Load())
	}
}

func TestReliable_FallsBackToNextProvider(t *testing.T) {
	primary := &mockProvider{
		name:      "primary",
		failUntil: 999,
		failError: "500 Internal Server Error",
	}
	fallback := &mockProvider{
		name:         "fallback",
		responseText: "from fallback",
	}
	cfg := DefaultReliableConfig()
	cfg.MaxRetries = 1
	cfg.BaseBackoff = 1 * time.Millisecond
	rp := NewReliableMulti([]Provider{primary, fallback}, cfg)

	ch, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch)
	if events[0].Text != "from fallback" {
		t.Errorf("expected 'from fallback', got %q", events[0].Text)
	}
}

func TestReliable_ModelFallback(t *testing.T) {
	// Provider that fails for "primary-model" but succeeds for "fallback-model"
	mock := &mockProvider{name: "primary"}
	var lastModel string
	// Override with a model-aware mock
	modelAware := &modelAwareProvider{
		name:        "primary",
		failModels:  map[string]bool{"primary-model": true},
		responseText: "fallback worked",
	}

	cfg := DefaultReliableConfig()
	cfg.MaxRetries = 0
	cfg.FallbackModels = []string{"fallback-model"}
	cfg.BaseBackoff = 1 * time.Millisecond
	_ = mock
	_ = lastModel
	rp := NewReliable(modelAware, cfg)

	ch, err := rp.Complete(context.Background(), CompletionRequest{Model: "primary-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch)
	if events[0].Text != "fallback worked" {
		t.Errorf("expected 'fallback worked', got %q", events[0].Text)
	}
}

func TestReliable_AllFailReturnsAggregatedError(t *testing.T) {
	p1 := &mockProvider{name: "p1", failUntil: 999, failError: "p1 error"}
	p2 := &mockProvider{name: "p2", failUntil: 999, failError: "p2 error"}

	cfg := DefaultReliableConfig()
	cfg.MaxRetries = 0
	cfg.BaseBackoff = 1 * time.Millisecond
	rp := NewReliableMulti([]Provider{p1, p2}, cfg)

	_, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	msg := err.Error()
	if !containsAll(msg, "provider=p1", "provider=p2", "p1 error", "p2 error") {
		t.Errorf("error should contain all provider failures, got: %s", msg)
	}
}

func TestReliable_RateLimitTriggersRetry(t *testing.T) {
	mock := &mockProvider{
		name:         "primary",
		failUntil:    1,
		failError:    "429 Too Many Requests rate limit",
		responseText: "recovered after rate limit",
	}
	cfg := DefaultReliableConfig()
	cfg.BaseBackoff = 1 * time.Millisecond
	rp := NewReliable(mock, cfg)

	ch, err := rp.Complete(context.Background(), CompletionRequest{Model: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch)
	if events[0].Text != "recovered after rate limit" {
		t.Errorf("expected recovery, got %q", events[0].Text)
	}
	if mock.callCount.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", mock.callCount.Load())
	}
}

// --- helpers ---

type modelAwareProvider struct {
	name         string
	failModels   map[string]bool
	responseText string
}

func (m *modelAwareProvider) Name() string              { return m.name }
func (m *modelAwareProvider) Capabilities() Capabilities { return Capabilities{Streaming: true} }
func (m *modelAwareProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error) {
	if m.failModels[req.Model] {
		return nil, fmt.Errorf("500 model %s unavailable", req.Model)
	}
	ch := make(chan CompletionEvent, 4)
	go func() {
		defer close(ch)
		ch <- CompletionEvent{Type: EventTextDelta, Text: m.responseText}
		ch <- CompletionEvent{Type: EventDone, StopReason: "stop"}
	}()
	return ch, nil
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
