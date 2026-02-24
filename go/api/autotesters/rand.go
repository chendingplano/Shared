package autotesters

import (
	"math/rand"
	"time"
)

// NewSeededRand creates a new rand.Rand instance with the given seed.
// Use this to ensure deterministic random number generation for reproducible test runs.
func NewSeededRand(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

// AutoGenerateSeed returns a seed based on the current timestamp.
// This is used when no explicit seed is provided, producing non-deterministic runs.
func AutoGenerateSeed() int64 {
	return time.Now().UnixNano()
}

// RandomString generates a random alphanumeric string of the given length.
// Useful for generating unique identifiers or test data.
func RandomString(r *rand.Rand, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
	}
	return string(b)
}

// RandomEmail generates a random email address for testing.
func RandomEmail(r *rand.Rand) string {
	domains := []string{"example.com", "test.org", "mail.io", "demo.net"}
	domain := domains[r.Intn(len(domains))]
	return RandomString(r, 8) + "@" + domain
}

// RandomInt returns a random integer in the range [min, max].
func RandomInt(r *rand.Rand, min, max int) int {
	if min >= max {
		return min
	}
	return r.Intn(max-min+1) + min
}

// RandomChoice selects a random element from the given slice.
// Returns the zero value if the slice is empty.
func RandomChoice[T any](r *rand.Rand, items []T) T {
	var zero T
	if len(items) == 0 {
		return zero
	}
	return items[r.Intn(len(items))]
}

// Shuffle randomly reorders the elements in the given slice in place.
func Shuffle[T any](r *rand.Rand, items []T) {
	rand.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}
