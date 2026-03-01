package agent

import (
	"fmt"
	"hash/fnv"
	"sync"
)

// Maximum bytes of tool output considered when hashing results.
// Keeps hashing fast and bounded for large outputs.
const outputHashPrefixBytes = 4096

// LoopDetectionConfig holds tuning knobs for each detection strategy.
type LoopDetectionConfig struct {
	// Identical (tool + args + output) repetitions before triggering.
	// 0 = disabled. Default: 3.
	NoProgressThreshold int
	// Full A-B cycles before triggering ping-pong detection.
	// 0 = disabled. Default: 2.
	PingPongCycles int
	// Consecutive failures of the *same* tool before triggering.
	// 0 = disabled. Default: 3.
	FailureStreakThreshold int
}

// DefaultLoopDetectionConfig returns the default configuration.
func DefaultLoopDetectionConfig() LoopDetectionConfig {
	return LoopDetectionConfig{
		NoProgressThreshold:    3,
		PingPongCycles:         2,
		FailureStreakThreshold: 3,
	}
}

// DetectionVerdict is the action the caller should take after Check().
type DetectionVerdict int

const (
	// VerdictContinue means no loop detected — proceed normally.
	VerdictContinue DetectionVerdict = iota
	// VerdictInjectWarning means first detection — inject a self-correction prompt.
	VerdictInjectWarning
	// VerdictHardStop means pattern persisted after warning — terminate.
	VerdictHardStop
)

// DetectionResult holds the verdict and, if applicable, the message to inject or the reason to stop.
type DetectionResult struct {
	Verdict DetectionVerdict
	Message string
}

// callRecord captures one completed tool invocation.
type callRecord struct {
	toolName   string
	argsSig    string
	resultHash uint64
	success    bool
}

// LoopDetector tracks tool call patterns and detects unproductive looping.
type LoopDetector struct {
	mu                  sync.Mutex
	config              LoopDetectionConfig
	history             []callRecord
	consecutiveFailures map[string]int
	warningInjected     bool
}

// NewLoopDetector creates a new detector with the given config.
func NewLoopDetector(config LoopDetectionConfig) *LoopDetector {
	return &LoopDetector{
		config:              config,
		consecutiveFailures: make(map[string]int),
	}
}

// RecordCall records a completed tool invocation.
func (d *LoopDetector) RecordCall(toolName, argsSig, output string, success bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.history = append(d.history, callRecord{
		toolName:   toolName,
		argsSig:    argsSig,
		resultHash: hashOutput(output),
		success:    success,
	})

	if success {
		delete(d.consecutiveFailures, toolName)
	} else {
		d.consecutiveFailures[toolName]++
	}
}

// Check evaluates the current history and returns a verdict.
func (d *LoopDetector) Check() DetectionResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	reason := d.checkNoProgressRepeat()
	if reason == "" {
		reason = d.checkPingPong()
	}
	if reason == "" {
		reason = d.checkFailureStreak()
	}

	if reason == "" {
		return DetectionResult{Verdict: VerdictContinue}
	}

	if d.warningInjected {
		return DetectionResult{
			Verdict: VerdictHardStop,
			Message: reason,
		}
	}

	d.warningInjected = true
	return DetectionResult{
		Verdict: VerdictInjectWarning,
		Message: formatWarning(reason),
	}
}

// Reset clears all state (e.g., after compaction or new session).
func (d *LoopDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.history = nil
	d.consecutiveFailures = make(map[string]int)
	d.warningInjected = false
}

// checkNoProgressRepeat detects the same tool + same args + same output hash
// repeated consecutively.
func (d *LoopDetector) checkNoProgressRepeat() string {
	threshold := d.config.NoProgressThreshold
	if threshold == 0 || len(d.history) == 0 {
		return ""
	}

	last := d.history[len(d.history)-1]
	streak := 0
	for i := len(d.history) - 1; i >= 0; i-- {
		r := d.history[i]
		if r.toolName == last.toolName && r.argsSig == last.argsSig && r.resultHash == last.resultHash {
			streak++
		} else {
			break
		}
	}

	if streak >= threshold {
		return fmt.Sprintf(
			"Tool '%s' called %d times with identical arguments and identical results — no progress detected",
			last.toolName, streak,
		)
	}
	return ""
}

// checkPingPong detects two calls alternating (A→B→A→B) with no progress.
func (d *LoopDetector) checkPingPong() string {
	cycles := d.config.PingPongCycles
	if cycles == 0 || len(d.history) < 4 {
		return ""
	}

	n := len(d.history)
	a := d.history[n-2]
	b := d.history[n-1]

	// The two sides must differ
	if a.toolName == b.toolName && a.argsSig == b.argsSig {
		return ""
	}

	minEntries := cycles * 2
	if n < minEntries {
		return ""
	}

	tail := d.history[n-minEntries:]
	for i := 0; i < len(tail)-1; i += 2 {
		pa := tail[i]
		pb := tail[i+1]
		if pa.toolName != a.toolName || pa.argsSig != a.argsSig || pa.resultHash != a.resultHash {
			return ""
		}
		if pb.toolName != b.toolName || pb.argsSig != b.argsSig || pb.resultHash != b.resultHash {
			return ""
		}
	}

	return fmt.Sprintf(
		"Ping-pong loop detected: '%s' and '%s' alternating %d times with no progress",
		a.toolName, b.toolName, cycles,
	)
}

// checkFailureStreak detects the same tool failing consecutively.
func (d *LoopDetector) checkFailureStreak() string {
	threshold := d.config.FailureStreakThreshold
	if threshold == 0 {
		return ""
	}
	for tool, count := range d.consecutiveFailures {
		if count >= threshold {
			return fmt.Sprintf("Tool '%s' failed %d consecutive times", tool, count)
		}
	}
	return ""
}

// hashOutput hashes the first outputHashPrefixBytes of the output.
func hashOutput(output string) uint64 {
	s := output
	if len(s) > outputHashPrefixBytes {
		// Walk back to avoid splitting a multi-byte UTF-8 character
		boundary := outputHashPrefixBytes
		for boundary > 0 && (s[boundary]&0xC0) == 0x80 {
			boundary--
		}
		s = s[:boundary]
	}
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// formatWarning creates the self-correction prompt injected on first detection.
func formatWarning(reason string) string {
	return fmt.Sprintf(
		"IMPORTANT: A loop pattern has been detected in your tool usage. %s. "+
			"You must change your approach: "+
			"(1) Try a different tool or different arguments, "+
			"(2) If polling a process, increase wait time or check if it's stuck, "+
			"(3) If the task cannot be completed, explain why and stop. "+
			"Do NOT repeat the same tool call with the same arguments.",
		reason,
	)
}
