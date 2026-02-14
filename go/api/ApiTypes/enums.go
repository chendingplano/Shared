package ApiTypes

const (
	ActivityType_AuthSuccess           string = "auth_success"
	ActivityType_AuthFailure           string = "auth_failure"
	ActivityType_BadRequest            string = "bad_request"
	ActivityType_ConfigError           string = "config_error"
	ActivityType_BadEmail              string = "bad_email"
	ActivityType_DatabaseError         string = "db_error"
	ActivityType_Failed                string = "failed"
	ActivityType_GitHubAuth            string = "github_auth"
	ActivityType_InvalidPassword       string = "invalid_password"
	ActivityType_InvalidToken          string = "invalid_token"
	ActivityType_InvalidEmail          string = "invalid_email"
	ActivityType_InternalError         string = "internal_error"
	ActivityType_MissHomeURL           string = "miss_home_url"
	ActivityType_RequestSuccess        string = "request_success"
	ActivityType_Redirect              string = "redirect"
	ActivityType_SetCookie             string = "set_cookie"
	ActivityType_SentEmail             string = "sent_email"
	ActivityType_SignupSuccess         string = "signup_success"
	ActivityType_Success               string = "success"
	ActivityType_UnverifiedEmail       string = "unverified_email"
	ActivityType_UserCreated           string = "user_created"
	ActivityType_UserLoginSuccess      string = "user_login_success"
	ActivityType_UserNotAuthed         string = "user_not_authed"
	ActivityType_UserNotFound          string = "user_not_found"
	ActivityType_UserExist             string = "user_exist"
	ActivityType_UserIsLoggedIn        string = "user_is_logged_in"
	ActivityType_UserLogout            string = "user_logout"
	ActivityType_UserPending           string = "user_pending"
	ActivityType_VerifyEmailSuccess    string = "verify_email_success"
	ActivityType_PasswordUpdateFailure string = "password_update_failure"
	ActivityType_WeakPassword          string = "weak_password"
)

const (
	ActivityName_Auth              string = "oauth"
	ActivityName_AddRecord         string = "add_record"
	ActivityName_JimoRequest       string = "jimo_request"
	ActivityName_Query             string = "query"
	ActivityName_LoadResourceStore string = "load_resource_store"
)

const (
	AppName_Auth           string = "auth"
	AppName_SysDataStore   string = "sys_data_store"
	AppName_RequestHandler string = "request_handler"
	AppName_Stores         string = "stores"
)

const (
	ModuleName_GoogleAuth     string = "google_auth"
	ModuleName_GitHubAuth     string = "github_auth"
	ModuleName_EmailAuth      string = "email_auth"
	ModuleName_Auth           string = "auth"
	ModuleName_AuthMe         string = "auth_me"
	ModuleName_PromptStore    string = "prompt_store"
	ModuleName_RequestHandler string = "request_handler"
	ModuleName_ResourceStore  string = "resource_store"
)

const (
	RequestType_DB_OPR string = "db_opr"
)

const (
	ReqAction_Query  string = "query"
	ReqAction_Insert string = "insert"
	ReqAction_Update string = "update"
	ReqAction_Delete string = "delete"
)

const (
	MysqlName = "mysql" // ✅ exported
	PgName    = "pg"    // ✅ exported
)

const (
	ResourceType_Table string = "table"
)

// Make sure sync the changes to src/lib/types/CommonTypes.ts
const (
	CustomHttpStatus_Success           int = 550
	CustomHttpStatus_ResourceNotFound  int = 551
	CustomHttpStatus_BadRequest        int = 552
	CustomHttpStatus_NotImplementedYet int = 553
	CustomHttpStatus_InternalError     int = 554
	CustomHttpStatus_ServerException   int = 555
	CustomHttpStatus_KeyNotUnique      int = 556
	CustomHttpStatus_NotLoggedIn       int = 557
	CustomHttpStatus_PasswordNotSet    int = 558
)

// Resource Operators
type RscOpr string

const (
	RscOpr_BulkUpdate    RscOpr = "bulk_update"
	RscOpr_BulkAddValues RscOpr = "bulk_add_values"
	RscOpr_Content       RscOpr = "content"
	RscOpr_Create        RscOpr = "create"
	RscOpr_Delete        RscOpr = "delete"
	RscOpr_List          RscOpr = "list"
	RscOpr_ListWithConds RscOpr = "list_with_conds"
	RscOpr_Move          RscOpr = "move"
	RscOpr_Preview       RscOpr = "preview"
	RscOpr_Read          RscOpr = "read"
	RscOpr_ReadWithConds RscOpr = "read_with_conds"
	RscOpr_Update        RscOpr = "update"
	RscOpr_UpdateSingle  RscOpr = "update_single"
	RscOpr_Upload        RscOpr = "upload"
)

// Resource Types
type RscType string

const (
	RscType_Table RscType = "table"
)

const (
	AppTokenName_Invite = "token_invite"
)
