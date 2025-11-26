import { ParseObjectOrArray } from '../utils/UtilFuncs';
class DBStore {
    constructor() { }
    async retrieveRecords(resource_name, field_names, conds = [], num_records = 200) {
        const cond_str = JSON.stringify(conds);
        const rsc_info = {
            start: 0,
            num_records: num_records
        };
        const rsc_str = JSON.stringify(rsc_info);
        const req = {
            request_type: "db_opr",
            action: "query",
            resource_name: resource_name,
            resource_opr: "select_with_fields",
            conditions: cond_str,
            resource_info: rsc_str,
        };
        const resp = await fetch("/shared_api/v1/jimo_req", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(req),
            credentials: 'include'
        });
        try {
            const json_value = await resp.json();
            if (resp.ok) {
                const data_str = json_value.results;
                const data_json = ParseObjectOrArray(data_str);
                return {
                    status: true,
                    error_msg: '',
                    results: data_json,
                    loc: json_value.loc
                };
            }
            if (resp.status === 401) {
                const error_msg = "Operation rejected (401):" + resp.statusText + " (SHD_DST_054)";
                return {
                    status: false,
                    error_msg: error_msg,
                    results: null,
                    loc: 'SHD_DST_062'
                };
            }
            if (resp.status === 404) {
                const error_msg = "Route not found (404):" + resp.statusText + " (SHD_DST_059)";
                return {
                    status: false,
                    error_msg: error_msg,
                    results: null,
                    loc: 'SHD_DST_062',
                };
            }
            const error_msg = json_value.error_msg + " (" + json_value.loc + ":SHD_DST_073)";
            return {
                status: false,
                error_msg: error_msg,
                results: null,
                loc: 'SHD_DST_081',
            };
        }
        catch (e) {
            if (e instanceof Error) {
                const error_msg = "Error fetching data:" + e.message + " (SHD_DST_081)";
                return {
                    status: false,
                    error_msg: error_msg,
                    results: null,
                    loc: 'SHD_DST_081',
                };
            }
            const error_msg = "Error fetching data:" + String(e) + " (SHD_DST_088)";
            return {
                status: false,
                error_msg: error_msg,
                results: null,
                loc: 'SHD_DST_081',
            };
        }
    }
    async saveRecord(resource_name, record) {
        console.log("Save Record (SHD_DST_099");
        if (record === null || record === undefined) {
            const error_msg = "record is null";
            return {
                status: false,
                error_msg: error_msg,
                result_type: '',
                results: '',
                loc: 'SHD_DST_099'
            };
        }
        const req = {
            request_type: "db_opr",
            action: "insert",
            resource_name: resource_name,
            resource_opr: "insert",
            conditions: '',
            resource_info: '',
            records: JSON.stringify(record)
        };
        const resp = await fetch("/shared_api/v1/jimo_req", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(req),
            credentials: 'include'
        });
        try {
            const json_value = await resp.json();
            return json_value;
        }
        catch (e) {
            if (e instanceof Error) {
                const error_msg = "Error fetching data:" + e.message;
                const resp = {
                    status: false,
                    error_msg: error_msg,
                    result_type: 'exception',
                    results: '',
                    loc: 'SHD_DST_140'
                };
                return resp;
            }
            const error_msg = "Error fetching data:" + String(e);
            const resp = {
                status: false,
                error_msg: error_msg,
                result_type: 'exception',
                results: '',
                loc: 'SHD_DST_150'
            };
            return resp;
        }
    }
    async upsert(id, definition) {
    }
}
// ✅ Create and export a SINGLE instance
export const db_store = new DBStore();
