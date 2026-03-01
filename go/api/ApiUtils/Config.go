package ApiUtils

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func ParseMySQLConfig(
	logger ApiTypes.JimoLogger,
	host string,
	port int,
	create_flag bool) (ApiTypes.DBConfig, error) {
	mysqlConf := ApiTypes.DBConfig{}
	mysqlConf.Host = host
	mysqlConf.Port = port
	mysqlConf.DBType = "mysql"
	mysqlConf.CreateFlag = create_flag
	mysqlConf.UserName = os.Getenv("MYSQL_USER_NAME")
	mysqlConf.Password = os.Getenv("MYSQL_PASSWORD")
	mysqlConf.DbName = os.Getenv("MYSQL_DB_NAME")

	if !create_flag {
		return mysqlConf, nil
	}

	if mysqlConf.UserName == "" {
		err := fmt.Errorf("missing env variable MYSQL_USER_NAME (MID_26022603)")
		return mysqlConf, err
	}

	if mysqlConf.Password == "" {
		err := fmt.Errorf("missing env variable MYSQL_PASSWORD (MID_26022604)")
		return mysqlConf, err
	}

	if mysqlConf.DbName == "" {
		err := fmt.Errorf("missing env variable MYSQL_DB_NAME (MID_26022605)")
		return mysqlConf, err
	}

	return mysqlConf, nil
}

func ParsePGConfig(
	logger ApiTypes.JimoLogger,
	host string,
	port int,
	create_flag bool) (ApiTypes.DBConfig, error) {
	pgConf := ApiTypes.DBConfig{}

	pgConf.Host = host
	pgConf.Port = port
	pgConf.DBType = "pg"
	pgConf.CreateFlag = create_flag
	pgConf.UserName = os.Getenv("PG_USER_NAME")
	pgConf.Password = os.Getenv("PG_PASSWORD")
	pgConf.DbName = os.Getenv("PG_DB_NAME")

	logger.Info("PG config loaded",
		"host", host,
		"port", port,
		"create_flag", create_flag,
		"user_name", pgConf.UserName,
		"db_name", pgConf.DbName)

	if !create_flag {
		return pgConf, nil
	}

	if pgConf.UserName == "" {
		err := fmt.Errorf("missing env variable MYSQL_USER_NAME (MID_26022603)")
		return pgConf, err
	}

	if pgConf.Password == "" {
		err := fmt.Errorf("missing env variable MYSQL_PASSWORD (MID_26022604)")
		return pgConf, err
	}

	if pgConf.DbName == "" {
		err := fmt.Errorf("missing env variable MYSQL_DB_NAME (MID_26022605)")
		return pgConf, err
	}

	return pgConf, nil
}

func ParseDatabaseInfo(
	logger ApiTypes.JimoLogger,
	db_type string,
	pg_db_name string,
	mysql_db_name string,
	pg_db *sql.DB,
	mysql_db *sql.DB) error {

	ApiTypes.DatabaseInfo.DBType = db_type
	ApiTypes.DatabaseInfo.PGDBName = pg_db_name
	ApiTypes.DatabaseInfo.MySQLDBName = mysql_db_name
	ApiTypes.DatabaseInfo.PGDBHandle = pg_db
	ApiTypes.DatabaseInfo.MySQLDBHandle = mysql_db

	if db_type == "" {
		return fmt.Errorf("unable to decode config (MID_26022602)")
	}

	if !ApiTypes.IsValidDBType(db_type) {
		return fmt.Errorf("unsupported database type: %s, Allowed:pg|mysql", db_type)
	}

	return nil
}
