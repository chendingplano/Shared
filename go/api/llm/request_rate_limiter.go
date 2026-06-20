package llm

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

var llmRateLimiterLogger = loggerutil.CreateDefaultLogger("LOC_0619143100")

const (
	defaultLLMMaxRequestsPerMinute = 3000
	defaultLLMMaxTokensPerMinute   = 200000
	defaultLLMTokenReservePerCall  = 256
	llmTokenBucketWindowSeconds    = 60
	llmTokenBucketSlots            = 64
)

var globalLLMRequestRateLimiter = &llmRequestRateLimiter{}

// llmModelRateRegistry stores programmatically registered per-model rate limits.
// Values registered here take precedence over the global env-var defaults.
var llmModelRateRegistry struct {
	mu      sync.RWMutex
	configs map[string]llmModelRateEntry
}

type llmModelRateEntry struct {
	requestsPerMinute   int
	tokensPerMinute     int
	tokenReservePerCall int
}

// RegisterLLMModelRateConfig registers per-model RPM, TPM, and token reserve
// with the process-wide rate limiter. Call this before the first LLM request
// for the model (typically when the model config is loaded from .models.toml).
// Zero values are ignored; non-zero values override the global env-var defaults.
func RegisterLLMModelRateConfig(modelName string, rpm, tpm, reservePerCall int) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || (rpm < 1 && tpm < 1 && reservePerCall < 1) {
		return
	}
	llmModelRateRegistry.mu.Lock()
	defer llmModelRateRegistry.mu.Unlock()
	if llmModelRateRegistry.configs == nil {
		llmModelRateRegistry.configs = make(map[string]llmModelRateEntry)
	}
	existing := llmModelRateRegistry.configs[modelName]
	if rpm > 0 {
		existing.requestsPerMinute = rpm
	}
	if tpm > 0 {
		existing.tokensPerMinute = tpm
	}
	if reservePerCall > 0 {
		existing.tokenReservePerCall = reservePerCall
	}
	llmModelRateRegistry.configs[modelName] = existing
}

type llmRequestRateLimiter struct {
	mu        sync.Mutex
	configKey string
	states    map[string]llmRateState
	now       func() time.Time
}

type llmRateState struct {
	requests     []time.Time
	tokenBuckets []llmSecondBucket
}

type llmSecondBucket struct {
	second int64
	tokens int
}

// llmRateLimitWait is returned by reserve when the caller must wait.
type llmRateLimitWait struct {
	delay          time.Duration
	rpmConstrained bool
	tpmConstrained bool
	effectiveRPM   int
	effectiveTPM   int
	tokens         int
	currentRPM     int
	currentTPM     int
}

func (w llmRateLimitWait) reason() string {
	switch {
	case w.rpmConstrained && w.tpmConstrained:
		return "rpm+tpm"
	case w.rpmConstrained:
		return "rpm"
	case w.tpmConstrained:
		return "tpm"
	default:
		return ""
	}
}

type llmRequestRateLimitConfig struct {
	enabled             bool
	requestsPerMinute   int
	tokensPerMinute     int
	tokenReservePerCall int
	requestOverrides    map[string]int
	tokenOverrides      map[string]int
	key                 string
}

