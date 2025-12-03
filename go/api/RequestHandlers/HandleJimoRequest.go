package RequestHandlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/stores"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	middleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/labstack/echo/v4"
)

type Operator string
type Field string
type LogicalOperator string

const (
    GreaterThan     Operator = ">"
    GreaterEqual    Operator = ">="
    Equal           Operator = "="
    LessEqual       Operator = "<="
    LessThan        Operator = "<"
    NotEqual        Operator = "<>"
    Contain         Operator = "contain"
    Prefix          Operator = "prefix"
)

const (
    AndOp LogicalOperator = "AND"
    OrOp  LogicalOperator = "OR"
)

type AtomicCondition struct {
    FieldName 		Field  			`json:"field_name"`
    Operator  		Operator 		`json:"operator"`
    Value1    		string 			`json:"value_1"`
    Value2    		string 			`json:"value_2"`
}

type ComplexCondition struct {
    Operator   		LogicalOperator     `json:"operator"`
    Conditions 		[]QueryCondition 	`json:"conditions"`
}

type QueryCondition interface{}

func HandleJimoRequest(c echo.Context) error {
	user_name, err := middleware.IsAuthenticated(c, "SHD_RHD_008")
    if err != nil {
	    log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef {
            LogID:              log_id,         
			ActivityName: 		ApiTypes.ActivityName_JimoRequest,
			ActivityType: 		ApiTypes.ActivityType_AuthFailure,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RHD_065"})

        log.Printf("%s (SHD_RHD_067)", error_msg)
        resp := ApiTypes.JimoResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "SHD_RHD_071",
        }
		return c.JSON(http.StatusBadRequest, resp)
    }

	r := c.Request()

	var req ApiTypes.JimoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	    log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body, log_id:%d", log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.ActivityName_JimoRequest,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RHD_089"})

        log.Printf("***** Alarm:%s (SHD_RHD_091)", error_msg)
        resp := ApiTypes.JimoResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "SHD_PST_095",
        }
		return c.JSON(http.StatusBadRequest, resp)
	}

    // JimoRequest:
    //  {
    //      request_type:   [RequestType_DB_OPR]
    //      action:         [ReqAction_Query]
    //      resource_name:  any string that identifies the resource;
    //      resource_opr:   resource_name + resource_opr identifies the resource record;
    //      conditions:     json string for the conditions;
    //      resource_info:  currently not used;
    //  }
	switch req.RequestType {
	case ApiTypes.RequestType_DB_OPR:
		 switch req.Action {
		 case ApiTypes.ReqAction_Query:
		 	  return HandleDBQuery(c, req, user_name)

         case ApiTypes.ReqAction_Insert:
              return HandleDBInsert(c, req, user_name)

		 default:
			  log_id := sysdatastores.NextActivityLogID()
			  error_msg := fmt.Sprintf("invalid request opr:%s, log_id:%d", req.Action, log_id)
			  log.Printf("***** Alarm:%s (SHD_RHD_121)", error_msg)
			  sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:              log_id,         
				ActivityName: 		ApiTypes.ActivityName_JimoRequest,
				ActivityType: 		ApiTypes.ActivityType_BadRequest,
				AppName: 			ApiTypes.AppName_RequestHandler,
				ModuleName: 		ApiTypes.ModuleName_RequestHandler,
				ActivityMsg: 		&error_msg,
				CallerLoc: 			"SHD_RHD_077"})

        	  log.Printf("***** Alarm:%s", error_msg)
        	  resp := ApiTypes.JimoResponse {
            	Status: false,
            	ErrorMsg: error_msg,
            	Loc: "SHD_RHD_083",
        	  }
			  return c.JSON(http.StatusBadRequest, resp)
		 }
		
	default:
	    log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request type:%s, log_id:%d", req.RequestType, log_id)
		log.Printf("***** Alarm:%s (SHD_RHD_143)", error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.ActivityName_JimoRequest,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RHD_077"})

        log.Printf("***** Alarm:%s", error_msg)
        resp := ApiTypes.JimoResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "SHD_RHD_083",
        }
		return c.JSON(http.StatusBadRequest, resp)
	}
}

