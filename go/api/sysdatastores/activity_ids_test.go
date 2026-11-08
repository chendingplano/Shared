package sysdatastores

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/idmanager"
)

func TestActivityLogUsesIDManager(t *testing.T) {
	previous := activityIDManager.Load()
	t.Cleanup(func() { activityIDManager.Store(previous) })
	activityIDManager.Store(nil)
	if id := NextActivityLogID(); id != 0 {
		t.Fatalf("uninitialized allocator returned %d", id)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := idmanager.New(db, "activity-test")
	if err != nil {
		t.Fatal(err)
	}
	activityIDManager.Store(m)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT next_id").WithArgs("activity_log_id").WillReturnRows(sqlmock.NewRows([]string{"next_id", "start_id", "max_id", "allocate_size", "allow_reuse"}).AddRow(15000, 1, idmanager.DefaultMaxID, 1000, false))
	mock.ExpectExec("UPDATE shared.id_mgr").WithArgs(int64(16000), "activity-test", "activity_log_id").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	for want := int64(15000); want < 16000; want++ {
		if id := NextActivityLogID(); id != want {
			t.Fatalf("got %d want %d", id, want)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT next_id").WithArgs("activity_log_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if id := NextActivityLogID(); id != 0 {
		t.Fatalf("failure returned %d", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActivityLogInitializationRequiresPool(t *testing.T) {
	if err := InitActivityLogIDs(context.Background(), nil, "worker"); err == nil {
		t.Fatal("accepted nil pool")
	}
}