func waitForLLMRequestRateLimit(ctx context.Context, modelName string, promptText string, inputText string) error {
	cfg := llmRequestRateLimitConfigFromEnv()
	if !cfg.enabled {
		return nil
	}

	// Apply per-model overrides from the programmatic registry (.models.toml values),
	// which take precedence over env-var overrides.
	reservePerCall := cfg.tokenReservePerCall
	trimmedModel := strings.TrimSpace(modelName)
	llmModelRateRegistry.mu.RLock()
	if entry, ok := llmModelRateRegistry.configs[trimmedModel]; ok {
		if entry.requestsPerMinute > 0 {
			if cfg.requestOverrides == nil {
				cfg.requestOverrides = make(map[string]int)
			}
			cfg.requestOverrides[trimmedModel] = entry.requestsPerMinute
		}
		if entry.tokensPerMinute > 0 {
			if cfg.tokenOverrides == nil {
				cfg.tokenOverrides = make(map[string]int)
			}
			cfg.tokenOverrides[trimmedModel] = entry.tokensPerMinute
		}
		if entry.tokenReservePerCall > 0 {
			reservePerCall = entry.tokenReservePerCall
		}
	}
	llmModelRateRegistry.mu.RUnlock()

	tokens := estimateLLMRequestTokens(promptText, inputText, reservePerCall)
	if tokens < 1 {
		tokens = 1
	}

	for {
		w := globalLLMRequestRateLimiter.reserve(trimmedModel, cfg, tokens)
		if w.delay <= 0 {
			return nil
		}
		llmRateLimiterLogger.Info("(MID_26061903) llm request delayed",
			"model_name", modelName,
			"reason", w.reason(),
			"rpm_limit", w.effectiveRPM,
			"rpm_current", w.currentRPM,
			"tpm_limit", w.effectiveTPM,
			"tpm_current", w.currentTPM,
			"estimated_tokens", w.tokens,
			"delay_ms", w.delay.Milliseconds())
		start := time.Now()
		timer := time.NewTimer(w.delay)
		select {
		case <-timer.C:
			timer.Stop()
			llmRateLimiterLogger.Info("(MID_26061904) llm limit delay elapsed, retrying request admission",
				"model_name", modelName,
				"wait_ms", time.Since(start).Milliseconds())
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

func (l *llmRequestRateLimiter) reserve(modelName string, cfg llmRequestRateLimitConfig, tokens int) llmRateLimitWait {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowTime()
	if l.configKey != cfg.key || l.states == nil {
		l.configKey = cfg.key
		l.states = map[string]llmRateState{}
	}
	state := l.states[modelName]

	effectiveRPM := cfg.requestsPerMinute
	if override, ok := cfg.requestOverrides[modelName]; ok && override > 0 {
		effectiveRPM = override
	}
	effectiveTPM := cfg.tokensPerMinute
	if override, ok := cfg.tokenOverrides[modelName]; ok && override > 0 {
		effectiveTPM = override
	}

	state.prune(now)

	currentRPM := len(state.requests)
	currentTPM := sumLLMTokenBuckets(state.tokenBuckets, now.Unix())

	reservedTokens := tokens
	if effectiveTPM > 0 && reservedTokens > effectiveTPM {
		reservedTokens = effectiveTPM
	}
	if reservedTokens < 1 {
		reservedTokens = 1
	}

	nextRPMAt, rpmBlocked := nextLLMRequestTimeByRPM(state.requests, effectiveRPM, now)
	nextTPMAt, tpmBlocked := nextLLMRequestTimeByTPM(state.tokenBuckets, effectiveTPM, reservedTokens, now)

	nextScheduled := now
	if nextRPMAt.After(nextScheduled) {
		nextScheduled = nextRPMAt
	}
	if nextTPMAt.After(nextScheduled) {
		nextScheduled = nextTPMAt
	}
	if nextScheduled.After(now) {
		l.states[modelName] = state
		return llmRateLimitWait{
			delay:          nextScheduled.Sub(now),
			rpmConstrained: rpmBlocked,
			tpmConstrained: tpmBlocked,
			effectiveRPM:   effectiveRPM,
			effectiveTPM:   effectiveTPM,
			tokens:         tokens,
			currentRPM:     currentRPM,
			currentTPM:     currentTPM,
		}
	}

	state.requests = append(state.requests, now)
	state.tokenBuckets = addLLMTokenBucket(state.tokenBuckets, now.Unix(), reservedTokens)
	l.states[modelName] = state

	return llmRateLimitWait{
		effectiveRPM: effectiveRPM,
		effectiveTPM: effectiveTPM,
		tokens:       tokens,
		currentRPM:   currentRPM,
		currentTPM:   currentTPM,
	}
}

func (l *llmRequestRateLimiter) nowTime() time.Time {
	if l != nil && l.now != nil {
		return l.now()
	}
	return time.Now()
}

func (s *llmRateState) prune(now time.Time) {
	cutoff := now.Add(-time.Minute)
	s.requests = pruneLLMRequestTimes(s.requests, cutoff)
	s.tokenBuckets = pruneLLMTokenBuckets(s.tokenBuckets, now.Unix())
}

func pruneLLMRequestTimes(values []time.Time, cutoff time.Time) []time.Time {
	idx := 0
	for idx < len(values) && !values[idx].After(cutoff) {
		idx++
	}
	if idx == 0 {
		return values
	}
	return append([]time.Time(nil), values[idx:]...)
}

func pruneLLMTokenBuckets(values []llmSecondBucket, nowSec int64) []llmSecondBucket {
	pruned := make([]llmSecondBucket, 0, len(values))
	for _, bucket := range values {
		if bucket.tokens < 1 {
			continue
		}
		if bucket.second > nowSec {
			pruned = append(pruned, bucket)
			continue
		}
		if nowSec-bucket.second >= llmTokenBucketWindowSeconds {
			continue
		}
		pruned = append(pruned, bucket)
	}
	return pruned
}

func nextLLMRequestTimeByRPM(reservations []time.Time, limit int, scheduled time.Time) (time.Time, bool) {
	if limit < 1 {
		return scheduled, false
	}
	cutoff := scheduled.Add(-time.Minute)
	count := 0
	var oldest time.Time
	for _, at := range reservations {
		if !at.After(cutoff) {
			continue
		}
		if count == 0 {
			oldest = at
		}
		count++
	}
	if count < limit {
		return scheduled, false
	}
	return oldest.Add(time.Minute), true
}

func nextLLMRequestTimeByTPM(buckets []llmSecondBucket, limit int, tokens int, scheduled time.Time) (time.Time, bool) {
	if limit < 1 {
		return scheduled, false
	}
	if tokens > limit {
		tokens = limit
	}
	nowSec := scheduled.Unix()
	active := activeLLMTokenBuckets(buckets, nowSec)
	total := 0
	for _, bucket := range active {
		total += bucket.tokens
	}
	if total+tokens <= limit {
		return scheduled, false
	}

	overflow := total + tokens - limit
	released := 0
	for _, bucket := range active {
		released += bucket.tokens
		if released >= overflow {
			return time.Unix(bucket.second+llmTokenBucketWindowSeconds, 0), true
		}
	}
	return time.Unix(nowSec+llmTokenBucketWindowSeconds, 0), true
}

func activeLLMTokenBuckets(buckets []llmSecondBucket, nowSec int64) []llmSecondBucket {
	active := make([]llmSecondBucket, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.tokens < 1 {
			continue
		}
		if bucket.second > nowSec {
			continue
		}
		if nowSec-bucket.second >= llmTokenBucketWindowSeconds {
			continue
		}
		active = append(active, bucket)
	}
	sort.Slice(active, func(i int, j int) bool {
		return active[i].second < active[j].second
	})
	return active
}

func sumLLMTokenBuckets(buckets []llmSecondBucket, nowSec int64) int {
	total := 0
	for _, bucket := range activeLLMTokenBuckets(buckets, nowSec) {
		total += bucket.tokens
	}
	return total
}

func addLLMTokenBucket(buckets []llmSecondBucket, second int64, tokens int) []llmSecondBucket {
	for i := range buckets {
		if buckets[i].second == second {
			buckets[i].tokens += tokens
			return buckets
		}
	}
	buckets = append(buckets, llmSecondBucket{
		second: second,
		tokens: tokens,
	})
	if len(buckets) <= llmTokenBucketSlots {
		return buckets
	}
	sort.Slice(buckets, func(i int, j int) bool {
		return buckets[i].second < buckets[j].second
	})
	return append([]llmSecondBucket(nil), buckets[len(buckets)-llmTokenBucketSlots:]...)
}

func llmRequestRateLimitConfigFromEnv() llmRequestRateLimitConfig {
	enabledRaw := strings.ToLower(strings.TrimSpace(os.Getenv("DOC_PROCESS_LLM_RATE_LIMIT_ENABLED")))
	enabled := enabledRaw != "false" && enabledRaw != "0" && enabledRaw != "no"
	rpm := positiveIntFromEnv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE", defaultLLMMaxRequestsPerMinute)
	tpm := positiveIntFromEnv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE", defaultLLMMaxTokensPerMinute)
	reserve := positiveIntFromEnv("DOC_PROCESS_LLM_TOKEN_RESERVE_PER_CALL", defaultLLMTokenReservePerCall)
	requestOverrides := parsePositiveIntOverrides(os.Getenv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE_OVERRIDES"))
	tokenOverrides := parsePositiveIntOverrides(os.Getenv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE_OVERRIDES"))
	return llmRequestRateLimitConfig{
		enabled:             enabled,
		requestsPerMinute:   rpm,
		tokensPerMinute:     tpm,
		tokenReservePerCall: reserve,
		requestOverrides:    requestOverrides,
		tokenOverrides:      tokenOverrides,
		key: strings.Join([]string{
			enabledRaw,
			strconv.Itoa(rpm),
			strconv.Itoa(tpm),
			strconv.Itoa(reserve),
			formatPositiveIntOverrides(requestOverrides),
			formatPositiveIntOverrides(tokenOverrides),
		}, "|"),
	}
}

func estimateLLMRequestTokens(promptText string, inputText string, reserve int) int {
	total := estimateLLMTextTokens(promptText) + estimateLLMTextTokens(inputText)
	if reserve > 0 {
		total += reserve
	}
	if total < 1 {
		return 1
	}
	return total
}

func estimateLLMTextTokens(input string) int {
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

func resetLLMRequestRateLimiterForTest() {
	globalLLMRequestRateLimiter.mu.Lock()
	defer globalLLMRequestRateLimiter.mu.Unlock()
	globalLLMRequestRateLimiter.configKey = ""
	globalLLMRequestRateLimiter.states = nil
	globalLLMRequestRateLimiter.now = nil
}

func parsePositiveIntOverrides(raw string) map[string]int {
	overrides := map[string]int{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		modelName, valueText, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		modelName = strings.TrimSpace(modelName)
		valueText = strings.TrimSpace(valueText)
		value, err := strconv.Atoi(valueText)
		if err != nil || value < 1 || modelName == "" {
			continue
		}
		overrides[modelName] = value
	}
	return overrides
}

func formatPositiveIntOverrides(values map[string]int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for modelName, value := range values {
		if strings.TrimSpace(modelName) == "" || value < 1 {
			continue
		}
		parts = append(parts, modelName+"="+strconv.Itoa(value))
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
