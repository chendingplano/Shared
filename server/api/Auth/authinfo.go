package auth

type AuthInfoDef struct {
	DBType 				string
	HomeURL				string
	SessionTableName	string
	UsersTableName		string
}

var AuthInfo AuthInfoDef

func SetAuthInfo(
			db_type string,
			home_url string,
			session_table_name string,
			users_table_name string) {
	AuthInfo.DBType = db_type
	AuthInfo.HomeURL = home_url
	AuthInfo.SessionTableName = session_table_name
	AuthInfo.UsersTableName = users_table_name
}

func GetAuthInfo() AuthInfoDef {
	return AuthInfo
}
