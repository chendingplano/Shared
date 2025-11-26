package requesthandlers

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
    FieldName Field  `json:"field_name"`
    Operator  Operator `json:"operator"`
    Value1    string `json:"value_1"`
    Value2    string `json:"value_2"`
}

type ComplexCondition struct {
    Operator   LogicalOperator     `json:"operator"`
    Conditions []QueryCondition `json:"conditions"`
}

type QueryCondition interface{}

func HandleJimoRequest(c echo.Context) error {
	user_name, err := middleware.IsAuthenticated(c, "SHD_RHD_008")
    if err != nil {
	    log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d (SHD_RHD_057)", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef {
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_JimoRequest,
			ActivityType: 		ApiTypes.ActivityType_AuthFailure,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RHD_024"})

        log.Printf("%s", error_msg)
        resp := ApiTypes.JimoResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "CWB_RHD_031",
        }
		return c.JSON(http.StatusBadRequest, resp)
    }

	r := c.Request()

	var req ApiTypes.JimoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	    log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("invalid request body, log_id:%d (SHD_RHD_043)", log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_JimoRequest,
			ActivityType: 		ApiTypes.ActivityType_BadRequest,
			AppName: 			ApiTypes.AppName_RequestHandler,
			ModuleName: 		ApiTypes.ModuleName_RequestHandler,
			ActivityMsg: 		&error_msg,
			CallerLoc: 			"SHD_RHD_051"})

        log.Printf("***** Alarm:%s", error_msg)
        resp := ApiTypes.JimoResponse {
            Status: false,
            ErrorMsg: error_msg,
            Loc: "CWB_PST_239",
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
			  error_msg := fmt.Sprintf("invalid request opr:%s, log_id:%d (SHD_RHD_108)", req.Action, log_id)
			  log.Printf("***** Alarm:%s", error_msg)
			  sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
				LogID:              log_id,         
				ActivityName: 		ApiTypes.Activity_JimoRequest,
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
		error_msg := fmt.Sprintf("invalid request type:%s, log_id:%d (SHD_RHD_068)", req.RequestType, log_id)
		log.Printf("***** Alarm:%s", error_msg)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
            LogID:              log_id,         
			ActivityName: 		ApiTypes.Activity_JimoRequest,
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

	if resource_def.ResourceType != ApiTypes.ResourceType_Table {
		error_msg := fmt.Sprintf("incorrect resource type, expecting:%s, actual:%s", 
				ApiTypes.ResourceType_Table, resource_def.ResourceType)
		log.Printf("***** Alarm:%s", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_148",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	resource_json := resource_def.ResourceDef
	if resource_json == nil {
		error_msg := fmt.Sprintf("missing resource def, resource_name:%s", resource_name)
		log.Printf("***** Alarm:%s", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_161",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	objMap, ok := resource_json.(map[string]interface{})
	if !ok {
		error_msg := fmt.Sprintf("invalid resource def (json), resource_name:%s", resource_name)
		log.Printf("***** Alarm:%s", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_176",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	db_name, table_name, err1 := GetTableName(objMap, resource_name)
	if err1 != nil {
		error_msg := fmt.Sprintf("%v", err1)
		log.Printf("***** Alarm:%s", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_240",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	selected_fields, field_data_types, err1 := GetSelectedFields(objMap, resource_name)
	if err1 != nil {
		error_msg := fmt.Sprintf("%v", err1)
		log.Printf("***** Alarm:%s", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_254",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	log.Printf("Table:%s.%s, selected fields:%s", db_name, table_name, selected_fields)
	conditions := req.Conditions

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB 
	switch db_type {
	case ApiTypes.MysqlName:
		 db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		 db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		 error_msg := fmt.Sprintf("invalid db type:%s", db_type)
		 log.Printf("***** Alarm:%s", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_249",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
	}

    field_names := strings.Join(selected_fields, ",")
	query := fmt.Sprintf("SELECT %s FROM %s", field_names, table_name)
	if conditions != "" {
		// There are no cnditions
		where_clause, err := ConstructWhereClause(conditions)
		if err != nil {
			error_msg := fmt.Sprintf("failed constructing query, err:%v", err1)
			log.Printf("***** Alarm:%s", error_msg)

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

	json_data, err := runQuery(db, query, selected_fields, field_data_types)
	if err != nil {
		error_msg := fmt.Sprintf("run query failed, err:%v", err)
		log.Printf("***** Alarm:%s", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_287",
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
		error_msg := fmt.Sprintf("resource not found, error:%v ,resource_name:%s, resource_opr:%s", 
				err, resource_name, resource_opr)
		log.Printf("+++++ WARNING:%s (SHD_RHD_329)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_334",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	if resource_def.ResourceType != ApiTypes.ResourceType_Table {
		error_msg := fmt.Sprintf("incorrect resource type, expecting:%s, actual:%s", 
				ApiTypes.ResourceType_Table, resource_def.ResourceType)
		log.Printf("***** Alarm:%s (SHD_RHD_343)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_348",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	resource_def_str := resource_def.ResourceDef
	if resource_def_str == nil {
		error_msg := fmt.Sprintf("missing resource def, resource_name:%s", resource_name)
		log.Printf("***** Alarm:%s (SHD_RHD_357)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_362",
		}
		c.JSON(http.StatusBadRequest, resp)
		return fmt.Errorf("%s", error_msg)
	}

	resource_def_json, ok := resource_def_str.(map[string]interface{})
	if !ok {
		error_msg := fmt.Sprintf("invalid resource def (json), resource_name:%s", resource_name)
		log.Printf("***** Alarm:%s (SHD_RHD_371)", error_msg)

		resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_376",
		}
		c.JSON(http.StatusInternalServerError, resp)
		return fmt.Errorf("%s", error_msg)
	}

	db_name, table_name, err1 := GetTableName(resource_def_json, resource_name)
	if err1 != nil {
		error_msg := fmt.Sprintf("%v", err1)
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

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB 
	switch db_type {
	case ApiTypes.MysqlName:
		 db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		 db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		 error_msg := fmt.Sprintf("invalid db type:%s", db_type)
		 log.Printf("***** Alarm:%s", error_msg)
		 resp := ApiTypes.JimoResponse {
			Status: false,	
			ErrorMsg: error_msg,
			Loc: "SHD_RHD_249",
		 }
		 c.JSON(http.StatusInternalServerError, resp)
		 return fmt.Errorf("%s", error_msg)
	}

	records_str 	:= req.Records
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

	err = insertIntoDB(db, table_name, insert_field_names, insert_data_types, records_json)
	return c.JSON(http.StatusOK, resp)
}

func insertIntoDB(
			db *sql.DB,
			table_name string,
			field_info[]ApiTypes.FieldInfo,
			records_json interface{}) error {
	// This function constructs an insert statement and runs the statement


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
         return fmt.Errorf("records_json must be a single record (object) or array of records")
    }
    
    if len(records) == 0 {
        return nil
    }
    
    // Build field list and placeholders
    fieldNames := make([]string, 0, len(field_info))
    fieldMap := make(map[string]ApiTypes.FieldInfo)
    
    for _, fi := range field_info {
        fieldNames = append(fieldNames, fi.FieldName)
        fieldMap[fi.FieldName] = fi
    }
    
    // Create INSERT statement
    columns := make([]string, len(fieldNames))
    placeholders := make([]string, len(fieldNames))
    
    for i, fieldName := range fieldNames {
        columns[i] = fieldName
        placeholders[i] = "?"
    }
    
    insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
        table_name,
        strings.Join(columns, ", "),
        strings.Join(placeholders, ", "))
    
    // Insert each record
    for _, record := range records {
        // Validate required fields
        for fieldName, fi := range fieldMap {
            if fi.Required {
                if _, exists := record[fieldName]; !exists {
                    return fmt.Errorf("required field '%s' is missing", fieldName)
                }
            }
        }
        
        // Build values slice in the same order as fieldNames
        values := make([]interface{}, len(fieldNames))
        for i, fieldName := range fieldNames {
            values[i] = record[fieldName]
        }
        
        _, err := db.Exec(insertSQL, values...)
        if err != nil {
            return fmt.Errorf("failed to insert record: %w", err)
        }
    }
    
    return nil
}

func runQuery(db *sql.DB, query string, selected_field_names []string, field_data_types []string) ([]byte, error) {
    rows, err := db.Query(query)
    if err != nil {
        log.Printf("error:%v", err)
        return nil, err
    }
    defer rows.Close()

    var results []map[string]interface{}

    for rows.Next() {
        // Create a slice of interface{} to hold the values
        values := make([]interface{}, len(selected_field_names))
        valuePtrs := make([]interface{}, len(selected_field_names))
        
        for i := range values {
            valuePtrs[i] = &values[i]
        }

        // Scan the row into the value pointers
        if err := rows.Scan(valuePtrs...); err != nil {
            log.Printf("scan error: %v", err)
            return nil, err
        }

        // Create a map for this row
        rowMap := make(map[string]interface{})
        
        for i, fieldName := range selected_field_names {
            value := values[i]
            
            // Convert the value based on its data type
            convertedValue := convertValueByType(value, field_data_types[i])
            rowMap[fieldName] = convertedValue
			log.Printf("Read record, field_name:%s, field_value:%s", fieldName, value)
        }
        
        results = append(results, rowMap)
    }

    if err = rows.Err(); err != nil {
        log.Printf("rows error: %v", err)
        return nil, err
    }

    // Convert the results to JSON
    jsonData, err := json.Marshal(results)
    if err != nil {
        log.Printf("JSON marshal error: %v", err)
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
        return fmt.Sprintf("%v", value)
        
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
			field_name string,
			loc string) (string, error) {
    if value_obj, ok := objMap[field_name]; ok {
		if value_str, ok := value_obj.(string); ok {
			return value_str, nil
		}

		error_msg := fmt.Sprintf("value is not a string, field_name:%s, resource_name:%s (%s)", 
			field_name, resource_name, loc)
		log.Printf("***** Alarm:%s", error_msg)
		err :=fmt.Errorf("%s", error_msg)
		return "", err
	}

	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (%s)", 
			field_name, resource_name, loc)
	log.Printf("***** Alarm:%s", error_msg)
	return "", fmt.Errorf("%s", error_msg)
}

func GetFieldStrArrayValue(objMap map[string]interface{}, 
			resource_name string,
			field_name string,
			loc string) ([]string, error) {
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

	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (%s)", 
			field_name, resource_name, loc)
	log.Printf("***** Alarm:%s", error_msg)
	return nil, fmt.Errorf("%s", error_msg)
}

func GetTableName(objMap map[string]interface{}, 
		resource_name string) (string, string, error) {
	// It retrieves: db_name, table_name, selected_fields from objMap
	db_name, err := GetFieldStrValue(objMap, resource_name, "db_name", "SHD_RHD_224")
	if err != nil {
		return "", "", err
	}

	table_name, err := GetFieldStrValue(objMap, resource_name, "table_name", "SHD_RHD_229")
	if err != nil {
		return "", "", err
	}

	log.Printf("Table:%s.%s (SHD_RHD_667)", db_name, table_name)
	return db_name, table_name, nil
}

func GetSelectedFields(objMap map[string]interface{}, 
		resource_name string) ([]string, []string, error) {
	// It retrieves: db_name, table_name, selected_fields from objMap
	selected_field_names, err := GetFieldStrArrayValue(objMap, resource_name, "selected_field_names", "SHD_RHD_515")
	if err != nil {
		return nil, nil, err
	}

	field_data_types, err := GetFieldStrArrayValue(objMap, resource_name, "field_data_types", "SHD_RHD_520")
	if err != nil {
		return nil, nil, err
	}

	log.Printf("selected fields:%s, types:%s (SHD_RHD_684)", selected_field_names, field_data_types)
	return selected_field_names, field_data_types, nil
}

func ConstructWhereClause(conditions QueryCondition) (string, error) {
    sql, err := buildCondition(conditions)
    if err != nil {
		log.Printf("***** Alarm: failed constructing query (SHD_RHD_281), err:%v", err)
        return "", err
    }
	log.Printf("Where clause constructed:%s", sql)
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