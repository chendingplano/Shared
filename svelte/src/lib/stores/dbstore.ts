import (
    "os"
)

import { z, ZodType, ZodObject, type ZodRawShape } from "zod"
import type {
            JimoRequest, 
            QueryRequest,
            UpdateRequest,
            InsertRequest,
            DeleteRequest,
            JimoResponse, 
            JsonObjectOrArray,
            CondDef,
            JoinDef,
            OrderbyDef,
            QueryResults,
            UpdateWithCondDef, 
            UpdateDef} from '$lib/types/CommonTypes';
import {ParseObjectOrArray} from '$lib/utils/UtilFuncs'
import {CustomHttpStatus, RequestType} from '$lib/types/CommonTypes';
import { StatusCodes } from 'http-status-codes';

export type QueryResult = {
    valid:          boolean,
    error_msg:      string,
    data:           JsonObjectOrArray
}

export const EmptyJimoResponse: JimoResponse = {
    status:         false,
    error_msg:      '',
    error_code:     0,
    req_id:         '',
    result_type:    'none',
    table_name:     '',
    num_records:    0,
    results:        '',
    loc:            'SHD_DST_029'
    }

// checkSystemResp checks the system response codes.
// If it is a system response code, it means the response
// is not a JimoResponse. It constructs a JimoResponse and returns it.
// Otherwise, it returns [false, anything] to let the caller
// parse the JimoResponse.
function checkSystemResp(resp: Response): [boolean, JimoResponse] {
    if (resp.status === 401) {
        const error_msg = "Operation rejected (401):" + resp.statusText
        return [true, {
                status: false,
                error_msg: error_msg,
                error_code: resp.status,
                result_type: 'http_error',
                results: '',
                loc: 'SHD_DST_022'
                } as JimoResponse]
    }

    if (resp.status === 404) {
        const error_msg = "404:" + resp.statusText
        return [true, {
                        status: false,
                        error_msg: error_msg,
                        error_code: resp.status,
                        result_type: 'http_error',
                        results: '',
                        loc: 'SHD_DST_034'
                    } as JimoResponse]
    }

    // Special handling for 550 status code, which is a custom code
    // returned by our server.
    if (resp.status === CustomHttpStatus.NotLoggedIn) {
        const error_msg = "User not logged in:" + resp.statusText
        return [true, {
                        status: false,
                        error_msg: error_msg,
                        error_code: resp.status,
                        result_type: 'http_error',
                        results: '',
                        loc: 'SHD_DST_048'
                    } as JimoResponse]
    }

    if (resp.status < CustomHttpStatus.Success) {
        // The returned is not a JSON doc. It is returned by the system.
        const error_msg = `Server returned ${resp.statusText}`
        return [true, {
                        status: false,
                        error_msg: error_msg,
                        error_code: resp.status,
                        result_type: 'http_error',
                        results: '',
                        loc: 'SHD_DST_060'
                    } as JimoResponse]
    }

    // The response should be a JimoResponse. Let the caller parses it.
    return [false, EmptyJimoResponse]
}

