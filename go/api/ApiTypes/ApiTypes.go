package ApiTypes

import (
	"context"
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

type RequestInfo struct {
	FullURL        string `json:"full_url"`
	PATH           string `json:"path"`
	Scheme         string `json:"scheme"`
	Host           string `json:"host"`
	OriginalScheme string `json:"original_scheme"`
	OriginalHost   string `json:"original_host"`
}

type IDRecordDef struct {
	CrtLogId    int64  `json:"crt_log_id"`
	NumLogIds   int    `json:"num_log_ids"`
	IdBlockSize int    `json:"id_block_size"`
	IdDesc      string `json:"id_desc"`
}

type ContextKey string

const CallFlowKey ContextKey = "jimo_call_flow"
const RequestIDKey ContextKey = "jimo_req_id"

var DBType string
var ProjectDBHandle *sql.DB
var SharedDBHandle *sql.DB
var ProjectMigrationDBHandle *sql.DB
var SharedMigrationDBHandle *sql.DB
var AutotesterDBHandle *sql.DB

type AppInfo struct {
	AppName          string `mapstructure:"app_name"`
	Debug            bool   `mapstructure:"debug"`
	AppPort          int
	AppHost          string
	NeedCreateTables bool   `mapstructure:"need_create_tables"`
	DatabaseType     string `mapstructure:"database_type"`
}

type DatabaseConfig struct {
	Create         bool   `mapstructure:"create"`
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	MaxConnections int    `mapstructure:"max_connections"`

	UserName         string
	Password         string
	ProjectDBName    string
	SharedDBName     string
	AutotesterDBName string

	ProjectDBHandle          *sql.DB
	SharedDBHandle           *sql.DB
	ProjectMigrationDBHandle *sql.DB
	SharedMigrationDBHandle  *sql.DB
	AutotesterDBHandle       *sql.DB
}

type CommonConfigDef struct {
	AppInfo         AppInfo         `mapstructure:"app_info"`
	MySQLConf       DatabaseConfig  `mapstructure:"mysql"`
	PGConf          DatabaseConfig  `mapstructure:"postgres"`
	MigrationConfig MigrationConfig `mapstructure:"migration"`
	Auth            struct {
		JWTSecret            string `mapstructure:"jwt_secret"`
		SessionDurationHours int    `mapstructure:"session_duration_hours"`
	} `mapstructure:"auth"`
}

var CommonConfig CommonConfigDef

type LibConfigDef struct {
	IDStartValue       int  `mapstructure:"id_start_value"`
	IDIncValue         int  `mapstructure:"id_inc_value"`
	AllowDynamicTables bool `mapstructure:"allow_dynamic_tables"`

	SystemTableNames SystemTableNames  `mapstructure:"system_table_names"`
	SystemIDs        SystemIDs         `mapstructure:"system_ids"`
	IconServiceConf  IconServiceConfig `mapstructure:"icon_service"`
}

type SystemTableNames struct {
	TableNameTest            string `mapstructure:"table_name_test"`
	TableNameLoginSessions   string `mapstructure:"table_name_login_sessions"`
	TableNameSessionLog      string `mapstructure:"table_name_session_log"`
	TableNameActivityLog     string `mapstructure:"table_name_activity_log"`
	TableNameIDMgr           string `mapstructure:"table_name_id_mgr"`
	TableNameEmailStore      string `mapstructure:"table_name_email_store"`
	TableNamePromptStore     string `mapstructure:"table_name_prompt_store"`
	TableNameResources       string `mapstructure:"table_name_resources"`
	TableNameTableManager    string `mapstructure:"table_name_table_manager"`
	TableNameAutoTestRuns    string `mapstructure:"table_name_auto_test_runs"`
	TableNameAutoTestResults string `mapstructure:"table_name_auto_test_results"`
	TableNameAutoTestLogs    string `mapstructure:"table_name_auto_test_logs"`
	TableNameDBMigrations    string `mapstructure:"table_name_goose"`
}

type SystemIDs struct {
	ActivityLogID string `mapstructure:"activity_log_id"`
	PromptStoreID string `mapstructure:"prompt_store_id"`
}

type IconServiceConfig struct {
	EnableIconService string `mapstructure:"enable_icon_service"`
	IconDataDir       string `mapstructure:"icon_data_dir"`
}

const (
	UserContextKey  ContextKey = "user_name"
	TokenContextKey ContextKey = "token"
)

var LibConfig LibConfigDef

func GetActivityLogTableName() string {
	return LibConfig.SystemTableNames.TableNameActivityLog
}

func GetSessionsTableName() string {
	return LibConfig.SystemTableNames.TableNameLoginSessions
}

func GetIDMgrTableName() string {
	return LibConfig.SystemTableNames.TableNameIDMgr
}

type IDMgrDef struct {
	IDName    string `json:"id_name"`
	CrtValue  int64  `json:"crt_value"`
	IDDesc    string `json:"id_desc"`
	CallerLoc string `json:"caller_loc"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
}

type ActivityLogDef struct {
	LogID          int64   `json:"log_id"`
	ActivityName   string  `json:"activity_name"`
	ActivityType   string  `json:"activity_type"`
	AppName        string  `json:"app_name"`
	ModuleName     string  `json:"module_name"`
	ActivityMsg    *string `json:"activity_msg"`
	Activity_notes *string `json:"activity_notes"`
	CallerLoc      string  `json:"caller_loc"`
	CreatedAt      *string `json:"created_at"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::FieldDef
type FieldDef struct {
	FieldName   string `json:"field_name"`
	DataType    string `json:"data_type"`
	Required    bool   `json:"required"`
	ReadOnly    bool   `json:"read_only"`
	ElementType string `json:"element_type,omitempty"`
	Desc        string `json:"desc,omitempty"`
}

type JimoRequest struct {
	RequestType string `json:"request_type"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::CondDef
type CondDef struct {
	// Atomic condition fields (used if this is an atomic condition)
	Type      ConditionType `json:"type"` // "atomic", "and", "or", "null"
	FieldName string        `json:"field_name,omitempty"`
	DataType  string        `json:"data_type,omitempty"`
	Opr       string        `json:"opr,omitempty"`
	Value     interface{}   `json:"value,omitempty"`

	// Group condition fields (only used if this is a group condition)
	Conditions []CondDef `json:"conditions,omitempty"` // Nested conditions for groups
}

type ConditionType string

const (
	ConditionTypeAtomic ConditionType = "atomic"
	ConditionTypeAnd    ConditionType = "and"
	ConditionTypeOr     ConditionType = "or"
	ConditionTypeNull   ConditionType = "null"
)

const (
	ResultType_String = "string"
	ResultType_JSON   = "json"
)

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::UpdateDef
type UpdateDef struct {
	FieldName string      `json:"field_name"`
	DataType  string      `json:"data_type"`
	Value     interface{} `json:"value"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::OrderbyDef
type OrderbyDef struct {
	FieldName string `json:"field_name"`
	DataType  string `json:"data_type"`
	IsAsc     bool   `json:"is_asc"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::OnClauseDef
type OnClauseDef struct {
	SourceFieldName string `json:"source_field_name"`
	JoinedFieldName string `json:"joined_field_name"`
	JoinOpr         string `json:"joined_opr"`
	DataType        string `json:"data_type"`
}

const (
	JoinTypeJoin      string = "join"
	JoinTypeLeftJoin  string = "left_join"
	JoinTypeRightJoin string = "right_join"
	JoinTypeInnerJoin string = "inner_join"
)

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::JoinDef
type JoinDef struct {
	FromTableName   string        `json:"from_table_name"`
	JoinedTableName string        `json:"joined_table_name"`
	OnClause        []OnClauseDef `json:"on_clause"`
	JoinType        string        `json:"join_type"`
	SelectedFields  []string      `json:"selected_fields"`
	FromFieldDefs   []FieldDef    `json:"from_field_defs"`
	JoinedFieldDefs []FieldDef    `json:"joined_field_defs"`
	ReadOnly        bool          `json:"read_only"`
	EmbedName       string        `json:"embed_name"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::JimoRequest
type ResourceRequest struct {
	RequestType          string                 `json:"request_type"`
	Action               string                 `json:"action"`
	ResourceName         string                 `json:"resource_name"`
	ResourceType         string                 `json:"resource_type"`
	DBName               string                 `json:"db_name"`
	TableName            string                 `json:"table_name"`
	Condition            CondDef                `json:"condition"`
	JoinDefs             []JoinDef              `json:"join_def"`
	Records              map[string]interface{} `json:"records,omitempty"`
	FieldDefs            []FieldDef             `json:"field_defs"`
	OnConflictCols       []string               `json:"on_conflict_cols"`
	OnConflictUpdateCols []string               `json:"on_conflict_update_cols"`
	FieldNames           []string               `json:"field_names"`
	Loc                  string                 `json:"loc"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::QueryRequest
type QueryRequest struct {
	RequestType string       `json:"request_type"`
	DBName      string       `json:"db_name"`
	TableName   string       `json:"table_name"`
	Condition   CondDef      `json:"condition"`
	JoinDefs    []JoinDef    `json:"join_def"`
	FieldDefs   []FieldDef   `json:"field_defs"`
	FieldNames  []string     `json:"field_names"`
	OrderbyDef  []OrderbyDef `json:"orderby_def"`
	Start       int          `json:"start"`
	PageSize    int          `json:"page_size"`
	Loc         string       `json:"loc"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::InsertRequest
type InsertRequest struct {
	RequestType          string                   `json:"request_type"`
	DBName               string                   `json:"db_name"`
	TableName            string                   `json:"table_name"`
	Records              []map[string]interface{} `json:"records,omitempty"`
	FieldDefs            []FieldDef               `json:"field_defs"`
	OnConflictCols       []string                 `json:"on_conflict_cols"`
	OnConflictUpdateCols []string                 `json:"on_conflict_update_cols"`
	Loc                  string                   `json:"loc"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::UpdateRequest
type UpdateRequest struct {
	RequestType          string                 `json:"request_type"`
	DBName               string                 `json:"db_name"`
	TableName            string                 `json:"table_name"`
	Condition            CondDef                `json:"condition"`
	Record               map[string]interface{} `json:"record"`
	UpdateEntries        []UpdateDef            `json:"update_def,omitempty"`
	FieldDefs            []FieldDef             `json:"field_defs"`
	OnConflictCols       []string               `json:"on_conflict_cols"`
	OnConflictUpdateCols []string               `json:"on_conflict_update_cols"`
	NeedRecord           bool                   `json:"need_record"`
	Loc                  string                 `json:"loc"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::DeleteRequest
type DeleteRequest struct {
	RequestType string     `json:"request_type"`
	DBName      string     `json:"db_name"`
	TableName   string     `json:"table_name"`
	Condition   CondDef    `json:"condition"`
	FieldDefs   []FieldDef `json:"field_defs"`
	Loc         string     `json:"loc"`
}

func IsValidDBType(db_type string) bool {
	return db_type == MysqlName || db_type == PgName
}

type AddPromptResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
	Loc      string `json:"loc,omitempty"`
}

// Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::JimoResponse
type JimoResponse struct {
	Status     bool        `json:"status"`
	ErrorMsg   string      `json:"error_msg"`
	ReqID      string      `json:"req_id"`
	ResultType string      `json:"result_type"`
	NumRecords int         `json:"num_records"`
	TableName  string      `json:"table_name"`
	BaseURL    string      `json:"base_url,omitempty"`
	Results    interface{} `json:"results"`
	ErrorCode  int         `json:"error_code"`
	Loc        string      `json:"loc,omitempty"`
}

type ResourceDef struct {
	ResourceID      int64                  `json:"resource_id"`
	ResourceName    string                 `json:"resource_name"`
	ResourceOpr     string                 `json:"resource_opr"`
	ResourceDesc    string                 `json:"resource_desc"`
	ResourceType    string                 `json:"resource_type"`
	DBName          string                 `json:"db_name"`
	TableName       string                 `json:"table_name"`
	ResourceStatus  string                 `json:"resource_status"`
	ResourceRemarks string                 `json:"resource_remarks"`
	ErrorMsg        string                 `json:"error_msg"`
	ResourceJSON    map[string]interface{} `json:"resource_def"`
	QueryCondsJSON  map[string]interface{} `json:"query_conds"`
}

type ResourceStoreDef struct {
	ResourceDef    ResourceDef
	FieldDefs      []FieldDef
	SelectedFields []FieldDef
}

// Event Related types
// Below are business events. We may want to separate
// business events to a separate file.
type Event struct {
	EventID       int64  `json:"event_id"`
	UserID        string `json:"user_id"`
	AggregateID   string `json:"aggregate_id"`
	AggregateType string `json:"aggregate_type"`
	EventType     string `json:"event_type"`
	EventVersion  int    `json:"event_version"`
	EventData     string `json:"event_data"`
	Metadata      string `json:"metadata"`
}

type StoredEvent struct {
	Event
	OccurredAt time.Time `json:"occurred_at"`
	RecordedAt time.Time `json:"recorded_at"`
}

func (se *StoredEvent) GetOccurredAt() time.Time {
	se.OccurredAt = time.Now()
	return se.OccurredAt
}

type AccountOpened struct {
	StoredEvent
	AccountID string `json:"account_id"`
	Currency  string `json:"currency"`
}

type MoneyDeposited struct {
	StoredEvent
	AccountID string          `json:"account_id"`
	Amount    decimal.Decimal `json:"amount"`
}

// App level types (TBD)
type UserAccount struct {
}

// Make sure this struct syncs with Shared/svelte/src/lib/types/CommonTypes.ts::UserInfo
// SECURITY: Sensitive fields use json:"-" to prevent exposure in API responses
type UserInfo struct {
	UserId                string    `json:"id"`
	UserName              string    `json:"name"`
	Password              string    `json:"-"` // SECURITY: Never expose password hash in API responses
	UserIdType            string    `json:"user_id_type"`
	FirstName             string    `json:"first_name"`
	LastName              string    `json:"last_name"`
	Email                 string    `json:"email"`
	UserMobile            string    `json:"user_mobile,omitempty"`
	UserAddress           string    `json:"user_address"`
	Verified              bool      `json:"verified"`
	Admin                 bool      `json:"admin"`
	IsOwner               bool      `json:"is_owner"`
	EmailVisibility       bool      `json:"email_visibility"`
	AuthType              string    `json:"auth_type"`
	UserStatus            string    `json:"user_status"`
	Avatar                string    `json:"avatar"`
	Locale                string    `json:"locale"`
	OutlookRefreshToken   string    `json:"outlook_refresh_token"` // SECURITY: Never expose OAuth tokens in API responses
	OutlookAccessToken    string    `json:"outlook_access_token"`  // SECURITY: Never expose OAuth tokens in API responses
	OutlookTokenExpiresAt time.Time `json:"outlook_token_expires_at"`
	OutlookSubID          string    `json:"outlook_sub_id"`
	OutlookSubExpiresAt   time.Time `json:"outlook_sub_expires_at"`
	VToken                string    `json:"-"` // SECURITY: Never expose verification tokens in API responses
	VTokenExpiresAt       time.Time `json:"v_token_expires_at"`
	Created               time.Time `json:"created"`
	Updated               time.Time `json:"updated"`
}

// Make sure this struct syncs with tax/web/src/lib/pocketbase-types.ts::UsersRecord
// SECURITY: Sensitive fields use json:"-" to prevent exposure in API responses
type UserInfoPocket struct {
	UserId          string `json:"id"`
	Email           string `json:"email"`
	Admin           bool   `json:"admin"`
	Created         string `json:"created"`
	Updated         string `json:"updated"`
	Avatar          string `json:"avatar"`
	EmailVisibility bool   `json:"emailVisibility"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	IsOwner         bool   `json:"isOwner"`
	Password        string `json:"-"` // SECURITY: Never expose password hash in API responses
	TokenKey        string `json:"-"` // SECURITY: Never expose token key in API responses
	Verified        bool   `json:"verified"`
}

type JimoLogger interface {
	Debug(message string, args ...any)
	Line(message string, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message string, args ...any)
	Trace(message string)
	Close()
}

// RequestContext is a framework-agnostic wrapper for request-scoped data
type RequestContext interface {
	// Context returns the underlying Go context (for deadlines, cancellation, values)
	Context() context.Context
	GetLogger() JimoLogger

	// ReqID returns a unique request ID (guaranteed non-empty)
	ReqID() string
	Close()

	// SetReqID stores the reqID in the context (idempotent)
	SetReqID(reqID string)

	GetCookie(name string) string
	SetCookie(session_id string)
	DeleteCookie(name string) // Clears a cookie by setting MaxAge=-1
	GetUserID() string
	IsAuthenticated() *UserInfo
	FormValue(name string) string
	GetBody() io.ReadCloser
	GetRequest() *http.Request
	Bind(v interface{}) error
	QueryParam(key string) string
	GetUserInfoByEmail(email string) (*UserInfo, bool)
	GetUserInfoByToken(token string) (*UserInfo, bool)
	GetUserInfoByAppToken(token_name string, token string) (*UserInfo, bool)
	GetUserInfoByUserID(user_id string) (*UserInfo, bool)
	MarkUserVerified(email string) error
	UpdateTokenByEmail(email string, token string) error
	UpdateAppTokenByEmail(email string, token_name string, token string) error
	VerifyUserPassword(userInfo *UserInfo, plaintextPassword string) (bool, int, string)
	UpdatePassword(email string, plaintextPassword string) (bool, int, string)
	SendHTMLResp(html_str string) error
	SendJSONResp(status_code int, json_resp map[string]interface{}) error
	JSON(status_code int, json_resp map[string]interface{}) error
	GenerateAuthToken(email string) (string, error)
	Redirect(redirect_url string, status_code int) error
	IsAuthed() bool
	GetCallFlow() string
	PushCallFlow(loc string) string
	PopCallFlow() string

	UpsertUser(
		user_info *UserInfo,
		plain_password string,
		verified bool,
		admin bool,
		is_owner bool,
		email_visibility bool,
		is_update bool) (*UserInfo, error)

	SaveSession(
		login_method string,
		session_id string,
		auth_token string,
		user_name string,
		user_name_type string,
		user_reg_id string,
		user_email string,
		expiry time.Time,
		need_update_user bool) error
}

// Config holds the configuration for the Migrator.
type MigrationConfig struct {
	MigrationsFS  fs.FS
	MigrationsDir string
	TableName     string

	SharedMigrationsFS  string
	SharedMigrationsDir string
	SharedTableName     string

	Verbose         bool
	AllowOutOfOrder bool

	VerboseStr         string `json:"verbose" mapstructure:"verbose"`
	AllowOutOfOrderStr string `json:"allow_outof_order" mapstructure:"allow_outof_order"`
}

// TesterDefinition holds the metadata for a tester as declared in a [[testers]]
// entry in a testers.toml file. It is the authoritative record of a tester's
// identity, purpose, and global enabled/disabled status.
//
// A tester that is disabled here (Enabled = false) will not run in any package,
// regardless of package-level settings. This acts as a global kill switch.
//
// Example testers.toml entry:
//
//	[[testers]]
//	name        = "tester_database"
//	desc        = "Tests database connectivity and basic CRUD operations"
//	purpose     = "validation"
//	type        = "integration"
//	dynamic_tcs = true
//	enabled     = true
//	creator     = "AutoTester Framework"
//	created_at  = "2026-02-20T00:00:00Z"
type TesterDefinition struct {
	// Name is the unique machine-readable identifier.
	// Allowed characters: letters, digits, dashes, underscores. Max 64 chars.
	Name string `toml:"name"`

	// Desc is a short, human-readable description of what the tester tests.
	Desc string `toml:"desc"`

	// Purpose is the tester's intended purpose (e.g. "validation", "regression").
	Purpose string `toml:"purpose"`

	// Type is the tester's category (e.g. "functional", "performance",
	// "compliance", "integration"). Default: "functional".
	Type string `toml:"type"`

	// DynamicTcs indicates whether this tester can generate test cases
	// dynamically at runtime (true) or only uses static hard-coded cases (false).
	DynamicTcs bool `toml:"dynamic_tcs"`

	// Enabled is the global on/off switch for this tester.
	// A nil pointer means the field was absent in TOML — treated as true (default enabled).
	// Set to false to prevent the tester from running in any package.
	Enabled *bool `toml:"enabled"`

	// Remarks holds any additional notes about the tester.
	Remarks string `toml:"remarks"`

	// Creator is the person or team who authored this tester.
	Creator string `toml:"creator"`

	// CreatedAt is the ISO-8601 timestamp of when this tester was created.
	CreatedAt string `toml:"created_at"`
}

// IsEnabled returns the effective enabled status of this tester definition.
// Returns true if Enabled is nil (field absent in TOML → default enabled) or
// if it points to true.
func (td *TesterDefinition) IsEnabled() bool {
	if td.Enabled == nil {
		return true // default: enabled when not specified
	}
	return *td.Enabled
}

// TesterConfig is the configuration for a single tester within a package.
// It controls whether the tester runs and its execution limits.
//
// Example:
//
//	{ name = "tester_database", enable = true, num_tcs = 20, seconds = 60 }
type TesterConfig struct {
	Name    string `toml:"name"`
	Enable  bool   `toml:"enable"` // If false, tester is excluded from the package
	NumTcs  int    `toml:"num_tcs"`
	Seconds int    `toml:"seconds"`
}

type PackageConfig struct {
	Name        string         `toml:"name"`
	Description string         `toml:"description"`
	Enable      bool           `toml:"enable"` // Ignored: packages are always loaded
	Testers     []TesterConfig `toml:"testers"`
}

// MigrationTesterConfig holds the configuration for the MigrationTester.
type AutotesterConfigDef struct {
	// DUT: Isolated test database for migration testing; NEVER production.
	// Database name MUST start with "testonly_" for safety.
	DBType      string `toml:"db_type"`
	DUTDBName   string `toml:"tester_db_name"`
	DUTDBHandle *sql.DB

	MigrationConfig   MigrationConfig `toml:"migration_config"`
	MigrationDBHandle *sql.DB

	NumDynamicCases     int `toml:"dft_num_dynamic_tcs"`   // default: 80
	MaxMigrationsInPool int `toml:"max_migraions_in_pool"` // default: 20

	Testers  []TesterDefinition `toml:"testers"`
	Packages []PackageConfig    `toml:"packages"`
}
