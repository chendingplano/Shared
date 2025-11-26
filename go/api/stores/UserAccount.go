package stores

import (
	"database/sql"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type UserAccountStore struct {
    db *sql.DB
}

func (a *UserAccountStore) Apply(event ApiTypes.Event) {
    // Rebuild state by replaying events
}

func (a *UserAccountStore) GetDB() *sql.DB {
	return a.db
}
