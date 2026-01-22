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
	"context"
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
	"github.com/chendingplano/shared/go/api/EchoFactory"
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
	rc := EchoFactory.NewFromEcho(c, "SHD_HJR_114")
	ctx := c.Request().Context()
	call_flow := ctx.Value(ApiTypes.CallFlowKey)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	body, _ := io.ReadAll(c.Request().Body)

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_119", call_flow)
	log.Printf("[req=%s] ++++++++++ HandleJimoRequestEcho:%s, data:%s",
		reqID, new_call_flow, string(body))

	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, new_call_flow)

	status_code, resp := handleJimoRequestPriv(new_ctx, rc, body)
	defer c.Request().Body.Close()
	c.JSON(status_code, resp)
	return nil
}

func handleJimoRequestPriv(
	ctx context.Context,
	rc ApiTypes.RequestContext,
	body []byte) (int, ApiTypes.JimoResponse) {
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	user_info, err := rc.IsAuthenticated()
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_135", call_flow))
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("auth failed, err:%v, log_id:%d", err, log_id)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_139", call_flow)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_AuthFailure,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_067)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_NotLoggedIn, resp
	}

	log.Printf("[req:%s] HandleJimoRequest, email:%s (SHD_RHD_054)", reqID, user_info.Email)

	// Step 2: Parse minimal info to get request_type
	var genericReq ApiTypes.JimoRequest
	if err := json.Unmarshal(body, &genericReq); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_166", call_flow)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})

		log.Printf("[req:%s] %s (SHD_RHD_117)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Step 3: Decode the full request based on request_type
	var user_name = user_info.UserName
	switch genericReq.RequestType {
	case ApiTypes.ReqAction_Insert:
		return HandleDBInsert(new_ctx, rc, body, user_name)

	case ApiTypes.ReqAction_Query:
		return HandleDBQuery(new_ctx, rc, body, user_name)

	case ApiTypes.ReqAction_Update:
		return HandleDBUpdate(new_ctx, rc, body, user_name)

	case ApiTypes.ReqAction_Delete:
		return HandleDBDelete(new_ctx, rc, body, user_name)

	default:
		log_id := sysdatastores.NextActivityLogID()
		error_msg := fmt.Sprintf("unrecognized request_type:%s, log_id:%d",
			genericReq.RequestType, log_id)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_205", call_flow)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})
		log.Printf("[req:%s] ***** Alarm:%s (%s->SHD_RHD_168)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}
}

func HandleDBQuery(
	ctx context.Context,
	rc ApiTypes.RequestContext,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_233", call_flow))
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_305", call_flow)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_304)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  error_msg,
			Loc:       new_call_flow,
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	query, args, selected_fields, aliases, field_def_map, err := buildQuery(new_ctx, req)
	table_name := req.TableName
	if err != nil {
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_330", call_flow)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  err.Error(),
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       new_call_flow,
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_355", call_flow)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			ErrorMsg:  error_msg,
			TableName: req.TableName,
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       new_call_flow,
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_389", call_flow)
		resp := ApiTypes.JimoResponse{
			Status:    false,
			ReqID:     reqID,
			TableName: req.TableName,
			ErrorMsg:  error_msg,
			ErrorCode: ApiTypes.CustomHttpStatus_InternalError,
			Loc:       new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", req.PageSize, req.Start)
	// log.Printf("[req:%s] To run query:%s, args:%v, table:%s, loc:%s (SHD_RHD_366)",
	// 	reqID, query, args, table_name, req.Loc)

	json_data, num_records, err := RunQuery(new_ctx, rc, req, db, query,
		args, selected_fields, aliases, field_def_map)
	if err != nil {
		log_id := sysdatastores.NextActivityLogID()
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_410", call_flow)
		error_msg := fmt.Sprintf("run query failed, err:%v, logid:%d, table:%s, loc:%s",
			err, log_id, table_name, req.Loc)
		error_msg1 := fmt.Sprintf("run query failed, err:%v, query:%s, "+
			"table_name:%s, loc:%s", err, query, req.TableName, req.Loc)
		log.Printf("[req:%s] ***** Alarm:%s (%s->SHD_RHD_297)", reqID, error_msg, call_flow)

		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_Query,
			ActivityType: ApiTypes.ActivityType_DatabaseError,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg1,
			CallerLoc:    new_call_flow})

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

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_437", call_flow)
	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ErrorMsg:   "",
		ResultType: "json_array",
		NumRecords: num_records,
		TableName:  req.TableName,
		Results:    json_data,
		Loc:        new_call_flow,
	}

	msg := fmt.Sprintf("query success, query:%s, num_records:%d, table:%s, loc:%s",
		query, num_records, req.TableName, req.Loc)

	sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
		ActivityName: ApiTypes.ActivityName_Query,
		ActivityType: ApiTypes.ActivityType_RequestSuccess,
		AppName:      ApiTypes.AppName_RequestHandler,
		ModuleName:   ApiTypes.ModuleName_RequestHandler,
		ActivityMsg:  &msg,
		CallerLoc:    new_call_flow})

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
	ctx context.Context,
	rc ApiTypes.RequestContext,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_590", call_flow))

	// This function handles the 'insert' request.
	// The data to be inserted is in req.records
	var req ApiTypes.InsertRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_597", call_flow)
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_142)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}

		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	db_name := req.DBName
	table_name := req.TableName
	field_defs := req.FieldDefs
	log.Printf("[req:%s] (SHD_RHD_622) handleDBInsert, table:%s:%s", reqID, db_name, table_name)
	log.Printf("[req:%s] (SHD_RHD_623) FieldDefs:%d", reqID, len(field_defs))
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

		new_call_flow := fmt.Sprintf("%s->SHD_RHD_669", call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	records := req.Records
	if len(records) <= 0 {
		error_msg := "missing records to insert."
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_581)", reqID, error_msg)

		new_call_flow := fmt.Sprintf("%s->SHD_RHD_684", call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	db_type := ApiTypes.DatabaseInfo.DBType
	var db *sql.DB
	var err error
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.DatabaseInfo.MySQLDBHandle
		err = InsertBatch(new_ctx, user_name, db, table_name, req, field_defs, records, 30, db_type)

	case ApiTypes.PgName:
		db = ApiTypes.DatabaseInfo.PGDBHandle
		err = InsertBatch(new_ctx, user_name, db, table_name, req, field_defs, records, 30, db_type)

	default:
		error_msg := fmt.Sprintf("invalid db type:%s", db_type)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_669", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	if err != nil {
		error_msg := fmt.Sprintf("failed insert to db:%v", err)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_721", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_732", call_flow)
	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ErrorMsg:   "",
		ResultType: "none",
		Loc:        new_call_flow,
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
	ctx context.Context,
	rc ApiTypes.RequestContext,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_233", call_flow))

	var req ApiTypes.UpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_763", call_flow)
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_641)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_799", call_flow)
		error_msg := fmt.Sprintf("[req=%s] ***** unrecognized database type (%s): %s", reqID, new_call_flow, db_type)
		log.Printf("%s", error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] (SHD_RHD_811) handleDBInsert:%s:%s", db_name, reqID, table_name)
	log.Printf("[req:%s] (SHD_RHD_812) FieldDefs:%d", reqID, len(field_defs))

	if table_name == "" {
		error_msg := "failed get table name"
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_799", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)

		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_828", call_flow)
	log.Printf("[req:%s] Table:%s.%s (%s)", reqID, db_name, table_name, new_call_flow)

	update_record := req.Record
	if len(update_record) <= 0 {
		error_msg := "no records provided for update"
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_834", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
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
	expr, err := buildConditionExpr(new_ctx, table_name, cond_def, field_map)
	if err != nil {
		error_msg := fmt.Sprintf("failed building conditions, err:%v", err)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_854", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	if expr == nil {
		error_msg := fmt.Sprintf("missing conditions, loc:%s", req.Loc)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_867", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
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
			new_call_flow := fmt.Sprintf("%s->SHD_RHD_886", call_flow)
			log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
			resp := ApiTypes.JimoResponse{
				Status:   false,
				ReqID:    reqID,
				ErrorMsg: error_msg,
				Loc:      new_call_flow,
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_907", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_924", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Get the number of affected rows
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Sprintf("failed to get rows affected: %v", err)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_932", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Success response
	new_call_flow = fmt.Sprintf("%s->SHD_RHD_951", call_flow)
	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ResultType: "json",
		NumRecords: 1,
		Results: map[string]interface{}{
			"rows_affected": rowsAffected,
			"sql":           sql, // Include SQL for debugging (remove in production)
		},
		Loc: new_call_flow,
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
	ctx context.Context,
	rc ApiTypes.RequestContext,
	body []byte,
	user_name string) (int, ApiTypes.JimoResponse) {

	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_983", call_flow))

	var req ApiTypes.DeleteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log_id := sysdatastores.NextActivityLogID()
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_988", call_flow)
		error_msg := fmt.Sprintf("failed parse request_type:%v, log_id:%d", err, log_id)
		sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
			LogID:        log_id,
			ActivityName: ApiTypes.ActivityName_JimoRequest,
			ActivityType: ApiTypes.ActivityType_BadRequest,
			AppName:      ApiTypes.AppName_RequestHandler,
			ModuleName:   ApiTypes.ModuleName_RequestHandler,
			ActivityMsg:  &error_msg,
			CallerLoc:    new_call_flow})

		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_641)", reqID, error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_024", call_flow)
		error_msg := fmt.Sprintf("[req=%s] ***** unrecognized database type (5s): %s, loc:%s", reqID, db_type, new_call_flow)
		log.Printf("%s", error_msg)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] (SHD_RHD_036) handleDBDelete:%s:%s", reqID, db_name, table_name)
	log.Printf("[req:%s] (SHD_RHD_037) FieldDefs:%d", reqID, len(field_defs))

	if table_name == "" {
		error_msg := "failed get table name. Resource name"
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_041", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)

		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	log.Printf("[req:%s] (SHD_RHD_053) Delete records, table:%s.%s", reqID, db_name, table_name)

	field_map := make(map[string]bool)
	for _, fd := range field_defs {
		field_map[fd.FieldName] = true
	}

	cond_def := req.Condition
	expr, err := buildConditionExpr(new_ctx, table_name, cond_def, field_map)
	if err != nil {
		error_msg := fmt.Sprintf("failed building conditions, err:%v", err)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_064", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	if expr == nil {
		error_msg := fmt.Sprintf("missing conditions, loc:%s", req.Loc)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_077", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_098", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_BadRequest, resp
	}

	// Execute the update query
	// Assuming you have a database connection variable called 'db'
	// Replace 'db' with your actual database connection variable
	result, err := db.Exec(sql, args...)
	if err != nil {
		error_msg := fmt.Sprintf("failed to execute update query: %v", err)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_115", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Get the number of affected rows
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		error_msg := fmt.Sprintf("failed to get rows affected: %v", err)
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_130", call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (%s)", reqID, error_msg, new_call_flow)
		resp := ApiTypes.JimoResponse{
			Status:   false,
			ReqID:    reqID,
			ErrorMsg: error_msg,
			Loc:      new_call_flow,
		}
		return ApiTypes.CustomHttpStatus_InternalError, resp
	}

	// Success response
	new_call_flow := fmt.Sprintf("%s->SHD_RHD_142", call_flow)
	resp := ApiTypes.JimoResponse{
		Status:     true,
		ReqID:      reqID,
		ResultType: "json",
		NumRecords: 1,
		Results: map[string]interface{}{
			"rows_affected": rowsAffected,
			"sql":           sql, // Include SQL for debugging (remove in production)
		},
		Loc: new_call_flow,
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
	ctx context.Context,
	rc ApiTypes.RequestContext,
	req ApiTypes.QueryRequest,
	db *sql.DB,
	query string,
	args []interface{},
	selected_fields []string,
	aliases []string,
	field_def_map map[string][]ApiTypes.FieldDef) ([]map[string]interface{}, int, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	rows, err := db.Query(query, args...)
	if err != nil {
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_186", call_flow)
		log.Printf("[req:%s] ***** Alarm:%v (%s)", reqID, err, new_call_flow)
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_216", call_flow)
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Printf("[req:%s] ***** Alarm:scan error: %v (%s)", reqID, err, new_call_flow)
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
				new_call_flow := fmt.Sprintf("%s->SHD_RHD_254", call_flow)
				error_msg := fmt.Sprintf("field not found (%s):%s, selected:%v, data_types:%v",
					new_call_flow, field_name, selected_fields, data_types)
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_272", call_flow)
		error_msg := fmt.Sprintf("rows error: %v (%s)", err, new_call_flow)
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
	ctx context.Context,
	rc ApiTypes.RequestContext,
	objMap map[string]interface{},
	resource_name string,
	field_name string) (string, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	if value_obj, ok := objMap[field_name]; ok {
		if value_str, ok := value_obj.(string); ok {
			return value_str, nil
		}

		new_call_flow := fmt.Sprintf("%s->SHD_RHD_372", call_flow)
		error_msg := fmt.Sprintf("value is not a string, field_name:%s, resource_name:%s (%s)",
			field_name, resource_name, new_call_flow)
		log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
		err := fmt.Errorf("%s", error_msg)
		return "", err
	}

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_380", call_flow)
	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (%s)",
		field_name, resource_name, new_call_flow)
	log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
	return "", fmt.Errorf("%s", error_msg)
}

