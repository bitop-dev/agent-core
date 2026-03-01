package agent

import (
	"strings"
	"testing"
)

func defaultConfig() LoopDetectionConfig {
	return DefaultLoopDetectionConfig()
}

func disabledConfig() LoopDetectionConfig {
	return LoopDetectionConfig{
		NoProgressThreshold:    0,
		PingPongCycles:         0,
		FailureStreakThreshold: 0,
	}
}

// 1. Below threshold → Continue
func TestBelowThresholdDoesNotTrigger(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("echo", `{"msg":"hi"}`, "hello", true)
	d.RecordCall("echo", `{"msg":"hi"}`, "hello", true)
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue, got %v: %s", r.Verdict, r.Message)
	}
}

// 2. No-progress repeat triggers warning at threshold
func TestNoProgressRepeatTriggersWarning(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	for i := 0; i < 3; i++ {
		d.RecordCall("echo", `{"msg":"hi"}`, "hello", true)
	}
	r := d.Check()
	if r.Verdict != VerdictInjectWarning {
		t.Fatalf("expected InjectWarning, got %v: %s", r.Verdict, r.Message)
	}
	if !strings.Contains(r.Message, "no progress") {
		t.Errorf("expected 'no progress' in message, got: %s", r.Message)
	}
}

// 3. Same input but different output → no trigger (progress detected)
func TestSameInputDifferentOutputDoesNotTrigger(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("echo", `{"msg":"hi"}`, "result_1", true)
	d.RecordCall("echo", `{"msg":"hi"}`, "result_2", true)
	d.RecordCall("echo", `{"msg":"hi"}`, "result_3", true)
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue, got %v: %s", r.Verdict, r.Message)
	}
}

// 4. Warning then continued loop → HardStop
func TestWarningThenContinuedLoopTriggersHardStop(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	for i := 0; i < 3; i++ {
		d.RecordCall("echo", `{"msg":"hi"}`, "same", true)
	}
	r := d.Check()
	if r.Verdict != VerdictInjectWarning {
		t.Fatalf("expected InjectWarning first, got %v", r.Verdict)
	}
	// One more identical call
	d.RecordCall("echo", `{"msg":"hi"}`, "same", true)
	r = d.Check()
	if r.Verdict != VerdictHardStop {
		t.Fatalf("expected HardStop, got %v: %s", r.Verdict, r.Message)
	}
	if !strings.Contains(r.Message, "no progress") {
		t.Errorf("expected 'no progress' in message, got: %s", r.Message)
	}
}

// 5. Ping-pong detection
func TestPingPongTriggersWarning(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	// 2 cycles: A-B-A-B
	d.RecordCall("tool_a", `{"x":1}`, "out_a", true)
	d.RecordCall("tool_b", `{"y":2}`, "out_b", true)
	d.RecordCall("tool_a", `{"x":1}`, "out_a", true)
	d.RecordCall("tool_b", `{"y":2}`, "out_b", true)
	r := d.Check()
	if r.Verdict != VerdictInjectWarning {
		t.Fatalf("expected InjectWarning, got %v: %s", r.Verdict, r.Message)
	}
	if !strings.Contains(r.Message, "Ping-pong") {
		t.Errorf("expected 'Ping-pong' in message, got: %s", r.Message)
	}
}

// 6. Ping-pong with progress does not trigger
func TestPingPongWithProgressDoesNotTrigger(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("tool_a", `{"x":1}`, "out_a_1", true)
	d.RecordCall("tool_b", `{"y":2}`, "out_b_1", true)
	d.RecordCall("tool_a", `{"x":1}`, "out_a_2", true) // different output
	d.RecordCall("tool_b", `{"y":2}`, "out_b_2", true) // different output
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue, got %v: %s", r.Verdict, r.Message)
	}
}