func HandleDBQuery(c echo.Context, 
			req ApiTypes.JimoRequest,
			user_name string) error {
	// It is a database query:
	//	- Get db name and table name
	//	- Check access controls
	//	- Get selected field names
	//	- Get the conditions
	//	- Construct the query statement
	//	- Run the query statement
	resource_name := req.ResourceName
	resource_opr := req.ResourceOpr
	resource_def, err := stores.GetResourceDef(resource_name, resource_opr)
	if err != nil {
		error_msg := fmt.Sprintf("resource not found, error:%v ,resource_name:%s, resource_opr:%s", 
				err, resource_name, resource_opr)
		log.Printf("+++++ WARNING:%s (SHD_RHD_165)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_134",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	if resource_def.ResourceDef.ResourceType != ApiTypes.ResourceType_Table {
		error_msg := fmt.Sprintf("incorrect resource type, expecting:%s, actual:%s", 
				ApiTypes.ResourceType_Table, resource_def.ResourceDef.ResourceType)
		log.Printf("***** Alarm:%s (SHD_RHD_193)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_148",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	resource_json := resource_def.ResourceDef.ResourceJSON
	if resource_json == nil {
	    log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("missing resource def, resource_name:%s, log_id:%d", resource_name, log_id)
		log.Printf("***** Alarm:%s (SHD_RHD_208)", error_msg)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.ActivityName_JimoRequest,
			ActivityType: 		ApiTypes.ActivityType_InternalError,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RHD_217"})

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_222",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	db_name := resource_def.ResourceDef.DBName
	table_name := resource_def.ResourceDef.TableName
	if db_name == "" || table_name == ""{
		error_msg := fmt.Sprintf("missing db/table name. Resource name:%s", resource_name)
		log.Printf("***** Alarm:%s (SHD_RHD_219)", error_msg)
		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_226",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	selected_fields := resource_def.SelectedFields
	if selected_fields == nil {
		error_msg := fmt.Sprintf("missing selected fields, resource name:%s", resource_name)
		log.Printf("***** Alarm:%s (SHD_RHD_235)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_254",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	log.Printf("Table:%s.%s, selected fields:%d", db_name, table_name, len(selected_fields))
	conditions := req.Conditions

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB 
	switch db_type {
	case ApiTypes.MysqlName:
		 db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		 db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		 error_msg := fmt.Sprintf("invalid db type:%s:%s:%s", db_type, ApiTypes.MysqlName, ApiTypes.PgName)
		 log.Printf("***** Alarm:%s (SHD_RHD_260)", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_249",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
	}

	var field_names string
	for _, field := range selected_fields {
		if field.FieldName == "" {
			error_msg := fmt.Sprintf("invalid selected field name, resource_name:%s", resource_name)
			log.Printf("***** Alarm:%s (SHD_RHD_273)", error_msg)
			resp := ApiTypes.JimoResponse {
				Status: false,
				ErrorMsg: error_msg,
				Loc: "SHD_RHD_278",
			}
			c.JSON(http.StatusInternalServerError, resp)
			return fmt.Errorf("%s", error_msg)
		} else {
			field_names = field_names + field.FieldName + ","
		}
	}
	query := fmt.Sprintf("SELECT %s FROM %s", field_names, table_name)
	if conditions != "" {
		// There are no cnditions
		where_clause, err := ConstructWhereClause(conditions)
		if err != nil {
			error_msg := fmt.Sprintf("failed constructing query, err:%v", err)
			log.Printf("***** Alarm:%s (SHD_RHD_227)", error_msg)

			resp := ApiTypes.JimoResponse {
				Status: false,	
				ErrorMsg: error_msg,
				Loc: "SHD_RHD_244",
			}
			c.JSON(http.StatusInternalServerError, resp)
			return fmt.Errorf("%s", error_msg)
		}

		if where_clause != "" {
			query = query + " " + where_clause
			log.Printf("Query with where clause:%s (SHD_RHD_252)", query)
		}
	}

	json_data, err := RunQuery(db, query, selected_fields)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("run query failed, err:%v, logid:%d", err, log_id)
		error_msg1 := fmt.Sprintf("run query failed, err:%v, query:%s, " +
				"resource_name:%s, resource_opr:%s", err, query, resource_name, resource_opr)
		log.Printf("***** Alarm:%s (SHD_RHD_297)", error_msg)
	
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:              log_id,
			ActivityName: 		ApiTypes.ActivityName_Query,
			ActivityType: 		ApiTypes.ActivityType_DatabaseError,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg1,
			CallerLoc: 			"SHD_RHD_307"})

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_313",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	resp := ApiTypes.JimoResponse {
		Status: 	true,	
		ErrorMsg: 	"",
		ResultType:	"json",
		Results: 	string(json_data),
		Loc: 		"SHD_RHD_296",
	}

	msg := fmt.Sprintf("query success, query:%s", query)
	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: 		ApiTypes.ActivityName_Query,
		ActivityType: 		ApiTypes.ActivityType_RequestSuccess,
		AppName: 			ApiTypes.AppName_RequestHandler,
		ModuleName: 		ApiTypes.ModuleName_RequestHandler,
		ActivityMsg: 		&msg,
		CallerLoc: 			"SHD_RHD_333"})

	return c.JSON(http.StatusOK, resp)
}

func HandleDBInsert(c echo.Context, 
			req ApiTypes.JimoRequest,
			user_name string) error {
    // This function handles the 'insert' request.
	// The data to be inserted is in req.records
	resource_name 		:= req.ResourceName
	resource_opr 		:= req.ResourceOpr
	resource_def, err 	:= stores.GetResourceDef(resource_name, resource_opr)
	if err != nil {
		error_msg := fmt.Sprintf("%v", err)
		log.Printf("+++++ WARNING:%s (SHD_RHD_329)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_334",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	// The resource type must be 'table'
	if resource_def.ResourceDef.ResourceType != ApiTypes.ResourceType_Table {
		error_msg := fmt.Sprintf("incorrect resource type, expecting:%s, actual:%s", 
				ApiTypes.ResourceType_Table, resource_def.ResourceDef.ResourceType)
		log.Printf("***** Alarm:%s (SHD_RHD_343)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_348",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	db_name := resource_def.ResourceDef.DBName
	table_name := resource_def.ResourceDef.TableName
	if table_name == "" {
		error_msg := fmt.Sprintf("failed get table name. Resource name:%s", resource_name)
		log.Printf("***** Alarm:%s (SHD_RHD_400)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_405",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	log.Printf("Table:%s.%s (SHD_RHD_411)", db_name, table_name)

	// Unmarshal the records
	records_str := req.Records
	var records_json interface{}
    err = json.Unmarshal([]byte(records_str), &records_json)
    if err != nil {
		 error_msg := fmt.Sprintf("invalid record format, err:%v", err)
		 log.Printf("***** Alarm:%s (SHD_RHD_439)", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_443",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
    }

	resp := ApiTypes.JimoResponse {
		Status: 	true,	
		ErrorMsg: 	"",
		ResultType:	"json",
		Loc: 		"SHD_RHD_296",
	}

	// Get field defs
	var field_defs = resource_def.FieldDefs
	if field_defs == nil {
		 error_msg := fmt.Sprintf("missing field_defs, resource_name:%s:%s", resource_name, resource_opr)
		 log.Printf("***** Alarm:%s (SHD_RHD_400)", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_404",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
	}

    // Convert records_json to array of maps
    var records []map[string]interface{}
    
    switch v := records_json.(type) {
    case map[string]interface{}:
         // Single record - wrap in array
         records = []map[string]interface{}{v}

    case []interface{}:
         // Array of records
         for _, item := range v {
            if record, ok := item.(map[string]interface{}); ok {
                records = append(records, record)
            } else {
                return fmt.Errorf("invalid record format in array")
            }
         }

    case []map[string]interface{}:
         records = v

    default:
		 error_msg := fmt.Sprintf("invalid records format, err:%v", v)
		 log.Printf("***** Alarm:%s (SHD_RHD_471)", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_474",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
    }
    
    if len(records) == 0 {
		error_msg := "no records to insert"
		log.Printf("***** Alarm:%s (SHD_RHD_483)", error_msg)
		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_487",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
    }

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB 
	switch db_type {
	case ApiTypes.MysqlName:
		 db = ApiTypes.DatabaseInfo.MySQLDBHandle
		 err = InsertBatch(user_name, db, table_name, resource_name, resource_def, field_defs, records, 30, db_type)

	case ApiTypes.PgName:
		 db = ApiTypes.DatabaseInfo.PGDBHandle
		 err = InsertBatch(user_name, db, table_name, resource_name, resource_def, field_defs, records, 30, db_type)

	default:
		 error_msg := fmt.Sprintf("invalid db type:%s", db_type)
		 log.Printf("***** Alarm:%s (SHD_RHD_465)", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_469",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
	}

	if err != nil {
		 error_msg := fmt.Sprintf("failed insert to db:%v", err)
		 log.Printf("***** Alarm:%s (SHD_RHD_477)", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_481",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
	}
	return c.JSON(http.StatusOK, resp)
}

// RunQuery executes the given query and returns the results as JSON string
func RunQuery(db *sql.DB, query string, selected_fields []ApiTypes.FieldDef) ([]byte, error) {
	log.Printf("RunQuery:%s (SHD_RHD_490)", query)
    rows, err := db.Query(query)
    if err != nil {
        log.Printf("error:%v (SHD_RHD_493)", err)
        return nil, err
    }
    defer rows.Close()

    var results []map[string]interface{}

    for rows.Next() {
        // Create a slice of interface{} to hold the values
        values := make([]interface{}, len(selected_fields))
        valuePtrs := make([]interface{}, len(selected_fields))
        
        for i := range values {
            valuePtrs[i] = &values[i]
        }

        // Scan the row into the value pointers
        if err := rows.Scan(valuePtrs...); err != nil {
            log.Printf("scan error: %v (SHD_RHD_511)", err)
            return nil, err
        }

        // Create a map for this row
        rowMap := make(map[string]interface{})
        
        for i, item := range selected_fields {
			fieldName := item.FieldName
            value := values[i]
            
            // Convert the value based on its data type
            convertedValue := convertValueByType(value, selected_fields[i].DataType)
            rowMap[fieldName] = convertedValue
        }
        
        results = append(results, rowMap)
    }

    if err = rows.Err(); err != nil {
        log.Printf("rows error: %v (SHD_RHD_530)", err)
        return nil, err
    }

    // Convert the results to JSON
    jsonData, err := json.Marshal(results)
    if err != nil {
        log.Printf("JSON marshal error: %v (SHD_RHD_537)", err)
        return nil, err
    }

    return jsonData, nil
}

// Helper function to convert database values to appropriate Go types based on field_data_types
func convertValueByType(value interface{}, dataType string) interface{} {
    if value == nil {
        return nil
    }

    switch dataType {
    case "string", "varchar", "text", "char", "longtext", "mediumtext":
        if val, ok := value.(string); ok {
            return val
        }
        return fmt.Sprintf("%v (SHD_RHD_555)", value)
        
    case "int", "integer", "bigint", "smallint", "tinyint":
        if val, ok := value.([]byte); ok {
            intVal, err := strconv.Atoi(string(val))
            if err == nil {
                return intVal
            }
        }
        if val, ok := value.(int64); ok {
            return int(val)
        }
        if val, ok := value.(int32); ok {
            return int(val)
        }
        if val, ok := value.(int); ok {
            return val
        }
        return value
        
    case "float", "double", "decimal", "numeric":
        if val, ok := value.([]byte); ok {
            floatVal, err := strconv.ParseFloat(string(val), 64)
            if err == nil {
                return floatVal
            }
        }
        if val, ok := value.(float64); ok {
            return val
        }
        if val, ok := value.(float32); ok {
            return float64(val)
        }
        return value
        
    case "bool", "boolean":
        if val, ok := value.([]byte); ok {
            str := string(val)
            boolVal, err := strconv.ParseBool(str)
            if err == nil {
                return boolVal
            }
            // Handle common boolean representations
            return str == "1" || str == "true" || str == "TRUE" || str == "True"
        }
        if val, ok := value.(bool); ok {
            return val
        }
        return value
        
    case "datetime", "timestamp", "date", "time":
        // For datetime types, return as string
        if val, ok := value.(string); ok {
            return val
        }
        if val, ok := value.([]byte); ok {
            return string(val)
        }
        return fmt.Sprintf("%v", value)
        
    default:
        // For unknown types or JSON, return as string or the original value
        if val, ok := value.([]byte); ok {
            return string(val)
        }
        return value
    }
}

func GetFieldStrValue(objMap map[string]interface{}, 
			resource_name string,
			field_name string) (string, error) {
    if value_obj, ok := objMap[field_name]; ok {
		if value_str, ok := value_obj.(string); ok {
			return value_str, nil
		}

		error_msg := fmt.Sprintf("value is not a string, field_name:%s, resource_name:%s (SHD_RHD_633)", 
			field_name, resource_name)
		log.Printf("***** Alarm:%s", error_msg)
		err :=fmt.Errorf("%s", error_msg)
		return "", err
	}

	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (SHD_RHD_640)", 
			field_name, resource_name)
	log.Printf("***** Alarm:%s", error_msg)
	return "", fmt.Errorf("%s", error_msg)
}

func GetFieldStrArrayValue(objMap map[string]interface{}, 
			resource_name string,
			field_name string) ([]string, error) {
	if value_obj, ok := objMap[field_name]; ok {
        if value_slice, ok := value_obj.([]interface{}); ok {
            result := make([]string, len(value_slice))
            for i, v := range value_slice {
                if str, ok := v.(string); ok {
                    result[i] = str
                } else {
                    return nil, fmt.Errorf("element at index %d is not a string, got %T", i, v)
                }
            }
            return result, nil
        }
        return nil, fmt.Errorf("field %s is not a []interface{} type, got %T", field_name, value_obj)
    }

	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (665)", 
			field_name, resource_name)
	log.Printf("+++++ Warn:%s", error_msg)
	return nil, nil
}

func GetFieldAnyArrayValue(objMap map[string]interface{}, 
			resource_name string,
			field_name string) ([]interface{}, error) {
	if value_obj, ok := objMap[field_name]; ok {
        if result, ok := value_obj.([]interface{}); ok {
            return result, nil
        }
        return nil, fmt.Errorf("field %s is not a []interface{} type, got %T", field_name, value_obj)
    }

	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (665)", 
			field_name, resource_name)
	log.Printf("+++++ Warn:%s", error_msg)
	return nil, fmt.Errorf("%s", error_msg)
}

func GetTableName(resource_def map[string]interface{}, 
		resource_name string) (string, string, error) {
	// It retrieves: db_name and table_name. db_name is optional.
	db_name, _ := GetFieldStrValue(resource_def, resource_name, "db_name")

	table_name, err := GetFieldStrValue(resource_def, resource_name, "table_name")
	if err != nil {
		return "", "", err
	}

	log.Printf("Table:%s.%s (SHD_RHD_667)", db_name, table_name)
	return db_name, table_name, nil
}

func GetSelectedFields(json_array []map[string]interface{}, resource_name string) ([]ApiTypes.FieldDef, error) {
    var fieldDefs []ApiTypes.FieldDef
    
    for i, item := range json_array {
        // Marshal back to JSON, then unmarshal to struct
        itemJSON, err := json.Marshal(item)
        if err != nil {
            return nil, fmt.Errorf("failed to marshal item at index %d in resource %s: %w", i, resource_name, err)
        }
        
        var fieldDef ApiTypes.FieldDef
        if err := json.Unmarshal(itemJSON, &fieldDef); err != nil {
            return nil, fmt.Errorf("failed to unmarshal item at index %d in resource %s: %w", i, resource_name, err)
        }
        
        converted := ApiTypes.FieldDef{
            FieldName: fieldDef.FieldName,
            DataType:  fieldDef.DataType,
            Required:  fieldDef.Required,
            Desc:      fieldDef.Desc,
        }
        fieldDefs = append(fieldDefs, converted)
    }
    
    return fieldDefs, nil
}

func ConstructWhereClause(conditions QueryCondition) (string, error) {
    sql, err := buildCondition(conditions)
    if err != nil {
		log.Printf("***** Alarm: failed constructing query (SHD_RHD_281), err:%v", err)
        return "", err
    }
	log.Printf("Where clause constructed:%s (SHD_RHD_742)", sql)
    return sql, nil
}

func buildCondition(condition QueryCondition) (string, error) {
    switch cond := condition.(type) {
    case AtomicCondition:
         return buildAtomicCondition(cond)

    case ComplexCondition:
         return buildComplexCondition(cond)

    case map[string]interface{}:
        // Handle if condition comes as map (from JSON unmarshaling)
        if op, exists := cond["operator"]; exists {
            if _, isLogicalOp := op.(string); isLogicalOp {
                // This is likely a ComplexCondition
                return buildComplexConditionFromMap(cond)
            }
        }
        // Otherwise treat as AtomicCondition
        return buildAtomicConditionFromMap(cond)
    default:
        return "", fmt.Errorf("unknown condition type: %T", condition)
    }
}

func buildAtomicCondition(cond AtomicCondition) (string, error) {
    field := string(cond.FieldName)
    value := cond.Value1
    
    switch cond.Operator {
    case Equal:
         return fmt.Sprintf("%s = '%s'", field, escapeSQL(value)), nil

    case GreaterThan:
         return fmt.Sprintf("%s > '%s'", field, escapeSQL(value)), nil

    case GreaterEqual:
         return fmt.Sprintf("%s >= '%s'", field, escapeSQL(value)), nil

    case LessThan:
         return fmt.Sprintf("%s < '%s'", field, escapeSQL(value)), nil

    case LessEqual:
         return fmt.Sprintf("%s <= '%s'", field, escapeSQL(value)), nil

    case NotEqual:
         return fmt.Sprintf("%s <> '%s'", field, escapeSQL(value)), nil

    case Contain:
         return fmt.Sprintf("%s LIKE '%%%s%%'", field, escapeSQL(value)), nil

    case Prefix:
         return fmt.Sprintf("%s LIKE '%s%%'", field, escapeSQL(value)), nil

    default:
         return "", fmt.Errorf("unsupported operator: %s", cond.Operator)
    }
}

func buildComplexCondition(cond ComplexCondition) (string, error) {
    if len(cond.Conditions) == 0 {
        return "", fmt.Errorf("complex condition cannot be empty")
    }
    
    var parts []string
    for _, subCond := range cond.Conditions {
        part, err := buildCondition(subCond)
        if err != nil {
            return "", err
        }
        parts = append(parts, fmt.Sprintf("(%s)", part))
    }
    
    return strings.Join(parts, fmt.Sprintf(" %s ", string(cond.Operator))), nil
}

// Helper function to handle conditions that come as maps (from JSON)
func buildComplexConditionFromMap(m map[string]interface{}) (string, error) {
    var complexCond ComplexCondition
    
    // Extract operator
    if op, ok := m["operator"].(string); ok {
        complexCond.Operator = LogicalOperator(op)
    } else {
        return "", fmt.Errorf("complex condition must have an operator")
    }
    
    // Extract conditions
    if conditions, ok := m["conditions"].([]interface{}); ok {
        for _, item := range conditions {
            complexCond.Conditions = append(complexCond.Conditions, item)
        }
    } else {
        return "", fmt.Errorf("complex condition must have conditions array")
    }
    
    return buildComplexCondition(complexCond)
}

func buildAtomicConditionFromMap(m map[string]interface{}) (string, error) {
    var atomicCond AtomicCondition
    
    if field, ok := m["field_name"].(string); ok {
        atomicCond.FieldName = Field(field)
    } else {
        return "", fmt.Errorf("atomic condition must have field_name")
    }
    
    if op, ok := m["operator"].(string); ok {
        atomicCond.Operator = Operator(op)
    } else {
        return "", fmt.Errorf("atomic condition must have operator")
    }
    
    if value1, ok := m["value_1"].(string); ok {
        atomicCond.Value1 = value1
    } else {
        return "", fmt.Errorf("atomic condition must have value_1")
    }
    
    if value2, ok := m["value_2"].(string); ok {
        atomicCond.Value2 = value2
    } else {
        atomicCond.Value2 = ""
    }
    
    return buildAtomicCondition(atomicCond)
}

// Basic SQL injection prevention
func escapeSQL(s string) string {
    // This is a basic example - in production, use prepared statements instead
    s = strings.ReplaceAll(s, "'", "''")
    return s
}