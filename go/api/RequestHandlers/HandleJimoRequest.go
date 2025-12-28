/*
*********************************************************
File: HandleJimoRequest.go
Description: this file implements the logic to handle HTTP
requests.

Conditions
----------
It supports simple and complex conditions. Below are examples of how
to construct the conditions.

	simpleCondition := ApiTypes.Condition{
	    Type:      ApiTypes.ConditionTypeAtomic,
	    FieldName: "status",
	    Opr:       "equal",
	    Value:     "active",
	    DataType:  "string",
	}

// Example 2: AND condition

	andCondition := ApiTypes.Condition{
	    Type: ApiTypes.ConditionTypeAnd,
	    Conditions: []ApiTypes.Condition{
	        {
	            Type:      ApiTypes.ConditionTypeAtomic,
	            FieldName: "age",
	            Opr:       "greater_than",
	            Value:     18,
	            DataType:  "number",
	        },
	        {
	            Type:      ApiTypes.ConditionTypeAtomic,
	            FieldName: "status",
	            Opr:       "equal",
	            Value:     "active",
	            DataType:  "string",
	        },
	    },
	}

// Example 3: Complex nested condition: (age > 18 AND status = 'active') OR role = 'admin'

	complexCondition := ApiTypes.Condition{
	    Type: ApiTypes.ConditionTypeOr,
	    Conditions: []ApiTypes.Condition{
	        {
	            Type: ApiTypes.ConditionTypeAnd,
	            Conditions: []ApiTypes.Condition{
	                {
	                    Type:      ApiTypes.ConditionTypeAtomic,
	                    FieldName: "age",
	                    Opr:       "greater_than",
	                    Value:     18,
	                    DataType:  "number",
	                },
	                {
	                    Type:      ApiTypes.ConditionTypeAtomic,
	                    FieldName: "status",
	                    Opr:       "equal",
	                    Value:     "active",
	                    DataType:  "string",
	                },
	            },
	        },
	        {
	            Type:      ApiTypes.ConditionTypeAtomic,
	            FieldName: "role",
	            Opr:       "equal",
	            Value:     "admin",
	            DataType:  "string",
	        },
	    },
	}

**********************************************************
*/
package RequestHandlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	sq "github.com/Masterminds/squirrel"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/sysdatastores"
	"github.com/labstack/echo/v4"
)

type Operator string
type Field string
type LogicalOperator string

const (
	GreaterThan  Operator = ">"
	GreaterEqual Operator = ">="
	Equal        Operator = "="
	LessEqual    Operator = "<="
	LessThan     Operator = "<"
	NotEqual     Operator = "<>"
	Contain      Operator = "contain"
	Prefix       Operator = "prefix"
)

func HandleJimoRequestEcho(c echo.Context) error {
	rc := NewFromEcho(c)
	body, _ := io.ReadAll(c.Request().Body)
	reqID := rc.ReqID()

	status_code, resp := handleJimoRequestPriv(rc, reqID, body)
	defer c.Request().Body.Close()
	c.JSON(status_code, resp)
	return nil
}