func GetFieldStrArrayValue(
	ctx context.Context,
	rc ApiTypes.RequestContext,
	objMap map[string]interface{},
	resource_name string,
	field_name string) ([]string, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)

	if value_obj, ok := objMap[field_name]; ok {
		if value_slice, ok := value_obj.([]interface{}); ok {
			result := make([]string, len(value_slice))
			for i, v := range value_slice {
				if str, ok := v.(string); ok {
					result[i] = str
				} else {
					new_call_flow := fmt.Sprintf("%s->SHD_RHD_400", call_flow)
					return nil, fmt.Errorf("element at index %d is not a string, got %T, loc:%s",
						i, v, new_call_flow)
				}
			}
			return result, nil
		}
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_406", call_flow)
		return nil, fmt.Errorf("field %s is not a []interface{} type, got %T, loc:%s",
			field_name, value_obj, new_call_flow)
	}

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_410", call_flow)
	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (665)",
		field_name, resource_name)
	log.Printf("[req:%s] +++++ Warn:%s (%s)", reqID, error_msg, new_call_flow)
	return nil, nil
}

func GetFieldAnyArrayValue(
	ctx context.Context,
	rc ApiTypes.RequestContext,
	objMap map[string]interface{},
	resource_name string,
	field_name string) ([]interface{}, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)

	if value_obj, ok := objMap[field_name]; ok {
		if result, ok := value_obj.([]interface{}); ok {
			return result, nil
		}
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_427", call_flow)
		return nil, fmt.Errorf("field %s is not a []interface{} type, got %T, loc:%s",
			field_name, value_obj, new_call_flow)
	}

	new_call_flow := fmt.Sprintf("%s->SHD_RHD_431", call_flow)
	error_msg := fmt.Sprintf("field not exist, field_name:%s, resource_name:%s (%s)",
		field_name, resource_name, new_call_flow)
	log.Printf("[req:%s] +++++ Warn:%s (%s)", reqID, error_msg, new_call_flow)
	return nil, fmt.Errorf("%s", error_msg)
}

