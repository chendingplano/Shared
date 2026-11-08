// Package idmanager allocates positive IDs from named PostgreSQL counters.
// Share one Manager per process and database. Unused cached IDs are deliberately
// abandoned on restart; uniqueness requires preserving the database counter.
package idmanager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	GlobalSequence       = "global"
	DefaultMaxID   int64 = math.MaxInt64 - 1
)

var (
	ErrInvalidInput      = errors.New("invalid ID manager input")
	ErrUnknownSequence   = errors.New("unknown ID sequence")
	ErrProvisionConflict = errors.New("ID sequence provisioning conflict")
	ErrExhausted         = errors.New("ID sequence exhausted")
	ErrInsufficientRange = errors.New("insufficient contiguous ID range")
	ErrDatabase          = errors.New("ID manager database failure")
)

// Range describes NumIDs contiguous IDs beginning at FirstID.
// The zero value is returned with an error on failure.
type Range struct {
	FirstID int64
	NumIDs  int64
}

// Sequence defines a namespace. Zero bounds/size select the documented defaults.
// AllowReuse deliberately permits duplicates, including among outstanding IDs.
type Sequence struct {
	Name         string
	Desc         string
	StartID      int64
	MaxID        int64
	AllocateSize int
	AllowReuse   bool
}

type cachedSequence struct {
	lock      chan struct{}
	next      int64
	remaining int64
}

// Manager owns caches, but never owns or closes the supplied connection pool.
// Its zero value is not usable; construct it with New.
type Manager struct {
	db        *sql.DB
	requester string
	logger    ApiTypes.JimoLogger
	mu        sync.Mutex
	sequences map[string]*cachedSequence
}

// New uses an existing centralized pool. Schema creation and global provisioning
// belong to the shared Goose migration; New performs no database work.
func New(db *sql.DB, requester string) (*Manager, error) {
	logger := loggerutil.CreateDefaultLogger("20261108-731")
	if db == nil || !validText(requester) || strings.TrimSpace(requester) == "" {
		err := fmt.Errorf("%w: pool and nonempty requester are required", ErrInvalidInput)
		logger.Error("ID manager initialization failed", "requester", requester, "error", err)
		return nil, err
	}
	return &Manager{db: db, requester: requester, logger: logger, sequences: make(map[string]*cachedSequence)}, nil
}

func validText(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, '\x00') }

func validateName(name string) error {
	if !validText(name) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 128 {
		return fmt.Errorf("%w: sequence name must contain 1–128 characters", ErrInvalidInput)
	}
	return nil
}

func normalize(seq Sequence) (Sequence, error) {
	if err := validateName(seq.Name); err != nil {
		return seq, err
	}
	if seq.StartID == 0 {
		seq.StartID = 1
	}
	if seq.MaxID == 0 {
		seq.MaxID = DefaultMaxID
	}
	if seq.AllocateSize == 0 {
		seq.AllocateSize = 1
	}
	if seq.StartID < 1 || seq.MaxID < seq.StartID || seq.MaxID > DefaultMaxID || seq.AllocateSize < 1 || int64(seq.AllocateSize) > math.MaxInt32 || !validText(seq.Desc) {
		return seq, fmt.Errorf("%w: invalid sequence bounds, allocation size or description", ErrInvalidInput)
	}
	if seq.AllowReuse && (seq.Name == GlobalSequence || seq.Name == "activity_log_id" || seq.Name == "llm_run_id") {
		return seq, fmt.Errorf("%w: %s must not reuse IDs", ErrInvalidInput, seq.Name)
	}
	return seq, nil
}