// 7. Consecutive failure streak
func TestFailureStreakTriggersWarning(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("shell", `{"cmd":"bad1"}`, "error: not found 1", false)
	d.RecordCall("shell", `{"cmd":"bad2"}`, "error: not found 2", false)
	d.RecordCall("shell", `{"cmd":"bad3"}`, "error: not found 3", false)
	r := d.Check()
	if r.Verdict != VerdictInjectWarning {
		t.Fatalf("expected InjectWarning, got %v: %s", r.Verdict, r.Message)
	}
	if !strings.Contains(r.Message, "failed 3 consecutive") {
		t.Errorf("expected 'failed 3 consecutive' in message, got: %s", r.Message)
	}
}

// 8. Failure streak resets on success
func TestFailureStreakResetsOnSuccess(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("shell", `{"cmd":"bad"}`, "err", false)
	d.RecordCall("shell", `{"cmd":"bad"}`, "err", false)
	d.RecordCall("shell", `{"cmd":"good"}`, "ok", true) // resets
	d.RecordCall("shell", `{"cmd":"bad"}`, "err", false)
	d.RecordCall("shell", `{"cmd":"bad"}`, "err", false)
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue, got %v: %s", r.Verdict, r.Message)
	}
}

// 9. All thresholds zero → disabled
func TestAllDisabledNeverTriggers(t *testing.T) {
	d := NewLoopDetector(disabledConfig())
	for i := 0; i < 20; i++ {
		d.RecordCall("echo", `{"msg":"hi"}`, "same", true)
	}
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue, got %v: %s", r.Verdict, r.Message)
	}
}

// 10. Mixed tools → no false positive
func TestMixedToolsNoFalsePositive(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("read_file", `{"path":"a.go"}`, "content_a", true)
	d.RecordCall("bash", `{"cmd":"ls"}`, "file_list", true)
	d.RecordCall("write_file", `{"path":"b.go"}`, "written", true)
	d.RecordCall("read_file", `{"path":"c.go"}`, "content_c", true)
	d.RecordCall("bash", `{"cmd":"go test"}`, "ok", true)
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue, got %v: %s", r.Verdict, r.Message)
	}
}

// 11. UTF-8 boundary safety
func TestHashOutputUTF8BoundarySafe(t *testing.T) {
	// Chinese chars are 3 bytes each; 1366 chars = 4098 bytes
	cjk := strings.Repeat("文", 1366)
	if len(cjk) <= outputHashPrefixBytes {
		t.Fatal("test string too short")
	}
	// Should not panic
	h1 := hashOutput(cjk)

	// Different content → different hash
	cjk2 := strings.Repeat("字", 1366)
	h2 := hashOutput(cjk2)
	if h1 == h2 {
		t.Error("expected different hashes for different content")
	}

	// Mixed ASCII + CJK at boundary
	mixed := strings.Repeat("a", 4094) + "文文" // 4094 + 6 = 4100 bytes
	h3 := hashOutput(mixed)
	if h3 == 0 {
		t.Error("expected non-zero hash")
	}
}

// 12. Reset clears all state
func TestResetClearsState(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	for i := 0; i < 3; i++ {
		d.RecordCall("echo", `{"msg":"hi"}`, "same", true)
	}
	r := d.Check()
	if r.Verdict != VerdictInjectWarning {
		t.Fatalf("expected InjectWarning before reset, got %v", r.Verdict)
	}

	d.Reset()

	// After reset, same pattern needs to build up again
	d.RecordCall("echo", `{"msg":"hi"}`, "same", true)
	d.RecordCall("echo", `{"msg":"hi"}`, "same", true)
	r = d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue after reset, got %v: %s", r.Verdict, r.Message)
	}
}

// 13. Failure streak across different tools doesn't cross-contaminate
func TestFailureStreakPerTool(t *testing.T) {
	d := NewLoopDetector(defaultConfig())
	d.RecordCall("tool_a", `{}`, "err", false)
	d.RecordCall("tool_a", `{}`, "err", false)
	d.RecordCall("tool_b", `{}`, "err", false)
	d.RecordCall("tool_b", `{}`, "err", false)
	r := d.Check()
	if r.Verdict != VerdictContinue {
		t.Fatalf("expected Continue (neither tool hit 3), got %v: %s", r.Verdict, r.Message)
	}
}