func handleJimoRequestPriv(
	rc RequestContext,
	reqID string,
	body []byte) (int, ApiTypes.JimoResponse) {
	user_info, err := rc.IsAuthenticated(reqID, "SHD_RHD_126")
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_065"})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_067)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_071",
		}
		return ApiTypes.CustomHttpStatus_NotLoggedIn, resp
	}

	log.Printf("[req:%s] HandleJimoRequest, email:%s (SHD_RHD_054)", reqID, user_info.Email)

	// Step 2: Parse minimal info to get request_type
	var genericReq ApiTypes.JimoRequest
	if err := json.Unmarshal(body, &genericReq); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_117"})

		log.Printf("[req:%s] %s (SHD_RHD_117)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_117",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Step 3: Decode the full request based on request_type
	var user_name = user_info.UserName
	switch genericReq.RequestType {
	case ApiTypes.ReqAction_Insert:
		return HandleDBInsert(rc, reqID, body, user_name)

	case ApiTypes.ReqAction_Query:
		return HandleDBQuery(rc, reqID, body, user_name)

	case ApiTypes.ReqAction_Update:
		return HandleDBUpdate(rc, reqID, body, user_name)

	case ApiTypes.ReqAction_Delete:
		return HandleDBDelete(rc, reqID, body, user_name)

	default:
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("unrecognized request_type:%s, log_id:%d",
			genericReq.RequestType, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_166"})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_168)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_166",
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}
}

func HandleDBQuery(rc RequestContext,
	reqID string,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	// It is a database query:
	//	- Get db name and table name
	//	- Check access controls
	//	- Get selected field names
	//	- Get the conditions
	//	- Construct the query statement
	//	- Run the query statement
	/*
			resource_name := req.ResourceName
			resource_def, err := stores.GetResourceDef(resource_name, "query")
			if err != nil {
				error_msg := fmt.Sprintf("resource not found, error:%v, resource_name:%s, loc:%s",
						err, resource_name, req.Loc)
				log.Printf("[req:%s] +++++ WARNING:%s (SHD_RHD_165)", reqID, error_msg)

				resp := ApiTypes.JimoResponse {
					Status: false,
					ReqID:	  reqID,
					ErrorMsg: error_msg,
					Loc: "SHD_RHD_134",
				}
				c.JSON(ApiTypes.CustomHttpStatus_BadRequest, resp)
				return fmt.Errorf("%s", error_msg)
			}

			if resource_def.ResourceDef.ResourceType != ApiTypes.ResourceType_Table {
				error_msg := fmt.Sprintf("incorrect resource type, expecting:%s, actual:%s",
						ApiTypes.ResourceType_Table, resource_def.ResourceDef.ResourceType)
				log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_193)", reqID, error_msg)

				resp := ApiTypes.JimoResponse {
					Status: false,
					ReqID:	  reqID,
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
				log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_208)", reqID, error_msg)

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
					ReqID:	  reqID,
					ErrorMsg: error_msg,
					Loc: "SHD_RHD_222",
				}
				c.JSON(http.StatusBadRequest, resp)
				return fmt.Errorf("%s", error_msg)
			}

			db_name := resource_def.ResourceDef.DBName
			table_name := resource_def.ResourceDef.TableName
	*/
	var req ApiTypes.QueryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_302"})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_304)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  error_msg,
			Loc:       "SHD_RHD_304",
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	query, args, selected_fields, aliases, field_def_map, err := buildQuery(reqID, req)
	table_name := req.TableName
	if err != nil {
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  err.Error(),
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       "SHD_RHD_324",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.DatabaseInfo.MySQLDBHandle

	case ApiTypes.PgName:
		db = ApiTypes.DatabaseInfo.PGDBHandle

	default:
		error_msg := fmt.Sprintf("invalid db type:%s:%s:%s, table_name:%s, loc:%s (SHD_RHD_447)",
			db_type, ApiTypes.MysqlName, ApiTypes.PgName, table_name, req.Loc)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_260)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			ErrorMsg:  error_msg,
			TableName: req.TableName,
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       "SHD_RHD_376",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	var orderby_defs = req.OrderbyDef
	if len(orderby_defs) > 0 {
		var orderby_str = ""
		for i, orderby_def := range orderby_defs {
			var direction = "DESC"
			if orderby_def.IsAsc {
				direction = "ASCE"
			}
			var bb = fmt.Sprintf("ORDER BY %s %s", orderby_def.FieldName, direction)
			if i == 0 {
				orderby_str = bb
			} else {
				orderby_str += ", " + bb
			}
		}

		query += " " + orderby_str
	}

	if req.PageSize <= 0 || req.Start < 0 {
		var error_msg = fmt.Sprintf("invalid limit clause (SHD_RHD_382), page_size:%d, start:%d",
			req.PageSize, req.Start)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  error_msg,
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       "SHD_RHD_389",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", req.PageSize, req.Start)
	// log.Printf("[req:%s] To run query:%s, args:%v, table:%s, loc:%s (SHD_RHD_366)",
	// 	reqID, query, args, table_name, req.Loc)

	json_data, num_records, err := RunQuery(rc, reqID, req, db, query,
		args, selected_fields, aliases, field_def_map)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("run query failed, err:%v, logid:%d, table:%s, loc:%s",
			err, log_id, table_name, req.Loc)
		error_msg1 := fmt.Sprintf("run query failed, err:%v, query:%s, "+
			"table_name:%s, loc:%s", err, query, req.TableName, req.Loc)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_297)", reqID, error_msg1)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Query,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg1,
			CallerLoc:    "SHD_RHD_307"})

		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  error_msg,
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       "SHD_RHD_313",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ErrorMsg:   "",
		ResultType: "json_array",
		NumRecords: num_records,
		TableName:  req.TableName,
		Results:    json_data,
		Loc:        "SHD_RHD_399",
	}

	msg := fmt.Sprintf("query success, query:%s, num_records:%d, table:%s, loc:%s",
		query, num_records, req.TableName, req.Loc)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Query,
		ActivityType: ApiTypes.ActivityType_RequestSuccess,
		AppName:      ApiTypes.AppName_RequestHandler,
		ModuleName:   ApiTypes.ModuleName_RequestHandler,
		ActivityMsg:  &msg,
		CallerLoc:    "SHD_RHD_333"})

	return http.StatusOK, resp
}

