package auth

type AuthInfoDef struct {
	DBType 				string
	HomeURL				string
	SessionTableName	string
	UsersTableName		string
}

var AuthInfo1 AuthInfoDef

func SetAuthInfo(
			db_type string,
			home_url string,
			session_table_name string,
			users_table_name string) {
	AuthInfo1.DBType = db_type
	AuthInfo1.HomeURL = home_url
	AuthInfo1.SessionTableName = session_table_name
	AuthInfo1.UsersTableName = users_table_name
}

func GetAuthInfo1() AuthInfoDef {
	return AuthInfo1
}
