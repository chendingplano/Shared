package ApiTypes

import (
	"database/sql"
	"time"

	"github.com/shopspring/decimal"
)

type RequestInfo struct {
    FullURL			string		`json:"full_url"`
    PATH 			string     	`json:"path"`
	Scheme			string    	`json:"scheme"`
	Host			string    	`json:"host"`
	OriginalScheme 	string    	`json:"original_scheme"`
	OriginalHost 	string    	`json:"original_host"`
}

type IDRecordDef struct {
	CrtLogId 		int64		`json:"crt_log_id"`
	NumLogIds 		int 		`json:"num_log_ids"`
	IdBlockSize     int 		`json:"id_block_size"`
	IdDesc 			string 		`json:"id_desc"`
}

type ContextKey string

type LibConfigDef struct {
	SystemTableNames struct {
	TableName_Sessions 		string `mapstructure:"table_name_login_sessions"`
	TableName_SessionLog 	string `mapstructure:"table_name_session_log"`
	TableName_Users 		string `mapstructure:"table_name_users"`
	TableName_IDMgr 		string `mapstructure:"table_name_id_mgr"`
	TableName_ActivityLog 	string `mapstructure:"table_name_activity_log"`
	TableName_EmailStore 	string `mapstructure:"table_name_email_store"`
	TableName_PromptStore 	string `mapstructure:"table_name_prompt_store"`
	TableName_Resources		string `mapstructure:"table_name_resources"`
	TableName_Test			string `mapstructure:"table_name_test"`
	} `mapstructure:"system_table_names"`
}

const (
    UserContextKey ContextKey = "user_name"
)

type DBConfig struct {
    Host        string
    Port        int
    DBType      string
    CreateFlag  bool
    UserName    string
    Password    string
    DbName      string
}

type DatabaseInfoDef struct {
    DBType                  string
    PGDBName                string
    MySQLDBName             string
    PGDBHandle             *sql.DB
    MySQLDBHandle          *sql.DB
    HomeURL                 string
}

var DatabaseInfo DatabaseInfoDef
var PG_DB_miner *sql.DB
var MySql_DB_miner *sql.DB
var LibConfig LibConfigDef

func GetDBType() string {
    return DatabaseInfo.DBType
}

func GetActivityLogTableName() string {
    return LibConfig.SystemTableNames.TableName_ActivityLog
}

func GetUsersTableName() string {
    return LibConfig.SystemTableNames.TableName_Users
}

func GetSessionsTableName() string {
    return LibConfig.SystemTableNames.TableName_Sessions
}

func GetIDMgrTableName() string {
    return LibConfig.SystemTableNames.TableName_IDMgr
}

type IDMgrDef struct {
    IDName 			string     	`json:"id_name"`
    CrtValue 		int64 		`json:"crt_value"`
    IDDesc 			string 		`json:"id_desc"`
    CallerLoc       string    	`json:"caller_loc"`
    UpdatedAt 		string    	`json:"updated_at"`
    CreatedAt 		string    	`json:"created_at"`
}

type ActivityLogDef struct {
    LogID          	int64		`json:"log_id"`
    ActivityName    string     	`json:"activity_name"`
	ActivityType 	string    	`json:"activity_type"`
    AppName 	    string     	`json:"app_name"`
    ModuleName 	    string    	`json:"module_name"`
    ActivityMsg	   *string    	`json:"activity_msg"`
    Activity_notes *string     	`json:"activity_notes"`
    CallerLoc       string    	`json:"caller_loc"`
    CreatedAt 	   *string    	`json:"created_at"`
}

type FieldInfo struct {
    FieldName       string      `json:"field_name"`
    DataType        string      `json:"data_type"`
    Required        bool        `json:"required"`
    Desc            string      `json:"desc,omitempty"`
}

type JimoRequest struct {
    RequestType     string      `json:"request_type"`
    Action          string      `json:"action"`
    ResourceName    string      `json:"resource_name"`
    ResourceOpr     string      `json:"resource_opr"`
    Conditions      string      `json:"conditions"`
    ResourceInfo    string      `json:"resource_info"`
    Records         string      `json:"records,omitempty"`
}

func IsValidDBType(db_type string) bool {
    return db_type == MysqlName || db_type == PgName
}

type AddPromptResponse struct {
	Status          bool        `json:"status"`
	ErrorMsg        string      `json:"error_msg"`
	Loc             string      `json:"loc,omitempty"`
}

type JimoResponse struct {
	Status          bool        `json:"status"`
	ErrorMsg        string      `json:"error_msg"`
    ResultType      string      `json:"result_type"`
    Results         string      `json:"results"`
	Loc             string      `json:"loc,omitempty"`
}

type ResourceDef struct {
    ResourceID      string      `json:"resource_id"`
    ResourceName    string      `json:"resource_name"`
    ResourceOpr     string      `json:"resource_opr"`
    ResourceType    string      `json:"resource_type"`
    ResourceDef     interface{} `json:"resource_def"`
    QueryConditions interface{} `json:"query_conditions"`
    ErrorMsg        string      `json:"error_msg"`
    LOC             string      `json:"loc"`
}

// Event Related types
// Below are business events. We may want to separate
// business events to a separate file.
type Event struct {
    EventID         int64        `json:"event_id"`
    UserID          string       `json:"user_id"`
    AggregateID     string       `json:"aggregate_id"`
    AggregateType   string       `json:"aggregate_type"`
    EventType       string       `json:"event_type"`
    EventVersion    int          `json:"event_version"`
    EventData       string       `json:"event_data"`
    Metadata        string       `json:"metadata"`
}

type StoredEvent struct {
    Event
    OccurredAt      time.Time    `json:"occurred_at"`
    RecordedAt      time.Time    `json:"recorded_at"`
}

func (se *StoredEvent) GetOccurredAt() time.Time {
    se.OccurredAt = time.Now() 
    return se.OccurredAt 
}

type AccountOpened struct {
    StoredEvent
    AccountID       string       `json:"account_id"`
    Currency        string       `json:"currency"`
}

type MoneyDeposited struct {
    StoredEvent
    AccountID       string              `json:"account_id"`
    Amount          decimal.Decimal     `json:"amount"`
}

// App level types (TBD)
type UserAccount struct {

}