// buildJoinClauses handles the join clause. A query with joins are
//
//		SELECT <selected_field_list>
//		  FROM <from_table_name> <join_type> <joined_table_name>
//	        ON <on-clause> [AND additional join clause]
//
//	type OnClauseDef struct {
//	   SourceFieldName string      `json:"source_field_name"`
//	   JoinedFieldName string      `json:"joined_field_name"`
//	   JoinOpr         string      `json:"joined_opr"`
//	   DataType        string      `json:"data_type"`
//
// JoinDef:
//
//	type JoinDef struct {
//	   FromTableName   	string      	`json:"from_table_name"`
//	   JoinedTableName 	string      	`json:"joined_table_name"`
//	   OnClause 		[]OnClauseDef 	`json:"on_clause"`
//	   SelectedFields  	[]string      	`json:"selected_fields"`
//	   EmbedName       	string      	`json:"embed_name"`
//
// 'SelectedFields' is an array of strings of the format:
//
//	<tablename>.<fieldname>[:<alias>]
//
// If EmbedName is not empty, all the selected field names
// are prepended with "<EmbedName>____" (four '_'!!!) For instance, if
// EmbedName = "question" and SelectedFields is ["field1", "field2"],
// the final selected field names are "question____field1, question____field2"
//
// The function returns the following:
//   - the join clause
//   - join types
//   - the selected field names
//   - aliases
func buildJoinClauses(
	join_def []ApiTypes.JoinDef,
	field_def_map map[string][]ApiTypes.FieldDef) ([]string, []string, []string, []string) {
	if len(join_def) == 0 {
		return []string{}, []string{}, []string{}, []string{}
	}

	var joinClauses []string
	var selectFields []string
	var joinTypes []string
	var aliases []string

	for _, jd := range join_def {
		// Build ON clause
		if len(jd.OnClause) == 0 {
			continue // Skip if no ON conditions provided
		}

		if len(jd.FromFieldDefs) > 0 {
			if _, ok := field_def_map[jd.FromTableName]; !ok {
				field_def_map[jd.FromTableName] = jd.FromFieldDefs
			}
		}

		if len(jd.JoinedFieldDefs) > 0 {
			if _, ok := field_def_map[jd.JoinedTableName]; !ok {
				field_def_map[jd.JoinedTableName] = jd.JoinedFieldDefs
			}
		}

		var onConditions []string
		for _, on := range jd.OnClause {
			// Default to '=' if no operator specified
			joinOpr := on.JoinOpr
			if joinOpr == "" {
				joinOpr = "="
			}

			// IMPORTANT: field names in Join On-Clause are not
			// qualified names!
			onCondition := fmt.Sprintf("%s.%s %s %s.%s",
				jd.FromTableName, on.SourceFieldName,
				joinOpr,
				jd.JoinedTableName, on.JoinedFieldName)
			onConditions = append(onConditions, onCondition)
		}

		onClauseStr := strings.Join(onConditions, " AND ")

		// Build JOIN clause (without join type - that's stored separately)
		joinClause := fmt.Sprintf("%s ON %s",
			jd.JoinedTableName,
			onClauseStr)
		joinClauses = append(joinClauses, joinClause)
		joinTypes = append(joinTypes, jd.JoinType)

		// Add selected fields with embed prefix if specified
		// Note that selected fields are qualified fields to avoid
		// duplicate field names in a join select statement.
		// If 'jd.EmbedName' is defined, the selected field name is
		// prepended with "jd.EmbedName" + "____". During scanning,
		// it should put these fields into a sub-doc named jd.EmbedName.
		new_selected, new_aliases := getAliases(jd.SelectedFields)
		if jd.EmbedName != "" {
			for i, field := range new_selected {
				new_aliase := fmt.Sprintf("%s____%s", jd.EmbedName, new_aliases[i])
				selectFields = append(selectFields, field)
				aliases = append(aliases, new_aliase)
			}
		} else {
			selectFields = append(selectFields, new_selected...)
			aliases = append(aliases, new_aliases...)
		}
	}

	// Return the JOIN clauses and the additional selected fields
	return joinClauses, joinTypes, selectFields, aliases
}

