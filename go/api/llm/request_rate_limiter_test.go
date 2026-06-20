package llm

import (
	"testing"
	"time"
)

func TestLLMRequestRateLimiter_AllowsBurstWithinRequestsPerMinuteBudget(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := &llmRequestRateLimiter{
		now: func() time.Time { return now },
	}
	cfg := llmRequestRateLimitConfig{
		enabled:           true,
		requestsPerMinute: 3,
		tokensPerMinute:   100000,
		key:               "rpm-burst",
	}

	for i := 1; i <= 3; i++ {
		wait := limiter.reserve("deepseek-v4-flash", cfg, 1)
		if wait.delay > 0 {
			t.Fatalf("request %d delay=%s, want no delay while within RPM budget", i, wait.delay)
		}
	}

	wait := limiter.reserve("deepseek-v4-flash", cfg, 1)
	if wait.delay != 60*time.Second {
		t.Fatalf("fourth request delay=%s, want 60s", wait.delay)
	}
	if !wait.rpmConstrained {
		t.Fatalf("fourth request rpmConstrained=%v, want true", wait.rpmConstrained)
	}
}

func TestLLMRequestRateLimiter_AllowsBurstWithinTokensPerMinuteBudget(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := &llmRequestRateLimiter{
		now: func() time.Time { return now },
	}
	cfg := llmRequestRateLimitConfig{
		enabled:           true,
		requestsPerMinute: 100000,
		tokensPerMinute:   90,
		key:               "tpm-burst",
	}

	for i := 1; i <= 3; i++ {
		wait := limiter.reserve("deepseek-v4-flash", cfg, 30)
		if wait.delay > 0 {
			t.Fatalf("request %d delay=%s, want no delay while within TPM budget", i, wait.delay)
		}
	}

	wait := limiter.reserve("deepseek-v4-flash", cfg, 1)
	if wait.delay != 60*time.Second {
		t.Fatalf("fourth request delay=%s, want 60s", wait.delay)
	}
	if !wait.tpmConstrained {
		t.Fatalf("fourth request tpmConstrained=%v, want true", wait.tpmConstrained)
	}
}

func TestLLMRequestRateLimiter_TPMWaitUsesOverflowNotWholeRequest(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := &llmRequestRateLimiter{
		configKey: "tpm-overflow",
		now: func() time.Time { return now },
		states: map[string]llmRateState{
			"deepseek-v4-flash": {
				tokenBuckets: []llmSecondBucket{
					{second: 41, tokens: 30},
					{second: 100, tokens: 60},
				},
			},
		},
	}
	cfg := llmRequestRateLimitConfig{
		enabled:           true,
		requestsPerMinute: 100000,
		tokensPerMinute:   100,
		key:               "tpm-overflow",
	}

	wait := limiter.reserve("deepseek-v4-flash", cfg, 20)
	if wait.delay != 1*time.Second {
		t.Fatalf("delay=%s, want 1s because oldest 30-token bucket covers 10-token overflow", wait.delay)
	}
	if !wait.tpmConstrained {
		t.Fatalf("tpmConstrained=%v, want true", wait.tpmConstrained)
	}
}

func TestLLMRequestRateLimiter_TPMWaitCanSpanMultipleBuckets(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := &llmRequestRateLimiter{
		configKey: "tpm-multi-bucket",
		now: func() time.Time { return now },
		states: map[string]llmRateState{
			"deepseek-v4-flash": {
				tokenBuckets: []llmSecondBucket{
					{second: 41, tokens: 5},
					{second: 42, tokens: 5},
					{second: 43, tokens: 5},
					{second: 100, tokens: 85},
				},
			},
		},
	}
	cfg := llmRequestRateLimitConfig{
		enabled:           true,
		requestsPerMinute: 100000,
		tokensPerMinute:   100,
		key:               "tpm-multi-bucket",
	}

	wait := limiter.reserve("deepseek-v4-flash", cfg, 15)
	if wait.delay != 3*time.Second {
		t.Fatalf("delay=%s, want 3s because 3 oldest buckets must expire to cover overflow", wait.delay)
	}
	if !wait.tpmConstrained {
		t.Fatalf("tpmConstrained=%v, want true", wait.tpmConstrained)
	}
}

func TestLLMRequestRateLimiter_AdmitsRequestAfterBucketExpires(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := &llmRequestRateLimiter{
		now: func() time.Time { return now },
	}
	cfg := llmRequestRateLimitConfig{
		enabled:           true,
		requestsPerMinute: 100000,
		tokensPerMinute:   100,
		key:               "tpm-expiry",
	}

	if wait := limiter.reserve("deepseek-v4-flash", cfg, 100); wait.delay != 0 {
		t.Fatalf("initial delay=%s, want 0", wait.delay)
	}
	if wait := limiter.reserve("deepseek-v4-flash", cfg, 1); wait.delay != 60*time.Second {
		t.Fatalf("blocked delay=%s, want 60s", wait.delay)
	}

	now = now.Add(60 * time.Second)
	if wait := limiter.reserve("deepseek-v4-flash", cfg, 1); wait.delay != 0 {
		t.Fatalf("post-expiry delay=%s, want 0", wait.delay)
	}
}