// Provision inserts a sequence or verifies its immutable bounds/reuse policy.
// Existing descriptions, batch sizes and counters are never changed.
func (m *Manager) Provision(ctx context.Context, seq Sequence) (err error) {
	defer func() {
		if err != nil {
			m.failure("provision", seq.Name, err)
		}
	}()
	seq, err = normalize(seq)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return databaseError(ctx, "begin provisioning", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO shared.id_mgr
		(id_name, id_desc, next_id, start_id, max_id, allocate_size, allow_reuse, creator)
		VALUES ($1, $2, $3, $3, $4, $5, $6, $7) ON CONFLICT (id_name) DO NOTHING`,
		seq.Name, seq.Desc, seq.StartID, seq.MaxID, seq.AllocateSize, seq.AllowReuse, m.requester)
	if err != nil {
		return databaseError(ctx, "insert sequence", err)
	}
	var start, max int64
	var reuse bool
	if err = tx.QueryRowContext(ctx, `SELECT start_id, max_id, allow_reuse FROM shared.id_mgr WHERE id_name = $1 FOR UPDATE`, seq.Name).Scan(&start, &max, &reuse); err != nil {
		return databaseError(ctx, "verify sequence", err)
	}
	if start != seq.StartID || max != seq.MaxID || reuse != seq.AllowReuse {
		return fmt.Errorf("%w: %s has start=%d max=%d reuse=%t", ErrProvisionConflict, seq.Name, start, max, reuse)
	}
	if err = tx.Commit(); err != nil {
		return databaseError(ctx, "commit provisioning", err)
	}
	inserted, _ := result.RowsAffected()
	m.logger.Info("ID sequence provisioned", "sequence", seq.Name, "requester", m.requester, "created", inserted == 1)
	return nil
}

// Next returns one ID, or 0 with an error.
func (m *Manager) Next(ctx context.Context, idName string) (int64, error) {
	r, err := m.Generate(ctx, idName, 1)
	return r.FirstID, err
}

func (m *Manager) sequence(name string) *cachedSequence {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sequences[name]
	if s == nil {
		s = &cachedSequence{lock: make(chan struct{}, 1)}
		m.sequences[name] = s
	}
	return s
}

// Generate returns exactly numIDs contiguous IDs or Range{} with an error.
// A refill abandons an insufficient old cache only after a confirmed commit.
func (m *Manager) Generate(ctx context.Context, idName string, numIDs int64) (r Range, err error) {
	defer func() {
		if err != nil {
			m.failure("allocate", idName, err)
		}
	}()
	if err = validateName(idName); err != nil {
		return Range{}, err
	}
	if numIDs < 1 {
		return Range{}, fmt.Errorf("%w: count must be positive", ErrInvalidInput)
	}
	if err = ctx.Err(); err != nil {
		return Range{}, err
	}
	s := m.sequence(idName)
	select {
	case s.lock <- struct{}{}:
		defer func() { <-s.lock }()
	case <-ctx.Done():
		return Range{}, ctx.Err()
	}
	// If both select cases were ready, honor cancellation before consuming cache.
	if err = ctx.Err(); err != nil {
		return Range{}, err
	}
	if s.remaining >= numIDs {
		r = Range{s.next, numIDs}
		s.next += numIDs
		s.remaining -= numIDs
		return r, nil
	}
	block, err := m.reserve(ctx, idName, numIDs)
	if err != nil {
		return Range{}, err
	}
	s.next = block.FirstID + numIDs
	s.remaining = block.NumIDs - numIDs
	return Range{block.FirstID, numIDs}, nil
}

func (m *Manager) reserve(ctx context.Context, name string, count int64) (Range, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return Range{}, databaseError(ctx, "begin reservation", err)
	}
	defer tx.Rollback()
	var next, start, max, size int64
	var reuse bool
	err = tx.QueryRowContext(ctx, `SELECT next_id, start_id, max_id, allocate_size, allow_reuse FROM shared.id_mgr WHERE id_name = $1 FOR UPDATE`, name).Scan(&next, &start, &max, &size, &reuse)
	if errors.Is(err, sql.ErrNoRows) {
		return Range{}, fmt.Errorf("%w: %s", ErrUnknownSequence, name)
	}
	if err != nil {
		return Range{}, databaseError(ctx, "read reservation", err)
	}
	// Fail closed if administrative changes violate the non-reuse policy.
	if reuse && (name == GlobalSequence || name == "activity_log_id" || name == "llm_run_id") {
		return Range{}, fmt.Errorf("%w: %s must not reuse IDs", ErrInvalidInput, name)
	}
	available := max - next + 1
	if available < count {
		if !reuse {
			if available == 0 {
				return Range{}, fmt.Errorf("%w: %s", ErrExhausted, name)
			}
			return Range{}, fmt.Errorf("%w: %s has %d IDs left, requested %d", ErrInsufficientRange, name, available, count)
		}
		if count > max-start+1 {
			return Range{}, fmt.Errorf("%w: request exceeds %s range", ErrInsufficientRange, name)
		}
		next, available = start, max-start+1
	}
	reserved := min(max64(size, count), available)
	after := next + reserved
	if reuse && after > max {
		after = start
	}
	if _, err = tx.ExecContext(ctx, `UPDATE shared.id_mgr SET next_id = $1, last_requester = $2, updated_at = CURRENT_TIMESTAMP WHERE id_name = $3`, after, m.requester, name); err != nil {
		return Range{}, databaseError(ctx, "update reservation", err)
	}
	if err = tx.Commit(); err != nil {
		return Range{}, databaseError(ctx, "commit reservation", err)
	}
	m.logger.Info("ID block reserved", "sequence", name, "requester", m.requester, "first_id", next, "num_ids", reserved)
	return Range{next, reserved}, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func databaseError(ctx context.Context, operation string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", operation, ctxErr)
	}
	return fmt.Errorf("%w: %s: %w", ErrDatabase, operation, err)
}

func (m *Manager) failure(operation, name string, err error) {
	m.logger.Error("ID manager operation failed", "operation", operation, "sequence", name, "requester", m.requester, "error", err)
}