// HandleDBInsert retrieves the request from the context.
// the request should have the resource name and resource opr.
// If it does have these, it will use these attributes to retrieve
// the resource definition. Otherwise, it checks whether the Table Manager
// If the table does not exist and dynamic table is allowed, it will
// create the table dynamically as a generic table.
func HandleDBInsert(
	rc RequestContext,
	reqID string,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	// This function handles the 'insert' request.
	// The data to be inserted is in req.records
	var req ApiTypes.InsertRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_142"})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_142)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_142",
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	db_name := req.DBName
	table_name := req.TableName
	field_defs := req.FieldDefs
	log.Printf("[req:%s] handleDBInsert, table:%s:%s", reqID, db_name, table_name)
	log.Printf("[req:%s] FieldDefs:%d", reqID, len(field_defs))
	/*
		if field_defs == nil {
			resource_def, err 	:= stores.GetResouroceDef(resource_name, resource_opr)
			if err != nil {
				error_msg := fmt.Sprintf("%v", err)
				log.Printf("[req:%s] +++++ WARNING:%s (SHD_RHD_329)", reqID, error_msg)

				resp := ApiTypes.JimoResponse {
					Status: false,
					ReqID:	  reqID,
					ErrorMsg: error_msg,
					Loc: "SHD_RHD_334",
				}
				c.JSON(ApiTypes.CustomHttpStatus_ResourceNotFound, resp)
				return fmt.Errorf("%s", error_msg)
			}

			if ff, exist := resource_def.
			field_defs = resource_def.ResourceDef.ResourceJSON
		}

		// The resource type must be 'table'
		if resource_def.ResourceDef.ResourceType != ApiTypes.ResourceType_Table {
			error_msg := fmt.Sprintf("incorrect resource type, expecting:%s, actual:%s",
					ApiTypes.ResourceType_Table, resource_def.ResourceDef.ResourceType)
			log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_343)", reqID, error_msg)

			resp := ApiTypes.JimoResponse {
				Status: false,
				ReqID:	  reqID,
				ErrorMsg: error_msg,
				Loc: "SHD_RHD_348",
			}
			c.JSON(ApiTypes.CustomHttpStatus_BadRequest, resp)
			return fmt.Errorf("%s", error_msg)
		}

		db_name := resource_def.ResourceDef.DBName
		table_name := resource_def.ResourceDef.TableName
	*/

	if table_name == "" {
		error_msg := "failed get table name."
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_400)", reqID, error_msg)

		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_405",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	records := req.Records
	if len(records) <= 0 {
		error_msg := "missing records to insert."
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_581)", reqID, error_msg)

		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_581",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB
	var err error
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.DatabaseInfo.MySQLDBHandle
		err = InsertBatch(user_name, db, table_name, req, field_defs, records, 30, db_type)

	case ApiTypes.PgName:
		db = ApiTypes.DatabaseInfo.PGDBHandle
		err = InsertBatch(user_name, db, table_name, req, field_defs, records, 30, db_type)

	default:
		error_msg := fmt.Sprintf("invalid db type:%s", db_type)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_465)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_469",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	if err != nil {
		error_msg := fmt.Sprintf("failed insert to db:%v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_477)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_481",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ErrorMsg:   "",
		ResultType: "none",
		Loc:        "SHD_RHD_638",
	}
	return http.StatusOK, resp
}

