import type { JimoResponse, JsonObjectOrArray } from '../types/CommonTypes';
import type { QueryResults } from '../types/DBTypes';
import type { CondDef } from '../types/DBTypes';
export type QueryResult = {
    valid: boolean;
    error_msg: string;
    data: JsonObjectOrArray;
};
declare class DBStore {
    constructor();
    retrieveRecords(resource_name: string, field_names: string[], conds?: CondDef[], num_records?: number): Promise<QueryResults>;
    saveRecord(resource_name: string, record: Record<string, unknown>): Promise<JimoResponse>;
    upsert(id: string, definition: any): Promise<void>;
}
export declare const db_store: DBStore;
export {};
