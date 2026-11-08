package stores

import (
	"database/sql"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func InitSharedStores(db_type string, db *sql.DB) {
	InitResourceStore(db_type, ApiTypes.LibConfig.SystemTableNames.TableNameResources, db)
}
