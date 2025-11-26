/////////////////////////////////////////////////////
// Description:
//  - Define InMemStore and StoreMap
// Created: 2025/11/01 by Chen Ding
/////////////////////////////////////////////////////

import { writable, type Writable } from 'svelte/store';

export type RecordInfo = {[key: string]:any}
export type InMemStoreDef = {
    CachedRecords:      RecordInfo[];
    CrtRecord:          RecordInfo;
    CrtView:            string;
    TotalRecords:       number;
    LimitSize:          number;
}

export type StoreRecord = {
    InMemStoreCrtRecords:   Writable<any>; 
    InMemStoreCrtRecord:    Writable<RecordInfo>; 
    InMemStoreCrtView:      Writable<string>;
};

export const StoreMap = new Map<string, Writable<InMemStoreDef>>()

export function InitInMemStore(
            store_name: string, 
            init_view: string,
            limit_size: number = 200) {
    var componentStores = StoreMap.get(store_name);
    if (!componentStores) {
        const in_mem_store: InMemStoreDef = {
            CachedRecords:  [],
            CrtRecord:      {},
            CrtView:        init_view,
            TotalRecords:   0,
            LimitSize:      limit_size
        }

        const store = writable(in_mem_store);
        StoreMap.set(store_name, store);
    }
}

// Export function to get stores by component name
export function GetStoreByName(
            store_name: string,
            init_view: string = '',
            record_set_size: number = 200): Writable<InMemStoreDef> {
    if (Object.hasOwn(StoreMap, "store_name")) {
        return StoreMap.get(store_name) as Writable<InMemStoreDef>
    }
    InitInMemStore(store_name, init_view, record_set_size)
    return StoreMap.get(store_name) as Writable<InMemStoreDef>
}
    
// Export function to get all store names
export function GetAllStoreNames() {
    return Array.from(StoreMap.keys());
}