package llm

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

var modelPermitLogger = loggerutil.CreateDefaultLogger("LOC_0619143000")

const (
	defaultModelPermitLimit  = 64
	defaultModelPermitTTLSec = 320
	modelPermitLimitEnv      = "DOC_PROCESS_LLM_MAX_INFLIGHT_PER_MODEL"
	modelPermitTTLSecondsEnv = "DOC_PROCESS_LLM_PERMIT_TTL_SEC"
	modelPermitOverridesEnv  = "DOC_PROCESS_LLM_MAX_INFLIGHT_OVERRIDES"
)

type modelPermitTimer interface {
	Stop() bool
}

type modelPermit interface {
	Release()
}

/*
type modelPermitController interface {
	Acquire(ctx context.Context, modelName string) (modelPermit, error)
}
*/

type modelPermitScheduleFunc func(d time.Duration, fn func()) modelPermitTimer

type timeAfterModelPermitTimer struct {
	timer *time.Timer
}

func (t *timeAfterModelPermitTimer) Stop() bool {
	if t == nil || t.timer == nil {
		return false
	}
	return t.timer.Stop()
}

type modelPermitControllerConfig struct {
	DefaultLimit int
	LeaseTTL     time.Duration
	Overrides    map[string]int
	Schedule     modelPermitScheduleFunc
}

type defaultModelPermitController struct {
	defaultLimit int
	leaseTTL     time.Duration
	overrides    map[string]int
	schedule     modelPermitScheduleFunc

	mu      sync.Mutex
	permits map[string]chan struct{}
}

type acquiredModelPermit struct {
	releaseOnce sync.Once
	timer       modelPermitTimer
	releaseFn   func()
}

func (p *acquiredModelPermit) Release() {
	if p == nil {
		return
	}
	p.releaseOnce.Do(func() {
		if p.timer != nil {
			p.timer.Stop()
		}
		if p.releaseFn != nil {
			p.releaseFn()
		}
	})
}

func newModelPermitController(cfg modelPermitControllerConfig) *defaultModelPermitController {
	defaultLimit := cfg.DefaultLimit
	if defaultLimit < 1 {
		defaultLimit = 1
	}
	leaseTTL := cfg.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultModelPermitTTLSec * time.Second
	}
	schedule := cfg.Schedule
	if schedule == nil {
		schedule = func(d time.Duration, fn func()) modelPermitTimer {
			return &timeAfterModelPermitTimer{timer: time.AfterFunc(d, fn)}
		}
	}
	overrides := make(map[string]int, len(cfg.Overrides))
	for modelName, limit := range cfg.Overrides {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || limit < 1 {
			continue
		}
		overrides[modelName] = limit
	}
	return &defaultModelPermitController{
		defaultLimit: defaultLimit,
		leaseTTL:     leaseTTL,
		overrides:    overrides,
		schedule:     schedule,
		permits:      make(map[string]chan struct{}),
	}
}

func (c *defaultModelPermitController) Acquire(ctx context.Context, modelName string) (modelPermit, error) {
	if c == nil {
		return &acquiredModelPermit{}, nil
	}
	permitCh := c.permitChannel(modelName)

	// Try a non-blocking acquire first; if the pool is full we must wait.
	select {
	case permitCh <- struct{}{}:
	default:
		modelPermitLogger.Info("(MID_26061901) llm request delayed waiting for inflight permit",
			"model_name", modelName,
			"pool_capacity", cap(permitCh))
		start := time.Now()
		select {
		case permitCh <- struct{}{}:
			modelPermitLogger.Info("(MID_26061902) llm inflight permit granted after wait",
				"model_name", modelName,
				"wait_ms", time.Since(start).Milliseconds())
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	permit := &acquiredModelPermit{
		releaseFn: func() {
			select {
			case <-permitCh:
			default:
			}
		},
	}
	permit.timer = c.schedule(c.leaseTTL, permit.Release)
	return permit, nil
}

func (c *defaultModelPermitController) permitChannel(modelName string) chan struct{} {
	modelName = strings.TrimSpace(modelName)
	c.mu.Lock()
	defer c.mu.Unlock()

	if permitCh, ok := c.permits[modelName]; ok {
		return permitCh
	}
	limit := c.defaultLimit
	if modelName != "" {
		if override, ok := c.overrides[modelName]; ok && override > 0 {
			limit = override
		}
	}
	permitCh := make(chan struct{}, limit)
	c.permits[modelName] = permitCh
	return permitCh
}

// registerLimit pre-registers a per-model in-flight concurrency limit. It must
// be called before the first Acquire for the model; if the channel was already
// created the call is silently ignored (channel size is fixed at creation).
func (c *defaultModelPermitController) registerLimit(modelName string, limit int) {
	if c == nil || modelName == "" || limit < 1 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.permits[modelName]; exists {
		return
	}
	if c.overrides == nil {
		c.overrides = make(map[string]int)
	}
	c.overrides[modelName] = limit
}

// RegisterModelInflightLimit pre-registers a per-model in-flight concurrency
// limit with the process-wide default permit controller. Call this before the
// first LLM request for the model (typically when the model config is loaded).
// Values from .models.toml take precedence over the env-var default.
func RegisterModelInflightLimit(modelName string, limit int) {
	defaultOpenAIClientPermitController.registerLimit(strings.TrimSpace(modelName), limit)
}

var defaultOpenAIClientPermitController = newModelPermitController(loadModelPermitControllerConfigFromEnv())

func loadModelPermitControllerConfigFromEnv() modelPermitControllerConfig {
	return modelPermitControllerConfig{
		DefaultLimit: envIntValue(modelPermitLimitEnv, defaultModelPermitLimit, 1),
		LeaseTTL:     time.Duration(envIntValue(modelPermitTTLSecondsEnv, defaultModelPermitTTLSec, 1)) * time.Second,
		Overrides:    parseModelPermitOverrides(os.Getenv(modelPermitOverridesEnv)),
	}
}

func parseModelPermitOverrides(raw string) map[string]int {
	overrides := map[string]int{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		modelName, limitText, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		modelName = strings.TrimSpace(modelName)
		limitText = strings.TrimSpace(limitText)
		limit, err := strconv.Atoi(limitText)
		if err != nil || limit < 1 || modelName == "" {
			continue
		}
		overrides[modelName] = limit
	}
	return overrides
}

func envIntValue(key string, fallback int, min int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	return n
}
