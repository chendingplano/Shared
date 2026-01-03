package ApiTypes

import (
	"database/sql"
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

const CallFlowKey ContextKey = "call_flow"
const RequestIDKey ContextKey = "reqID"

type LibConfigDef struct {
	IDStartValue int `mapstructure:"id_start_value"`
	IDIncValue   int `mapstructure:"id_inc_value"`

	SystemTableNames SystemTableNames `mapstructure:"system_table_names"`
	SystemIDs        SystemIDs        `mapstructure:"system_ids"`
}

type SystemTableNames struct {
	TableNameTest          string `mapstructure:"table_name_test"`
	TableNameLoginSessions string `mapstructure:"table_name_login_sessions"`
	TableNameSessionLog    string `mapstructure:"table_name_session_log"`
	TableNameUsers         string `mapstructure:"table_name_users"`
	TableNameActivityLog   string `mapstructure:"table_name_activity_log"`
	TableNameIDMgr         string `mapstructure:"table_name_id_mgr"`
	TableNameEmailStore    string `mapstructure:"table_name_email_store"`
	TableNamePromptStore   string `mapstructure:"table_name_prompt_store"`
	TableNameResources     string `mapstructure:"table_name_resources"`
	TableNameTableManager  string `mapstructure:"table_name_table_manager"`
}

type SystemIDs struct {
	ActivityLogID string `mapstructure:"activity_log_id"`
	PromptStoreID string `mapstructure:"prompt_store_id"`
}

const (
	UserContextKey  ContextKey = "user_name"
	TokenContextKey ContextKey = "token"
)

type DBConfig struct {
	Host       string
	Port       int
	DBType     string
	CreateFlag bool
	UserName   string
	Password   string
	DbName     string
}

type DatabaseInfoDef struct {
	DBType        string
	PGDBName      string
	MySQLDBName   string
	PGDBHandle    *sql.DB
	MySQLDBHandle *sql.DB
	HomeURL       string
}

var DatabaseInfo DatabaseInfoDef
var PG_DB_miner *sql.DB
var MySql_DB_miner *sql.DB
var LibConfig LibConfigDef

func GetDBType() string {
	return DatabaseInfo.DBType
}

func GetActivityLogTableName() string {
	return LibConfig.SystemTableNames.TableNameActivityLog
}

func GetUsersTableName() string {
	return LibConfig.SystemTableNames.TableNameUsers
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

// Maks ure it syncs with svelte/src/lib/types/CommonTypes.ts::JimoResponse
type JimoResponse struct {
	Status     bool        `json:"status"`
	ErrorMsg   string      `json:"error_msg"`
	ReqID      string      `json:"req_id"`
	ResultType string      `json:"result_type"`
	NumRecords int         `json:"num_records"`
	TableName  string      `json:"table_name"`
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
type UserInfo struct {
	UserId          string `json:"user_id"`
	UserName        string `json:"user_name"`
	Password        string `json:"password"`
	UserIdType      string `json:"user_id_type"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	Email           string `json:"email"`
	UserMobile      string `json:"user_mobile,omitempty"`
	UserAddress     string `json:"user_address"`
	Verified        bool   `json:"verfied"`
	Admin           bool   `json:"admin"`
	EmailVisibility bool   `json:"emailVisibility"`
	AuthType        string `json:"auth_type"`
	UserStatus      string `json:"user_status"`
	Avatar          string `json:"avatar"`
	Locale          string `json:"locale"`
	VToken          string `json:"v_token"`
}

// Make sure this struct syncs with tax/web/src/lib/pocketbas-types.ts::UsersRecord
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
	Password        string `json:"password"`
	TokenKey        string `json:"tokenKey"`
	Verified        bool   `json:"verfied"`
}
