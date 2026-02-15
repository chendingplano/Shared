// Package auth provides rate limiting for authentication endpoints.
// This prevents brute-force attacks on login, signup, and password reset.
package auth

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimitConfig configures the rate limiter behavior
type RateLimitConfig struct {
	// MaxAttempts is the maximum number of attempts allowed within the window
	MaxAttempts int
	// WindowDuration is the time window for counting attempts
	WindowDuration time.Duration
	// BlockDuration is how long to block after exceeding MaxAttempts
	BlockDuration time.Duration
	// KeyFunc extracts the rate limit key from the request (default: IP address)
	KeyFunc func(c echo.Context) string
}

// DefaultRateLimitConfig returns sensible defaults for auth endpoints
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		MaxAttempts:    5,               // 5 attempts
		WindowDuration: 15 * time.Minute, // per 15 minutes
		BlockDuration:  15 * time.Minute, // block for 15 minutes after exceeding
		KeyFunc:        defaultKeyFunc,
	}
}

// StrictRateLimitConfig returns stricter limits for sensitive endpoints
func StrictRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		MaxAttempts:    3,               // 3 attempts
		WindowDuration: 15 * time.Minute, // per 15 minutes
		BlockDuration:  30 * time.Minute, // block for 30 minutes
		KeyFunc:        defaultKeyFunc,
	}
}

// defaultKeyFunc uses IP address as the rate limit key
func defaultKeyFunc(c echo.Context) string {
	// Try X-Forwarded-For first (for proxied requests)
	// SECURITY: X-Forwarded-For can contain multiple IPs when there are multiple proxies.
	// Format: "client, proxy1, proxy2, ..." - we want only the first (client) IP.
	xff := c.Request().Header.Get("X-Forwarded-For")
	if xff != "" {
		// Extract only the first IP (the original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// Fall back to remote address
	return c.RealIP()
}

// rateLimitEntry tracks attempts for a single key
type rateLimitEntry struct {
	attempts    int
	windowStart time.Time
	blockedAt   time.Time
}

// RateLimiter implements a sliding window rate limiter
type RateLimiter struct {
	mu      sync.RWMutex
	entries map[string]*rateLimitEntry
	config  RateLimitConfig
}

// NewRateLimiter creates a new rate limiter with the given config
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateLimitEntry),
		config:  config,
	}
	// Start cleanup goroutine
	go rl.cleanup()
	return rl
}

// cleanup periodically removes expired entries to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, entry := range rl.entries {
			// Check if window has expired
			windowExpired := now.Sub(entry.windowStart) > rl.config.WindowDuration

			// Check if block has expired (or was never blocked)
			blockExpired := entry.blockedAt.IsZero() || now.Sub(entry.blockedAt) > rl.config.BlockDuration

			// Remove entry if both conditions are met
			if windowExpired && blockExpired {
				delete(rl.entries, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks if a request should be allowed and records the attempt
func (rl *RateLimiter) Allow(key string) (bool, int, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.entries[key]

	if !exists {
		// First attempt from this key
		rl.entries[key] = &rateLimitEntry{
			attempts:    1,
			windowStart: now,
		}
		return true, rl.config.MaxAttempts - 1, 0
	}

	// Check if currently blocked
	if !entry.blockedAt.IsZero() && now.Sub(entry.blockedAt) < rl.config.BlockDuration {
		remaining := rl.config.BlockDuration - now.Sub(entry.blockedAt)
		return false, 0, remaining
	}

	// Check if window has expired
	if now.Sub(entry.windowStart) > rl.config.WindowDuration {
		// Reset the window
		entry.attempts = 1
		entry.windowStart = now
		entry.blockedAt = time.Time{}
		return true, rl.config.MaxAttempts - 1, 0
	}

	// Within window - check attempts
	entry.attempts++
	if entry.attempts > rl.config.MaxAttempts {
		// Block this key
		entry.blockedAt = now
		return false, 0, rl.config.BlockDuration
	}

	return true, rl.config.MaxAttempts - entry.attempts, 0
}

// Reset clears the rate limit for a key (e.g., after successful login)
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, key)
}

// Global rate limiters for different auth endpoints
var (
	loginRateLimiter         *RateLimiter
	signupRateLimiter        *RateLimiter
	passwordResetRateLimiter *RateLimiter
	// SECURITY: Per-account rate limiter to prevent distributed brute-force attacks.
	// Even if an attacker uses multiple IPs, they can only attempt a limited number
	// of logins per account.
	accountLockoutRateLimiter *RateLimiter
	rateLimiterOnce           sync.Once
)

