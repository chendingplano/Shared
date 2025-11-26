package ApiTypes

const (
	ActivityType_AuthSuccess			string = "auth_success"
	ActivityType_AuthFailure 			string = "auth_failure"
	ActivityType_BadRequest				string = "bad_request"
	ActivityType_ConfigError			string = "config_error"
	ActivityType_BadEmail				string = "bad_email"
	ActivityType_DatabaseError 			string = "db_error"
	ActivityType_GitHubAuth				string = "github_auth"
	ActivityType_InvalidPassword		string = "invalid_password"
	ActivityType_InvalidToken			string = "invalid_token"
	ActivityType_InvalidEmail			string = "invalid_email"
	ActivityType_InternalError			string = "internal_error"
	ActivityType_MissHomeURL			string = "miss_home_url"
	ActivityType_RequestSuccess			string = "request_success"
	ActivityType_Redirect				string = "redirect"
	ActivityType_SetCookie				string = "set_cookie"
	ActivityType_SentEmail				string = "sent_email"
	ActivityType_SignupSuccess			string = "signup_success"
	ActivityType_UnverifiedEmail		string = "unverified_email"
	ActivityType_UserCreated			string = "user_created"
	ActivityType_UserLoginSuccess		string = "user_login_success"
	ActivityType_UserNotAuthed			string = "user_not_authed"
	ActivityType_UserNotFound 			string = "user_not_found"
	ActivityType_UserExist 				string = "user_exist"
	ActivityType_UserIsLoggedIn			string = "user_is_logged_in"
	ActivityType_UserLogout				string = "user_logout"
	ActivityType_UserPending			string = "user_pending"
	ActivityType_VerifyEmailSuccess		string = "verify_email_success"
)

const (
	Activity_Auth 						string = "oauth"
	Activity_AddRecord			 		string = "add_record"
	Activity_JimoRequest			 	string = "jimo_request"
)

const (
	AppName_Auth 						string = "auth"
	AppName_SysDataStore				string = "sys_data_store"
	AppName_RequestHandler				string = "request_handler"
)

const (
	ModuleName_GoogleAuth 				string = "google_auth"
	ModuleName_GitHubAuth 				string = "github_auth"
	ModuleName_EmailAuth 				string = "email_auth"
	ModuleName_Auth 					string = "auth"
	ModuleName_AuthMe 					string = "auth_me"
	ModuleName_PromptStore				string = "prompt_store"
	ModuleName_RequestHandler			string = "request_handler"
	ModuleName_ResourceStore			string = "resource_store"
)

const (
	RequestType_DB_OPR					string = "db_opr"
)

const (
	ReqAction_Query						string = "query"
	ReqAction_Insert					string = "insert"
)

const (
    MysqlName = "mysql"  // ✅ exported
    PgName    = "pg"     // ✅ exported
)

const (
	ResourceType_Table 					string = "table"
)
