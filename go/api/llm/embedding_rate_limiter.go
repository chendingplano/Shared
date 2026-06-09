package llm

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultEmbeddingMaxRequestsPerMinute = 3000
	defaultEmbeddingMaxTokensPerMinute   = 900000
)

var globalEmbeddingRateLimiter = &embeddingRateLimiter{}

type embeddingRateLimiter struct {
	mu            sync.Mutex
	configKey     string
	nextRequestAt time.Time
	nextTokenAt   time.Time
}

type embeddingRateLimitConfig struct {
	enabled           bool
	requestsPerMinute int
	tokensPerMinute   int
	key               string
}

func waitForEmbeddingRateLimit(ctx context.Context, inputs []string) error {
	cfg := embeddingRateLimitConfigFromEnv()
	if !cfg.enabled {
		return nil
	}
	tokens := estimateEmbeddingTokens(inputs)
	if tokens < 1 {
		tokens = 1
	}
	delay := globalEmbeddingRateLimiter.reserve(cfg, tokens)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *embeddingRateLimiter) reserve(cfg embeddingRateLimitConfig, tokens int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if l.configKey != cfg.key {
		l.configKey = cfg.key
		l.nextRequestAt = time.Time{}
		l.nextTokenAt = time.Time{}
	}

	scheduled := now
	if l.nextRequestAt.After(scheduled) {
		scheduled = l.nextRequestAt
	}
	if l.nextTokenAt.After(scheduled) {
		scheduled = l.nextTokenAt
	}

	requestGap := time.Minute / time.Duration(cfg.requestsPerMinute)
	tokenGap := time.Duration(tokens) * time.Minute / time.Duration(cfg.tokensPerMinute)
	l.nextRequestAt = scheduled.Add(requestGap)
	l.nextTokenAt = scheduled.Add(tokenGap)

	return scheduled.Sub(now)
}

func embeddingRateLimitConfigFromEnv() embeddingRateLimitConfig {
	enabledRaw := strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDING_RATE_LIMIT_ENABLED")))
	enabled := enabledRaw != "false" && enabledRaw != "0" && enabledRaw != "no"
	rpm := positiveIntFromEnv("EMBEDDING_MAX_REQUESTS_PER_MINUTE", defaultEmbeddingMaxRequestsPerMinute)
	tpm := positiveIntFromEnv("EMBEDDING_MAX_TOKENS_PER_MINUTE", defaultEmbeddingMaxTokensPerMinute)
	return embeddingRateLimitConfig{
		enabled:           enabled,
		requestsPerMinute: rpm,
		tokensPerMinute:   tpm,
		key:               enabledRaw + "|" + strconv.Itoa(rpm) + "|" + strconv.Itoa(tpm),
	}
}

func positiveIntFromEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func estimateEmbeddingTokens(inputs []string) int {
	total := 0
	for _, input := range inputs {
		total += estimateEmbeddingTokensForText(input)
	}
	if total < 1 {
		return 1
	}
	return total
}

func estimateEmbeddingTokensForText(input string) int {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0
	}
	runes := utf8.RuneCountInString(input)
	if runes == 0 {
		return 0
	}
	if len(input) != runes {
		return runes
	}
	return (len(input) + 2) / 3
}

func resetEmbeddingRateLimiterForTest() {
	globalEmbeddingRateLimiter.mu.Lock()
	defer globalEmbeddingRateLimiter.mu.Unlock()
	globalEmbeddingRateLimiter.configKey = ""
	globalEmbeddingRateLimiter.nextRequestAt = time.Time{}
	globalEmbeddingRateLimiter.nextTokenAt = time.Time{}
}
