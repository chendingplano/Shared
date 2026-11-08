package sysdatastores

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/chendingplano/shared/go/api/idmanager"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var activityIDManager atomic.Pointer[idmanager.Manager]

// InitActivityLogIDs provisions the activity sequence after shared migrations.
// Call once at startup, before accepting requests or starting log writers.
func InitActivityLogIDs(ctx context.Context, db *sql.DB, requester string) error {
	m, err := idmanager.New(db, requester)
	if err != nil {
		return err
	}
	if err := m.Provision(ctx, idmanager.Sequence{Name: "activity_log_id", Desc: "IDs for activity log", AllocateSize: 1000}); err != nil {
		return err
	}
	activityIDManager.Store(m)
	return nil
}

// NextActivityLogID retains the existing call signature. Failure returns 0 and is
// logged by the manager. The bounded context also bounds shutdown delays.
func NextActivityLogID() int64 {
	m := activityIDManager.Load()
	if m == nil {
		loggerutil.CreateDefaultLogger("20261108-462").Error("Activity ID manager is not initialized")
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, _ := m.Next(ctx, "activity_log_id")
	return id
}
