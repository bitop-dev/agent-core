package provider

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// ReliableConfig configures the ReliableProvider behavior.
type ReliableConfig struct {
	MaxRetries    int           // max retry attempts per (provider, model) pair
	BaseBackoff   time.Duration // initial backoff between retries
	MaxBackoff    time.Duration // cap on exponential backoff
	FallbackModels []string    // models to try if the primary model fails
}

// DefaultReliableConfig returns sensible defaults.
func DefaultReliableConfig() ReliableConfig {
	return ReliableConfig{
		MaxRetries:  3,
		BaseBackoff: 1 * time.Second,
		MaxBackoff:  10 * time.Second,
	}
}

// ReliableProvider wraps one or more providers with retry, backoff,
// API key rotation, model fallback, and provider failover.
//
// Three-level failover strategy (from zeroclaw):
//   Outer:  model fallback chain (primary model, then configured alternatives)
//   Middle: provider chain (primary provider, then fallbacks)
//   Inner:  retry loop with exponential backoff
//
// On rate-limit (429): rotate API key and retry.
// On non-retryable error: skip to next provider.
// On context-exceeded: abort immediately (caller should compact).
type ReliableProvider struct {
	providers []namedProvider
	apiKeys   []string
	keyIndex  atomic.Int64
	config    ReliableConfig
}

type namedProvider struct {
	name     string
	provider Provider
}

// failureRecord tracks one failed attempt for the diagnostic trail.
type failureRecord struct {
	Provider string
	Model    string
	Attempt  int
	MaxAttempts int
	Reason   string
	Detail   string
}

func (f failureRecord) String() string {
	return fmt.Sprintf("provider=%s model=%s attempt %d/%d: %s; error=%s",
		f.Provider, f.Model, f.Attempt, f.MaxAttempts, f.Reason, f.Detail)
}

// NewReliable creates a ReliableProvider wrapping a single provider.
func NewReliable(p Provider, cfg ReliableConfig) *ReliableProvider {
	return &ReliableProvider{
		providers: []namedProvider{{name: p.Name(), provider: p}},
		config:    cfg,
	}
}

// NewReliableMulti creates a ReliableProvider wrapping multiple providers in priority order.
func NewReliableMulti(providers []Provider, cfg ReliableConfig) *ReliableProvider {
	named := make([]namedProvider, len(providers))
	for i, p := range providers {
		named[i] = namedProvider{name: p.Name(), provider: p}
	}
	return &ReliableProvider{
		providers: named,
		config:    cfg,
	}
}

// WithAPIKeys sets additional API keys for round-robin rotation on rate-limit errors.
func (rp *ReliableProvider) WithAPIKeys(keys []string) *ReliableProvider {
	rp.apiKeys = keys
	return rp
}

// WithFallbackModels sets the model fallback chain.
func (rp *ReliableProvider) WithFallbackModels(models []string) *ReliableProvider {
	rp.config.FallbackModels = models
	return rp
}

func (rp *ReliableProvider) Name() string {
	if len(rp.providers) > 0 {
		return rp.providers[0].name
	}
	return "reliable"
}

func (rp *ReliableProvider) Capabilities() Capabilities {
	if len(rp.providers) > 0 {
		return rp.providers[0].provider.Capabilities()
	}
	return Capabilities{}
}