// HandleDBUpdate updates records.
// 'req' attributes include:
//
//		DBName                  string      `json:"db_name"`
//	 TableName               string      `json:"table_name"`
//	 Conditions              string      `json:"conditions"`
//	 Records                 string      `json:"records,omitempty"`
//	 FieldDefs               string  	`json:"field_defs"`
func HandleDBUpdate(
	rc RequestContext,
	reqID string,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	var req ApiTypes.UpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_639"})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_641)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_641",
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	db_name := req.DBName
	table_name := req.TableName
	field_defs := req.FieldDefs

	db_type := ApiTypes.GetDBType()
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner

	default:
		error_msg := fmt.Sprintf("[req=%s] ***** unrecognized database type (SHD_RHD_765): %s", reqID, db_type)
		log.Printf("%s", error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_771",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] handleDBInsert:%s:%s", db_name, reqID, table_name)
	log.Printf("[req:%s] FieldDefs:%d", reqID, len(field_defs))

	if table_name == "" {
		error_msg := "failed get table name"
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_400)", reqID, error_msg)

		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_405",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] Table:%s.%s (SHD_RHD_411)", reqID, db_name, table_name)

	update_record := req.Record
	if len(update_record) <= 0 {
		error_msg := "no records provided for update"
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_772)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_772",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	field_map := make(map[string]bool)
	for _, fd := range field_defs {
		field_map[fd.FieldName] = true
	}

	cond_def := req.Condition
	expr, err := buildConditionExpr(table_name, cond_def, field_map)
	if err != nil {
		error_msg := fmt.Sprintf("failed building conditions, err:%v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_760)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_760",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	if expr == nil {
		error_msg := fmt.Sprintf("missing conditions, loc:%s", req.Loc)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_717)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_717",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Build the UPDATE query using Squirrel
	query := sq.Update(table_name).PlaceholderFormat(sq.Dollar)

	// Add SET clauses for each field in the update data
	for field, value := range update_record {
		// Validate field name (security critical!)
		if !isValidFieldName(field) {
			error_msg := fmt.Sprintf("invalid field name (SHD_RHD_757): %s", field)
			log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_774)", reqID, error_msg)
			resp := ApiTypes.JimoResponse{
				Status:   false,
				ReqID:    reqID,
				ErrorMsg: error_msg,
				Loc:      "SHD_RHD_774",
			}
			return ApiTypes.CustomHttpStatus_BadRequest, resp
		}

		query = query.Set(field, value)
	}

	// Add WHERE clause
	query = query.Where(expr)

	// Generate the SQL and arguments
	sql, args, err := query.ToSql()
	if err != nil {
		error_msg := fmt.Sprintf("failed to build SQL query: %v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_793)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_793",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Execute the update query
	// Assuming you have a database connection variable called 'db'
	// Replace 'db' with your actual database connection variable
	result, err := db.Exec(sql, args...)
	if err != nil {
		error_msg := fmt.Sprintf("failed to execute update query: %v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_808)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_808",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Get the number of affected rows
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Sprintf("failed to get rows affected: %v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_821)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_821",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Success response
	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ResultType: "json",
		NumRecords: 1,
		Results: map[string]interface{}{
			"rows_affected": rowsAffected,
			"sql":           sql, // Include SQL for debugging (remove in production)
		},
		Loc: "SHD_RHD_837",
	}

	return ApiTypes.CustomHttpStatus_Success, resp
}

// HandleDBDelete delete records.
// 'req' attributes include:
//
//		DBName                  string      `json:"db_name"`
//	 TableName               string      `json:"table_name"`
//	 Conditions              string      `json:"conditions"`
//	 Records                 string      `json:"records,omitempty"`
//	 FieldDefs               string  	`json:"field_defs"`
func HandleDBDelete(
	rc RequestContext,
	reqID string,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	var req ApiTypes.DeleteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    "SHD_RHD_639"})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_641)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_641",
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	db_name := req.DBName
	table_name := req.TableName
	field_defs := req.FieldDefs

	db_type := ApiTypes.GetDBType()
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner

	default:
		error_msg := fmt.Sprintf("[req=%s] ***** unrecognized database type (SHD_RHD_973): %s", reqID, db_type)
		log.Printf("%s", error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_979",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] handleDBDelete:%s:%s", reqID, db_name, table_name)
	log.Printf("[req:%s] FieldDefs:%d", reqID, len(field_defs))

	if table_name == "" {
		error_msg := "failed get table name. Resource name"
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_847)", reqID, error_msg)

		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_880",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] Delete records, table:%s.%s (SHD_RHD_885)", reqID, db_name, table_name)

	field_map := make(map[string]bool)
	for _, fd := range field_defs {
		field_map[fd.FieldName] = true
	}

	cond_def := req.Condition
	expr, err := buildConditionExpr(table_name, cond_def, field_map)
	if err != nil {
		error_msg := fmt.Sprintf("failed building conditions, err:%v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_686)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_868",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	if expr == nil {
		error_msg := fmt.Sprintf("missing conditions, loc:%s", req.Loc)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_879)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_879",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Build the UPDATE query using Squirrel
	query := sq.Delete(table_name).PlaceholderFormat(sq.Dollar)

	// Add WHERE clause
	query = query.Where(expr)

	// Generate the SQL and arguments
	sql, args, err := query.ToSql()
	if err != nil {
		error_msg := fmt.Sprintf("failed to build SQL query: %v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_887)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_887",
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Execute the update query
	// Assuming you have a database connection variable called 'db'
	// Replace 'db' with your actual database connection variable
	result, err := db.Exec(sql, args...)
	if err != nil {
		error_msg := fmt.Sprintf("failed to execute update query: %v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_902)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_902",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Get the number of affected rows
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Sprintf("failed to get rows affected: %v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_915)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      "SHD_RHD_915",
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Success response
	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ResultType: "json",
		NumRecords: 1,
		Results: map[string]interface{}{
			"rows_affected": rowsAffected,
			"sql":           sql, // Include SQL for debugging (remove in production)
		},
		Loc: "SHD_RHD_931",
	}

	return ApiTypes.CustomHttpStatus_Success, resp
}

// Condition represents a single condition in the WHERE clause
type Condition struct {
	FieldName string
	Opr       string
	Value     string
}

// LogicOperator represents the logical operator between conditions
type LogicOperator string

const (
	LogicAND LogicOperator = "AND"
	LogicOR  LogicOperator = "OR"
)

// RunQuery executes the given query and returns the results as JSON string
func RunQuery(
	rc RequestContext,
	reqID string,
	req ApiTypes.QueryRequest,
	db *sql.DB,
	query string,
	args []interface{},
	selected_fields []string,
	aliases []string,
	field_def_map map[string][]ApiTypes.FieldDef) ([]map[string]interface{}, int, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("[req:%s] ***** Alarm:%v (SHD_RHD_493)", reqID, err)
		return nil, 0, err
	}
	defer rows.Close()

	var data_types = make(map[string]string)
	log.Printf("[req:%s] RunQuery:%s, args:%v, table:%s (SHD_RHD_490)", reqID, query, args, req.TableName)
	for table_name, field_defs := range field_def_map {
		for i := range field_defs {
			full_name := fmt.Sprintf("%s.%s", table_name, field_defs[i].FieldName)
			data_types[full_name] = field_defs[i].DataType
		}
	}

	var results []map[string]interface{}

	var count int = 0
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(selected_fields))
		valuePtrs := make([]interface{}, len(selected_fields))
		count++

		for i := range values {
			valuePtrs[i] = &values[i]
		}

		log.Printf("[req:%s] Handle record:%d, args:%v (SHD_RHD_058)", reqID, count, args)
		// Scan the row into the value pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Printf("[req:%s] ***** Alarm:scan error: %v (SHD_RHD_511)", reqID, err)
			return nil, 0, fmt.Errorf("scan error:%v (SHD_RHD_511)", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		objMap := make(map[string]map[string]interface{})

		for i, field_name := range selected_fields {
			value := values[i]
			field_aliase := aliases[i]

			// Convert the value based on its data type
			// 'data_types' is a map of full field names!!!
			// rowMap is a map of alises!!!
			if data_type, exists := data_types[field_name]; exists {
				convertedValue := convertValueByType(value, data_type)

				// Process <embed_name>____<alias_name>
				embed_index := strings.LastIndex(field_aliase, "____")
				if embed_index != -1 {
					fieldParts := strings.Split(field_aliase, "____")
					if len(fieldParts) == 2 {
						sub_obj, exist := objMap[fieldParts[0]]
						if !exist {
							sub_obj = make(map[string]interface{})
							objMap[fieldParts[0]] = sub_obj
						}
						sub_obj[fieldParts[1]] = convertedValue
					} else {
						rowMap[field_aliase] = convertedValue
					}
				} else {
					rowMap[field_aliase] = convertedValue
				}
			} else {
				error_msg := fmt.Sprintf("field not found (SHD_RHD_955):%s, selected:%v, data_types:%v",
					field_name, selected_fields, data_types)
				log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
				return nil, 0, fmt.Errorf("%s", error_msg)
			}
		}

		for embed_name, subobj := range objMap {
			rowMap[embed_name] = subobj
		}

		results = append(results, rowMap)
	}

	log.Printf("[req:%s] Query success, records:%d (SHD_RHD_970)", reqID, count)

	if err = rows.Err(); err != nil {
		error_msg := fmt.Sprintf("rows error: %v (SHD_RHD_530)", err)
		log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
		return nil, 0, fmt.Errorf("%s", error_msg)
	}

	return results, count, nil
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

func GetFieldStrValue(
	rc RequestContext,
	reqID string,
	objMap map[string]interface{},
	resource_name string,
	field_name string) (string, error) {
	if value_obj, ok := objMap[field_name]; ok {
		if value_str, ok := value_obj.(string); ok {
			return value_str, nil
		}

		error_msg := fmt.Sprintf("value is not a string, field_name:%s, resource_name:%s (SHD_RHD_633)",
			field_name, resource_name)
		log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
		err := fmt.Errorf("%s", error_msg)
		return "", err
	}

	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (SHD_RHD_640)",
		field_name, resource_name)
	log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
	return "", fmt.Errorf("%s", error_msg)
}

func GetFieldStrArrayValue(
	rc RequestContext,
	reqID string,
	objMap map[string]interface{},
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
	log.Printf("[req:%s] +++++ Warn:%s", reqID, error_msg)
	return nil, nil
}

func GetFieldAnyArrayValue(
	rc RequestContext,
	reqID string,
	objMap map[string]interface{},
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
	log.Printf("[req:%s] +++++ Warn:%s", reqID, error_msg)
	return nil, fmt.Errorf("%s", error_msg)
}

func GetTableName(
	rc RequestContext,
	reqID string,
	resource_def map[string]interface{},
	resource_name string) (string, string, error) {
	// It retrieves: db_name and table_name. db_name is optional.
	db_name, _ := GetFieldStrValue(rc, reqID, resource_def, resource_name, "db_name")

	table_name, err := GetFieldStrValue(rc, reqID, resource_def, resource_name, "table_name")
	if err != nil {
		return "", "", err
	}

	log.Printf("[req:%s] Table:%s.%s (SHD_RHD_667)", reqID, db_name, table_name)
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

// buildConditionExpr builds conditions defined by 'condition'.
// Field names in the condition are all local field names.
// 'field_map' is also local field name map!
func buildConditionExpr(
	table_name string,
	condition ApiTypes.CondDef,
	field_map map[string]bool) (sq.Sqlizer, error) {
	switch condition.Type {
	case ApiTypes.ConditionTypeNull:
		return nil, nil

	case ApiTypes.ConditionTypeAtomic:
		// Build atomic condition
		field := condition.FieldName
		dataType := condition.DataType
		rawValue := condition.Value

		// Validate field name (security critical!)
		if !field_map[field] {
			return nil, fmt.Errorf("invalid field name: %s, field_map:%v in table:%s",
				field, field_map, table_name)
		}

		// Use rawValue directly for parameterized queries - Squirrel handles type conversion
		var expr sq.Sqlizer
		switch Operator(condition.Opr) {
		case Equal:
			expr = sq.Eq{field: rawValue}
		case GreaterThan:
			expr = sq.Gt{field: rawValue}
		case GreaterEqual:
			expr = sq.GtOrEq{field: rawValue}
		case LessThan:
			expr = sq.Lt{field: rawValue}
		case LessEqual:
			expr = sq.LtOrEq{field: rawValue}
		case NotEqual:
			expr = sq.NotEq{field: rawValue}
		case Contain:
			if dataType == "string" {
				strVal, ok := rawValue.(string)
				if !ok {
					return nil, fmt.Errorf("CONTAIN operator requires string value, got %T, table_name:%s", rawValue, table_name)
				}
				expr = sq.Like{field: "%" + strVal + "%"}
			} else {
				return nil, fmt.Errorf("CONTAIN operator only supported for string type, got %s, table_name:%s", dataType, table_name)
			}
		case Prefix:
			if dataType == "string" {
				strVal, ok := rawValue.(string)
				if !ok {
					return nil, fmt.Errorf("PREFIX operator requires string value, got %T, table_name:%s", rawValue, table_name)
				}
				expr = sq.Like{field: strVal + "%"}
			} else {
				return nil, fmt.Errorf("PREFIX operator only supported for string type, got %s, table_name:%s", dataType, table_name)
			}
		default:
			return nil, fmt.Errorf("unsupported operator (SHD_RHD_319): %s, table_name:%s", condition.Opr, table_name)
		}
		return expr, nil

	case ApiTypes.ConditionTypeAnd:
		// Build AND of multiple conditions
		if len(condition.Conditions) == 0 {
			return nil, fmt.Errorf("AND condition must have at least one sub-condition, table_name:%s", table_name)
		}

		var subExprs []sq.Sqlizer
		for _, subCond := range condition.Conditions {
			expr, err := buildConditionExpr(table_name, subCond, field_map)
			if err != nil {
				return nil, err
			}

			if expr != nil {
				subExprs = append(subExprs, expr)
			}
		}
		return sq.And(subExprs), nil

	case ApiTypes.ConditionTypeOr:
		// Build OR of multiple conditions
		if len(condition.Conditions) == 0 {
			return nil, fmt.Errorf("OR condition must have at least one sub-condition, table_name:%s", table_name)
		}

		var subExprs []sq.Sqlizer
		for _, subCond := range condition.Conditions {
			expr, err := buildConditionExpr(table_name, subCond, field_map)
			if err != nil {
				return nil, err
			}

			if expr != nil {
				subExprs = append(subExprs, expr)
			}
		}
		return sq.Or(subExprs), nil

	default:
		return nil, fmt.Errorf("unknown condition type: %s, table_name:%s", condition.Type, table_name)
	}
}

// buildQuery builds a query. It returns:
//   - Query (the statement)
//   - args
//   - selectged fields
//   - aliases
//   - field_def map
//   - error
//
// 1. Simple queries automatically
// 2. Complex queries manual:
//   - Directly write in code
//   - Write it and save it in database, use it when you need, parameterized
func buildQuery(
	reqID string,
	req ApiTypes.QueryRequest) (string, []interface{}, []string, []string, map[string][]ApiTypes.FieldDef, error) {
	db_name := req.DBName
	table_name := req.TableName
	if db_name == "" || table_name == "" {
		error_msg := "missing db/table name (SHD_RHD_219)"
		log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
		return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
	}

	fieldDefMap := make(map[string][]ApiTypes.FieldDef)

	// Field Defs use local field names. Selected field names
	// use qualified (i.e., tablename.fieldname) names.
	// Selected names are in the form:
	//		<tablename>.<fieldname>[:<alias>]
	// <alias> is optional. If not specified, it defaults to <fieldname>.
	field_defs := req.FieldDefs
	fieldDefMap[table_name] = field_defs
	selected_fields := req.FieldNames
	if len(selected_fields) == 0 {
		error_msg := fmt.Sprintf("missing selected fields, table name:%s", table_name)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_235)", reqID, error_msg)
		return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
	}

	if len(field_defs) == 0 {
		error_msg := fmt.Sprintf("missing field_defs, table name:%s", table_name)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_323)", reqID, error_msg)
		return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
	}

	query_cond := req.Condition
	log.Printf("[req:%s] Table:%s.%s, selected fields:%v, condition:%s, loc:%s",
		reqID, db_name, table_name, selected_fields, query_cond, req.Loc)

	// Note: the field map assumes field names are not full names (i.e.,
	// tablename.fieldname). This is okey for table conditions. Join
	// conditions should be defined in Joins.
	field_map := make(map[string]bool)
	for _, fd := range field_defs {
		// full_name := fmt.Sprintf("%s.%s", table_name, fd.FieldName)
		field_map[fd.FieldName] = true
	}

	expr, err := buildConditionExpr(table_name, query_cond, field_map)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}

	join_defs := req.JoinDefs
	joinClauses, joinTypes, additionalSelectedFields, additional_aliases :=
		buildJoinClauses(join_defs, fieldDefMap)

	// Combine selected fields
	var allSelectedFields []string
	var allAliases []string
	var new_selected_fields, alias_names = getAliases(selected_fields)
	allSelectedFields = append(allSelectedFields, new_selected_fields...)
	allAliases = append(allAliases, alias_names...)

	if len(additionalSelectedFields) > 0 {
		allSelectedFields = append(allSelectedFields, additionalSelectedFields...)
		allAliases = append(allAliases, additional_aliases...)
	}

	// Build the base query
	query := sq.Select(allSelectedFields...).From(table_name).PlaceholderFormat(sq.Dollar)

	// Add JOIN clauses
	// log.Printf("[req=%s] JoinClauses:%v, join_types:%v (SHD_RHD_533)", reqID, joinClauses, joinTypes)
	if len(joinClauses) > 0 {
		for i, join := range joinClauses {
			// if i == 0 {
			//     continue // Skip the first empty part
			// }
			switch joinTypes[i] {
			case ApiTypes.JoinTypeJoin:
				// log.Printf("[req=%s] Join:%v (SHD_RHD_541)", reqID, join)
				query = query.Join(join)

			case ApiTypes.JoinTypeLeftJoin:
				// log.Printf("[req=%s] LeftJoin:%v (SHD_RHD_545)", reqID, join)
				query = query.LeftJoin(join)

			case ApiTypes.JoinTypeRightJoin:
				// log.Printf("[req=%s] RightJoin:%v (SHD_RHD_549)", reqID, join)
				query = query.RightJoin(join)

			case ApiTypes.JoinTypeInnerJoin:
				log.Printf("[req=%s] InnerJoin:%v (SHD_RHD_553)", reqID, join)
				query = query.InnerJoin(join)

			default:
				error_msg := fmt.Sprintf("invalid join type, pos:%d, join clauses:%v, join_types:%v", i, joinClauses, joinTypes)
				log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_538)", reqID, error_msg)
				return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
			}
		}
	}

	// Add WHERE clause
	if expr != nil {
		log.Printf("[req:%s] expr:%v (SHD_RHD_476)", reqID, expr)
		query = query.Where(expr)
	}

	// var start = req.Start
	// var page_size = req.PageSize
	// query.Limit(uint64(page_size)).Offset(uint64(start))

	sql, args, err := query.ToSql()
	if err != nil {
		error_msg := fmt.Sprintf("failed building query:%v", err)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_549)", reqID, error_msg)
		return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
	}
	log.Printf("[req:%s] Generated query:%s, args:%d (SHD_RHD_483)", reqID, sql, len(args))
	return sql, args, allSelectedFields, allAliases, fieldDefMap, nil
}

