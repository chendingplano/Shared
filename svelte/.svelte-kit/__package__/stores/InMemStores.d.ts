import { type Writable } from 'svelte/store';
export type RecordInfo = {
    [key: string]: any;
};
export type InMemStoreDef = {
    CachedRecords: RecordInfo[];
    CrtRecord: RecordInfo;
    CrtView: string;
    TotalRecords: number;
    LimitSize: number;
};
export type StoreRecord = {
    InMemStoreCrtRecords: Writable<any>;
    InMemStoreCrtRecord: Writable<RecordInfo>;
    InMemStoreCrtView: Writable<string>;
};
export declare const StoreMap: Map<string, Writable<InMemStoreDef>>;
export declare function InitInMemStore(store_name: string, init_view: string, limit_size?: number): void;
export declare function GetStoreByName(store_name: string, init_view?: string, record_set_size?: number): Writable<InMemStoreDef>;
export declare function GetAllStoreNames(): string[];