// Complete implements Provider with retry, backoff, key rotation, and failover.
//
// For streaming, we attempt the call and if the initial connection fails, we
// retry. If the stream starts successfully but fails mid-stream, we surface
// the error through the event channel (the caller's turn loop handles it).
func (rp *ReliableProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan CompletionEvent, error) {
	// Build model chain: [primary, fallback1, fallback2, ...]
	models := rp.modelChain(req.Model)

	var failures []failureRecord
	maxAttempts := rp.config.MaxRetries + 1

	// Outer: model fallback chain
	for _, currentModel := range models {
		// Middle: provider chain
		for _, np := range rp.providers {

			backoff := rp.config.BaseBackoff

			// Inner: retry loop
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				// Check context before attempting
				if ctx.Err() != nil {
					return nil, fmt.Errorf("context cancelled: %w (attempts:\n%s)",
						ctx.Err(), formatFailures(failures))
				}

				// Clone request with current model
				attemptReq := req
				attemptReq.Model = currentModel

				ch, err := np.provider.Complete(ctx, attemptReq)
				if err != nil {
					// Connection-level failure (before streaming starts)
					class := ClassifyError(err)
					reason := string(class)
					detail := compactErrorDetail(err)

					failures = append(failures, failureRecord{
						Provider:    np.name,
						Model:       currentModel,
						Attempt:     attempt,
						MaxAttempts: maxAttempts,
						Reason:      reason,
						Detail:      detail,
					})

					switch class {
					case ErrorContextFull:
						// Abort immediately — caller should compact and retry
						return nil, fmt.Errorf("context window exceeded: %w", err)

					case ErrorNonRetryable:
						log.Printf("[reliable] %s/%s: non-retryable error, skipping: %s",
							np.name, currentModel, detail)
						break // next provider

					case ErrorRateLimited:
						rp.rotateKey()
						if attempt < maxAttempts {
							wait := rp.computeBackoff(backoff, err)
							log.Printf("[reliable] %s/%s: rate limited, rotating key, retry in %s: %s",
								np.name, currentModel, wait, detail)
							sleepCtx(ctx, wait)
							backoff = rp.nextBackoff(backoff)
							continue
						}

					default: // ErrorRetryable
						if attempt < maxAttempts {
							wait := rp.computeBackoff(backoff, err)
							log.Printf("[reliable] %s/%s: retryable error (attempt %d/%d), retry in %s: %s",
								np.name, currentModel, attempt, maxAttempts, wait, detail)
							sleepCtx(ctx, wait)
							backoff = rp.nextBackoff(backoff)
							continue
						}
					}

					// If we reach here, break to next provider
					break
				}

				// Stream started successfully. Wrap it with error detection.
				// If the stream produces an EventProviderError, the turn loop
				// handles it (we don't retry mid-stream).
				if attempt > 1 || currentModel != req.Model {
					log.Printf("[reliable] %s/%s: recovered (original: %s, attempt %d)",
						np.name, currentModel, req.Model, attempt)
				}
				return ch, nil
			}

			log.Printf("[reliable] %s/%s: exhausted retries, trying next provider",
				np.name, currentModel)
		}
	}

	return nil, fmt.Errorf("all providers/models failed. Attempts:\n%s",
		formatFailures(failures))
}

// modelChain returns [primaryModel, fallback1, fallback2, ...]
func (rp *ReliableProvider) modelChain(model string) []string {
	chain := []string{model}
	for _, fb := range rp.config.FallbackModels {
		if fb != model {
			chain = append(chain, fb)
		}
	}
	return chain
}

// rotateKey advances to the next API key (round-robin).
func (rp *ReliableProvider) rotateKey() {
	if len(rp.apiKeys) > 0 {
		idx := rp.keyIndex.Add(1)
		key := rp.apiKeys[idx%int64(len(rp.apiKeys))]
		log.Printf("[reliable] rotated to key ending ...%s", key[max(0, len(key)-4):])
		for _, np := range rp.providers {
			if kr, ok := np.provider.(KeyRotatable); ok {
				kr.SetAPIKey(key)
			}
		}
	}
}

// computeBackoff returns the wait duration, respecting Retry-After if present.
func (rp *ReliableProvider) computeBackoff(base time.Duration, err error) time.Duration {
	if retryAfter := ParseRetryAfterMs(err); retryAfter > 0 {
		ra := time.Duration(retryAfter) * time.Millisecond
		// Cap at 30s, but use at least the base backoff
		if ra > 30*time.Second {
			ra = 30 * time.Second
		}
		if ra > base {
			return ra
		}
	}
	return base
}

// nextBackoff doubles the backoff, capped at MaxBackoff.
func (rp *ReliableProvider) nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > rp.config.MaxBackoff {
		return rp.config.MaxBackoff
	}
	return next
}

// sleepCtx sleeps for the given duration or until the context is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func compactErrorDetail(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

func formatFailures(failures []failureRecord) string {
	lines := make([]string, len(failures))
	for i, f := range failures {
		lines[i] = "  " + f.String()
	}
	return strings.Join(lines, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