// Whitelist of allowed field names (adjust based on your schema)
var allowedFields = map[string]bool{
	"name":       true,
	"email":      true,
	"created_at": true,
	// ... add all valid field names here
}

func isValidFieldName(field string) bool {
	return allowedFields[field]
}

func getAliases(selected_field_names []string) ([]string, []string) {
	// field name format:
	//	<tablename>.<fieldname>[:<alias>]
	// If ":<alias>" is not present, it defaults to <fieldname>
	// This function returns two arrays of strings. The first one
	// is all the selected fields without ":<alias>" and the
	// second one is the aliases.
	fields := make([]string, len(selected_field_names))
	aliases := make([]string, len(selected_field_names))

	for i, fieldSpec := range selected_field_names {
		// Check if there's an alias (colon present)
		colonIndex := strings.LastIndex(fieldSpec, ":")

		var baseField string
		if colonIndex != -1 {
			// Alias is present
			baseField = fieldSpec[:colonIndex]
			aliases[i] = fieldSpec[colonIndex+1:]
		} else {
			// No alias
			baseField = fieldSpec
			// Extract fieldname for alias (part after last dot)
			fieldParts := strings.Split(fieldSpec, ".")
			if len(fieldParts) > 0 {
				aliases[i] = fieldParts[len(fieldParts)-1]
			} else {
				aliases[i] = fieldSpec
			}
		}

		fields[i] = baseField
	}

	return fields, aliases
}