// initRateLimiters initializes the global rate limiters
func initRateLimiters() {
	rateLimiterOnce.Do(func() {
		loginRateLimiter = NewRateLimiter(DefaultRateLimitConfig())
		signupRateLimiter = NewRateLimiter(RateLimitConfig{
			MaxAttempts:    10,              // More lenient for signup
			WindowDuration: 1 * time.Hour,
			BlockDuration:  1 * time.Hour,
			KeyFunc:        defaultKeyFunc,
		})
		passwordResetRateLimiter = NewRateLimiter(StrictRateLimitConfig())
		// SECURITY: Per-account lockout - more lenient than IP-based since
		// legitimate users may forget passwords, but strict enough to prevent
		// brute-force attacks on specific accounts.
		accountLockoutRateLimiter = NewRateLimiter(RateLimitConfig{
			MaxAttempts:    10,               // 10 attempts per account
			WindowDuration: 30 * time.Minute, // within 30 minutes
			BlockDuration:  30 * time.Minute, // lock account for 30 minutes
			KeyFunc:        defaultKeyFunc,   // Not used for account lockout (uses email directly)
		})
	})
}

// RateLimitMiddleware creates Echo middleware for rate limiting
func RateLimitMiddleware(rl *RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := rl.config.KeyFunc(c)
			allowed, remaining, retryAfter := rl.Allow(key)

			// Set rate limit headers
			c.Response().Header().Set("X-RateLimit-Remaining", string(rune('0'+remaining)))

			if !allowed {
				c.Response().Header().Set("Retry-After", retryAfter.String())
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"status":      "error",
					"message":     "Too many attempts. Please try again later.",
					"retry_after": retryAfter.Seconds(),
					"loc":         "SHD_RLM_001",
				})
			}

			return next(c)
		}
	}
}

// CheckLoginRateLimit checks if a login attempt is allowed
// Returns (allowed, remainingAttempts, retryAfterDuration)
func CheckLoginRateLimit(ip string) (bool, int, time.Duration) {
	initRateLimiters()
	return loginRateLimiter.Allow(ip)
}

// ResetLoginRateLimit resets the rate limit after successful login
func ResetLoginRateLimit(ip string) {
	initRateLimiters()
	loginRateLimiter.Reset(ip)
}

// CheckSignupRateLimit checks if a signup attempt is allowed
func CheckSignupRateLimit(ip string) (bool, int, time.Duration) {
	initRateLimiters()
	return signupRateLimiter.Allow(ip)
}

// GetLoginRateLimiter returns the login rate limiter for middleware use
func GetLoginRateLimiter() *RateLimiter {
	initRateLimiters()
	return loginRateLimiter
}

// CheckAccountLockout checks if a specific account (email) has exceeded login attempts.
// SECURITY: This protects against distributed brute-force attacks where an attacker
// uses multiple IPs to attack a single account.
// Returns (allowed, remainingAttempts, retryAfterDuration)
func CheckAccountLockout(email string) (bool, int, time.Duration) {
	initRateLimiters()
	// Normalize email to lowercase for consistent rate limiting
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	return accountLockoutRateLimiter.Allow(normalizedEmail)
}

// ResetAccountLockout resets the per-account rate limit after successful login
func ResetAccountLockout(email string) {
	initRateLimiters()
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	accountLockoutRateLimiter.Reset(normalizedEmail)
}

// CheckLoginRateLimits checks both IP-based and account-based rate limits.
// SECURITY: This provides defense in depth against brute-force attacks.
// - IP-based: Prevents a single IP from attempting many logins
// - Account-based: Prevents distributed attacks against a single account
// Returns (allowed, retryAfterDuration, reason)
func CheckLoginRateLimits(ip string, email string) (bool, time.Duration, string) {
	// Check IP-based rate limit first
	ipAllowed, _, ipRetryAfter := CheckLoginRateLimit(ip)
	if !ipAllowed {
		return false, ipRetryAfter, "ip_blocked"
	}

	// Check account-based rate limit
	accountAllowed, _, accountRetryAfter := CheckAccountLockout(email)
	if !accountAllowed {
		return false, accountRetryAfter, "account_locked"
	}

	return true, 0, ""
}

// ResetLoginRateLimits resets both IP and account rate limits after successful login
func ResetLoginRateLimits(ip string, email string) {
	ResetLoginRateLimit(ip)
	ResetAccountLockout(email)
}
