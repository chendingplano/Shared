/////////////////////////////////////////////////////
// Description:
//  - Define InMemStore and StoreMap
// Created: 2025/11/01 by Chen Ding
/////////////////////////////////////////////////////
import { writable } from 'svelte/store';
export const StoreMap = new Map();
export function InitInMemStore(store_name, init_view, limit_size = 200) {
    var componentStores = StoreMap.get(store_name);
    if (!componentStores) {
        const in_mem_store = {
            CachedRecords: [],
            CrtRecord: {},
            CrtView: init_view,
            TotalRecords: 0,
            LimitSize: limit_size
        };
        const store = writable(in_mem_store);
        StoreMap.set(store_name, store);
    }
}
// Export function to get stores by component name
export function GetStoreByName(store_name, init_view = '', record_set_size = 200) {
    if (Object.hasOwn(StoreMap, "store_name")) {
        return StoreMap.get(store_name);
    }
    InitInMemStore(store_name, init_view, record_set_size);
    return StoreMap.get(store_name);
}
// Export function to get all store names
export function GetAllStoreNames() {
    return Array.from(StoreMap.keys());
}