/*
// Helper function to parse simple PostgreSQL text arrays like {a,b,c} or {"a","b"}
func parsePostgresTextArray(raw string) ([]string, error) {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		// Not a valid array literal
		error_msg := fmt.Sprintf("not a valid array literal:%v", raw)
		log.Printf("***** Alarm:%s", error_msg)
		return []string{raw}, fmt.Errorf("%s", error_msg)
	}
	inner := raw[1 : len(raw)-1]
	if inner == "" {
		return []string{}, nil
	}

	// Handle quoted elements: PostgreSQL uses " for escaping
	// Simple approach: split on comma not inside quotes
	var elements []string
	var current strings.Builder
	inQuotes := false
	escaped := false // unused here since lib/pq doesn't escape inside

	for _, ch := range inner {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}

		switch ch {
		case '"':
			inQuotes = !inQuotes
		case '\\':
			// In real PG, backslash escapes, but lib/pq unescapes already
			// We'll assume input is clean
			current.WriteRune(ch)
		case ',':
			if !inQuotes {
				elements = append(elements, strings.TrimSpace(current.String()))
				current.Reset()
				continue
			}
			fallthrough
		default:
			current.WriteRune(ch)
		}
	}
	// Add last element
	if current.Len() > 0 || len(elements) == 0 {
		elements = append(elements, strings.TrimSpace(current.String()))
	}

	// Remove surrounding quotes and unescape (basic)
	for i, el := range elements {
		el = strings.TrimSpace(el)
		if len(el) >= 2 && el[0] == '"' && el[len(el)-1] == '"' {
			el = el[1 : len(el)-1]
			// Unescape double quotes ("" -> ")
			el = strings.ReplaceAll(el, `""`, `"`)
		}
		elements[i] = el
	}

	return elements, nil
}
*/