func GetTableName(
	ctx context.Context,
	rc ApiTypes.RequestContext,
	resource_def map[string]interface{},
	resource_name string) (string, string, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_233", call_flow))

	// It retrieves: db_name and table_name. db_name is optional.
	db_name, _ := GetFieldStrValue(new_ctx, rc, resource_def, resource_name, "db_name")

	table_name, err := GetFieldStrValue(new_ctx, rc, resource_def, resource_name, "table_name")
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
	ctx context.Context,
	table_name string,
	condition ApiTypes.CondDef,
	field_map map[string]bool) (sq.Sqlizer, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_233", call_flow))

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
			new_call_flow := fmt.Sprintf("%s->SHD_RHD_527", call_flow)
			return nil, fmt.Errorf("invalid field name: %s, field_map:%v in table:%s, loc:%s",
				field, field_map, table_name, new_call_flow)
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
					new_call_flow := fmt.Sprintf("%s->SHD_RHD_524", call_flow)
					return nil, fmt.Errorf("CONTAIN operator requires string value, got %T, table_name:%s, loc:%s",
						rawValue, table_name, new_call_flow)
				}
				expr = sq.Like{field: "%" + strVal + "%"}
			} else {
				new_call_flow := fmt.Sprintf("%s->SHD_RHD_529", call_flow)
				return nil, fmt.Errorf("CONTAIN operator only supported for string type, got %s, table_name:%s, loc:%s",
					dataType, table_name, new_call_flow)
			}
		case Prefix:
			if dataType == "string" {
				strVal, ok := rawValue.(string)
				if !ok {
					new_call_flow := fmt.Sprintf("%s->SHD_RHD_536", call_flow)
					return nil, fmt.Errorf("PREFIX operator requires string value, got %T, table_name:%s, loc:%s", rawValue, table_name, new_call_flow)
				}
				expr = sq.Like{field: strVal + "%"}
			} else {
				new_call_flow := fmt.Sprintf("%s->SHD_RHD_541", call_flow)
				return nil, fmt.Errorf("PREFIX operator only supported for string type, got %s, table_name:%s, loc:%s", dataType, table_name, new_call_flow)
			}
		default:
			new_call_flow := fmt.Sprintf("%s->SHD_RHD_545", call_flow)
			return nil, fmt.Errorf("unsupported operator (SHD_RHD_319): %s, table_name:%s, loc:%s", condition.Opr, table_name, new_call_flow)
		}
		return expr, nil

	case ApiTypes.ConditionTypeAnd:
		// Build AND of multiple conditions
		if len(condition.Conditions) == 0 {
			new_call_flow := fmt.Sprintf("%s->SHD_RHD_553", call_flow)
			return nil, fmt.Errorf("AND condition must have at least one sub-condition, table_name:%s, loc:%s", table_name, new_call_flow)
		}

		var subExprs []sq.Sqlizer
		for _, subCond := range condition.Conditions {
			expr, err := buildConditionExpr(new_ctx, table_name, subCond, field_map)
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
			new_call_flow := fmt.Sprintf("%s->SHD_RHD_573", call_flow)
			return nil, fmt.Errorf("OR condition must have at least one sub-condition, table_name:%s, loc:%s", table_name, new_call_flow)
		}

		var subExprs []sq.Sqlizer
		for _, subCond := range condition.Conditions {
			expr, err := buildConditionExpr(new_ctx, table_name, subCond, field_map)
			if err != nil {
				return nil, err
			}

			if expr != nil {
				subExprs = append(subExprs, expr)
			}
		}
		return sq.Or(subExprs), nil

	default:
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_591", call_flow)
		return nil, fmt.Errorf("unknown condition type: %s, table_name:%s, loc:%s",
			condition.Type, table_name, new_call_flow)
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
	ctx context.Context,
	req ApiTypes.QueryRequest) (string, []interface{}, []string, []string, map[string][]ApiTypes.FieldDef, error) {
	call_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	reqID := ctx.Value(ApiTypes.RequestIDKey).(string)
	new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, fmt.Sprintf("%s->SHD_RHD_644", call_flow))

	db_name := req.DBName
	table_name := req.TableName
	if table_name == "" {
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_646", call_flow)
		error_msg := fmt.Sprintf("missing table name, db:%s, table:%s, loc:%s",
			db_name, table_name, new_call_flow)
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_630", call_flow)
		error_msg := fmt.Sprintf("missing selected fields, table name:%s, loc:%s", table_name, new_call_flow)
		log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
		return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
	}

	if len(field_defs) == 0 {
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_637", call_flow)
		error_msg := fmt.Sprintf("missing field_defs, table name:%s, loc:%s", table_name, new_call_flow)
		log.Printf("[req:%s] ***** Alarm:%s (SHD_RHD_323)", reqID, error_msg)
		return "", nil, nil, nil, nil, fmt.Errorf("%s", error_msg)
	}

	query_cond := req.Condition
	log.Printf("[req:%s] (SHD_RHD_679) Table:%s.%s, selected fields:%v, condition:%s, loc:%s",
		reqID, db_name, table_name, selected_fields, query_cond, req.Loc)

	// Note: the field map assumes field names are not full names (i.e.,
	// tablename.fieldname). This is okey for table conditions. Join
	// conditions should be defined in Joins.
	field_map := make(map[string]bool)
	for _, fd := range field_defs {
		// full_name := fmt.Sprintf("%s.%s", table_name, fd.FieldName)
		field_map[fd.FieldName] = true
	}

	expr, err := buildConditionExpr(new_ctx, table_name, query_cond, field_map)
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
		new_call_flow := fmt.Sprintf("%s->SHD_RHD_724", call_flow)
		error_msg := fmt.Sprintf("failed building query:%v, loc:%s", err, new_call_flow)
		log.Printf("[req:%s] ***** Alarm:%s", reqID, error_msg)
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