async function handleResp(resp: Response): Promise<JimoResponse> {
    if (!resp.ok) {
        const [resp_generated, jimo_response] = checkSystemResp(resp)
        if (resp_generated) {
            return jimo_response
        }
    }

    try {
        const resp_json = await resp.json() as JimoResponse
        return resp_json
    } catch (e) {
		if(e instanceof Error) {
            const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${e.message}`;
            const rr = {
	            status:         false,
	            error_msg:      error_msg,
                result_type:    'exception',
                results:        '',
                error_code:     CustomHttpStatus.ServerException,
	            loc:            'SHD_DST_153'
            }
            return rr as JimoResponse
		}

		const error_msg = "Server exception:" + String(e);
        const rr = {
	        status:         false,
	        error_msg:      error_msg,
            result_type:    'exception',
            results:        '',
            error_code:     CustomHttpStatus.ServerException,
	        loc:            'SHD_DST_164'
        }
        return rr as JimoResponse
    }
}


/*
users.go

type UserTableDefinition {
    Name string `postgres:"TEXT"`
    Age int `postgres:"INT64"`
    Hello string `postgres"TEXT"`
}

func RegisterTables() {
    tableRegistrar.Add("alijwdlaijd", UserTableDefinition{})
}


type UserIDResponse {
    Name string `json:"name" postgres:"TEXT PRIMARY KEY"`
    Age int
}

func GetUserInfo(userID string) UserIDResponse {
    res := db.Query("SELECT name, age FROM users WHERE ID=$1", userID)
    return res
}

api.go

api.Add(GetUserInfo)

// run `mise generate` => run `jimo generate --input-directory "./src" --output-directory "./frontend/gen"

automatically generated javascript file:

type UserIDResponse {
    name: string
    age: number
}

async function getUserInfo(userID: string): UserIDResponse {
    const res = await fetch("/api/v1/UserService.GetUserInfo", { method: "POST", body: {userID: userID}});
    const res2 = await res.json();
    return res2 as UserIDResponse;
}

// this is what you'll run in javascript

<script lang="ts">
    let userInfo = $state<UserIDResponse | null>(null);

    onMount(() => {
        userInfo = await getUserInfo("abcd");
    })
</script>

*/

// Automatically generate: GraphQL/HTTP Request
// Select xxx from Users
class DBStore {
    constructor() {}

    getFileURL(user_name: string, filename: string, token: string): string {
        // The URL for a file is composed:
        // 'http://<domain_name>:<port>/<data_dir>/<username>/<filename>
        // TBD: will use config!!!
        if (typeof filename === "string" && filename.length > 0) {
            const frontendURLEnv = process.env.FRONTEND_URL
            return `http://localhost:5173/data/custom_files/${user_name}/${filename}`
        }
        return ""
    }

    async retrieveRecords(
            db_name: string,
            table_name: string,
            field_names: string[],
            field_defs: Record<string, unknown>[],
            loc: string,
            conds: CondDef,
            join_def: JoinDef[],
            orderby_def: OrderbyDef[],
            record_schema: unknown| null,
            embed_name: string,
            embed_schema: unknown| null,
            start: number,
            num_records: number) : Promise<JimoResponse> {
        const rsc_info = {
            start: 0,
            num_records: num_records
        }
        const rsc_str = JSON.stringify(rsc_info)
        const req : QueryRequest = {
            request_type:   RequestType.Query,
            db_name:        db_name,
            table_name:     table_name,
            field_defs:     field_defs,
            condition:      conds,
            join_def:       join_def,
            field_names:    field_names,
            orderby_def:    orderby_def,
            start:          start,
            page_size:      num_records,
            loc:            loc
        }

        try {
            const resp = await fetch("/shared_api/v1/jimo_req", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(req),
                credentials: 'include'
                });

            if (!resp.ok) {
                const [resp_generated, jimo_response] = checkSystemResp(resp)
                if (resp_generated) {
                    return jimo_response
                }
            }

            try {
                const text = await resp.text();
                console.log('Raw response (SHD_DBS_251):', text); // 👈 Look at this!
                const resp_json = JSON.parse(text) as JimoResponse;
                if (resp_json.status) {
                    if (resp_json.num_records <= 0) {
                        return resp_json
                    }
                    if (record_schema) {
                        const r_schema = record_schema as z.ZodType;
                        if (resp_json.result_type === "json_array") {
                            const results = resp_json.results
                            if (Array.isArray(results)) {
                                const valid_records: Record<string, unknown>[] = []
                                for (const record of results) {
                                    const result = r_schema.safeParse(record as unknown)
                                    if (result.success) {
                                        if (embed_schema) {
                                            const rr = record as Record<string, unknown>
                                            if (typeof rr[embed_name] !== 'object') {
                                                console.warn(`Missing/incorrect embedded object (SHD_DBS_280):${embed_name}, type:${typeof rr[embed_name]}`)
                                            } else {
                                                const e_schema = embed_schema as z.ZodType;
                                                console.log(`Embed object (SHD_DBS_283):${rr[embed_name]}`)
                                                const result1 = e_schema.safeParse(rr[embed_name] as unknown)
                                                if (result1.success) {
                                                    valid_records.push(record as Record<string, unknown>)
                                                } else {
                                                    console.warn(`Embed object validation failed, name:${embed_name} (${loc}:SHD_DBS_275)`, {
                                                        errors: result1.error.issues,
                                                        record: result.data,
                                                    })
                                                }
                                            }
                                        } else {
                                            valid_records.push(record as Record<string, unknown>)
                                        }
                                    } else {
                                        console.log("Error 2 (SHD_DBS_293)")
                                        console.warn(`Object validation failed (${loc}:SHD_DBS_299)`, {
                                            errors: result.error.issues,
                                            record: result.data,
                                        })
                                    }
                                }
                                resp_json.results = valid_records
                            } else {
                                console.warn(`Expecting an array but got:${typeof results}, loc:${loc}:ARX_DBS_291)`)
                            }
                        }
                    }
                }
                // const resp_json = await resp.json() as JimoResponse
                /*
                if (resp_json.status) {
                    const data_str = resp_json.results
                    const records = ParseObjectOrArray(data_str) as JsonObjectOrArray
                    resp_json.records = records
                }
                */
                return resp_json
            } catch (e) {
		        if (e instanceof Error) {
                    const error_msg = `Server returned ${resp.status} ${resp.statusText}, get exception: ${e.message}`;
                    return {
	                    status:         false,
	                    error_msg:      error_msg,
                        result_type:    'exception',
                        results:        '',
                        error_code:     CustomHttpStatus.ServerException,
	                    loc:            'SHD_DST_153'
                    } as JimoResponse
		        }

		        const error_msg = "Error fetching data:" + String(e);
                const rr = {
	                status:         false,
	                error_msg:      error_msg,
                    result_type:    'exception',
                    results:        '',
                    error_code:     CustomHttpStatus.ResourceNotFound,
	                loc:            'SHD_DST_164'
                }
                return rr as JimoResponse
            }
        } catch (e) {
            if (e instanceof Error) {
                const error_msg = "Error fetching data:" + e.message;
                const resp = {
                    status:         false,
                    error_msg:      error_msg,
                    result_type:    'exception',
                    error_code:     CustomHttpStatus.ServerException,
                    table_name:     table_name,
                    num_records:    0,
                    results:        '',
                    loc:            'SHD_DST_176'
                }
                return resp
            }   

            const error_msg = "Error fetching data:" + e;
            const resp = {
                status:         false,
                error_msg:      error_msg,
                result_type:    'exception',
                results:        '',
                error_code:     CustomHttpStatus.ServerException,
                loc:            'SHD_DST_188'
            }
        }
        const error_msg = "Unknown error occurred.";
        return {
            status:         false,
            error_msg:      error_msg,
            result_type:    'exception',
            table_name:     table_name,
            num_records:    0,
            results:        '',
            error_code:     CustomHttpStatus.ServerException,
            loc:            'SHD_DST_196'
        }
    }

    async deleteRecord(
            db_name: string,
            table_name: string,
            cond_def: CondDef,
            field_defs: Record<string, unknown>[],
            delete_single: boolean,
            loc: string): Promise<[boolean, number, string]> {
        const req : DeleteRequest = {
            request_type:   RequestType.Delete,
            db_name:        db_name,
            table_name:     table_name,
            condition:      cond_def,
            field_defs:     field_defs,
            loc:            loc
        }

        try {
            const resp = await fetch("/shared_api/v1/jimo_req", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(req),
                credentials: 'include'
                });

            if (!resp.ok) {
                const [resp_generated, jimo_response] = checkSystemResp(resp)
                if (resp_generated) {
                    const error_msg = jimo_response.error_msg + 
                        ", loc:" + jimo_response.loc
                    return [false, jimo_response.error_code, error_msg]
                }
            }

            try {
                const resp_json = await resp.json() as JimoResponse
                if (resp_json.status) {
                    // const data_str = resp_json.results
                    // const records = ParseObjectOrArray(data_str) as JsonObjectOrArray
                    // resp_json.records = records
                    return [true, 200, ""]
                }
                const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${resp_json.error_msg}, loc:${resp_json.loc}`;
                return [false, resp_json.error_code, error_msg]
            } catch (e) {
		        if (e instanceof Error) {
                    const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${e.message}`;
                    return [false, CustomHttpStatus.ServerException, error_msg]
		        }

                const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${String(e)}`;
                return [false, CustomHttpStatus.ServerException, error_msg]
            }
        } catch (e) {
            if (e instanceof Error) {
                const error_msg = "Error fetching data (SHD_DST_286):" + e.message;
                return [false, CustomHttpStatus.ServerException, error_msg]
            }   

            const error_msg = "Error fetching data (SHD_DST_290):" + e;
            return [false, CustomHttpStatus.ServerException, error_msg]
        }
    }

    async saveRecord(
            db_name: string,
            table_name: string,
            field_defs: Record<string, unknown>[],
            records: Record<string, unknown>[],
            on_conflict_cols: string[],
            on_conflict_update_cols: string[],
            loc: string) : Promise<JimoResponse> {

        if (records.length <= 0) {
            const error_msg = "record is null"
            return {
	            status:         false,
	            error_msg:      error_msg,
                result_type:    '',
                results:        '',
                error_code:     CustomHttpStatus.BadRequest,
	            loc:            'SHD_DST_099'
            } as JimoResponse
        }

        const req : InsertRequest = {
            request_type:               RequestType.Insert,
            db_name:                    db_name,
            table_name:                 table_name,
            records:                    records,
            field_defs:                 field_defs,
            on_conflict_cols:           on_conflict_cols,
            on_conflict_update_cols:    on_conflict_update_cols,
            loc:                        loc,
        }

        try {
            const resp = await fetch("/shared_api/v1/jimo_req", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(req),
                credentials: 'include'
                });

            if (!resp.ok) {
                const [resp_generated, jimo_response] = checkSystemResp(resp)
                if (resp_generated) {
                    return jimo_response
                }
            }

            try {
                const resp_json = await resp.json() as JimoResponse
                return resp_json
            } catch (e) {
		        if (e instanceof Error) {
                    const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${e.message}`;
                    const rr = {
	                    status:         false,
	                    error_msg:      error_msg,
                        result_type:    'exception',
                        results:        '',
                        error_code:     resp.status,
	                    loc:            'SHD_DST_153'
                    }
                    return rr as JimoResponse
		        }

		        const error_msg = "Error fetching data:" + String(e);
                const rr = {
	                status:         false,
	                error_msg:      error_msg,
                    result_type:    'exception',
                    results:        '',
                    error_code:     500,
	                loc:            'SHD_DST_164'
                }
                return rr as JimoResponse
            }
        } catch (e) {
            if (e instanceof Error) {
                const error_msg = "Error fetching data:" + e.message;
                const resp = {
                    status:         false,
                    error_msg:      error_msg,
                    result_type:    'exception',
                    error_code:     500,
                    table_name:     table_name,
                    num_records:    0,
                    results:        '',
                    loc:            'SHD_DST_176'
                }
                return resp
            }   

            const error_msg = "Error fetching data:" + e;
            const resp = {
                status:         false,
                error_msg:      error_msg,
                result_type:    'exception',
                results:        '',
                error_code:     500,
                loc:            'SHD_DST_188'
            }
        }
        const error_msg = "Unknown error occurred.";
        return {
            status:         false,
            error_msg:      error_msg,
            result_type:    'exception',
            num_records:    0,
            table_name:     table_name,
            results:        '',
            error_code:     500,
            loc:            'SHD_DST_196'
        }
    }

    // updateRecord updates a record. The fields to be updated
    // is in 'update_entries'.
    async updateRecord(
            db_name: string,
            table_name: string,
            field_defs: Record<string, unknown>[],
            condition: CondDef,
            record: Record<string, unknown>,
            update_entries: UpdateDef[],
            on_conflict_cols: string[],
            on_conflict_update_cols: string[],
            need_record: boolean,
            loc: string) : Promise<JimoResponse> {
        const req : UpdateRequest = {
            request_type:               RequestType.Update,
            db_name:                    db_name,
            table_name:                 table_name,
            field_defs:                 field_defs,
            condition:                  condition,
            record:                     record,
            update_entries:             update_entries,
            on_conflict_cols:           on_conflict_cols,
            on_conflict_update_cols:    on_conflict_update_cols,
            need_record:                need_record,
            loc:                        loc,
        }

        try {
            const resp = await fetch("/shared_api/v1/jimo_req", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(req),
                credentials: 'include'
                });

            const resp_json = await handleResp(resp)
            return resp_json
        } catch (e) {
            if (e instanceof Error) {
                const error_msg = `Error fetching data: ${e.message}`;
                return {
                    status:         false,
                    error_msg:      error_msg,
                    result_type:    'exception',
                    table_name:     table_name,
                    num_records:    0,
                    results:        '',
                    error_code:     CustomHttpStatus.ServerException,
                    loc:            'SHD_DST_464'
                }
            }   

            const error_msg = `Error fetching data: ${e}`;
            return {status:         false,
                    error_msg:      error_msg,
                    result_type:    'exception',
                    num_records:    0,
                    table_name:     table_name,
                    results:        '',
                    error_code:     CustomHttpStatus.ServerException,
                    loc:            'SHD_DST_464'
            }
        } 
    }

    /*
    async updateMultipleRecords(
            db_name: string,
            table_name: string,
            field_defs: Record<string, unknown>[],
            update_entries: UpdateWithCondDef,
            loc: string) : Promise<JimoResponse> {
        if (update_entries === null || update_entries === undefined) {
            const error_msg = "missing update_entries"
            return {
	            status:         false,
	            error_msg:      error_msg,
                result_type:    '',
                results:        '',
                error_code:     CustomHttpStatus.InvalidRequest,
	            loc:            'SHD_DST_429'
            } as JimoResponse
        }

        // IMPORTANT: Not implemented yet on the backend (Chen Ding, 2025/12/11)!!!
        const req : JimoRequest = {
            request_type:   "resource_request",
            action:         "update_multiple",
            resource_name:  table_name,
            db_name:        db_name,
            table_name:     table_name,
            field_defs:     field_defs,
            conditions:     [],
            resource_info:  '',
            loc:            loc,
            records:        update_entries
        }

        try {
            const resp = await fetch("/shared_api/v1/jimo_req", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(req),
                credentials: 'include'
                });

            return handleResp(resp)
        } catch (e) {
            if (e instanceof Error) {
                const error_msg = "Error fetching data:" + e.message;
                const resp = {
                    status:         false,
                    error_msg:      error_msg,
                    result_type:    'exception',
                    error_code:     CustomHttpStatus.ServerException,
                    results:        '',
                    loc:            'SHD_DST_176'
                }
                return resp
            }   

            const error_msg = "Error fetching data:" + e;
            const resp = {
                status:         false,
                error_msg:      error_msg,
                result_type:    'exception',
                results:        '',
                error_code:     CustomHttpStatus.ServerException,
                loc:            'SHD_DST_188'
            }
            return resp
        }
    }
    */

    async getToken() : Promise<[string, number, string]> {
        return ["token-12345", StatusCodes.OK, '']
        // return ["", CustomHttpStatus.NotImplementedYet, "NotImplementedYet"]
    }

}

// ✅ Create and export a SINGLE instance
export const db_store = new DBStore();