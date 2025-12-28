///////////////////////////////////////////////////////
// Description:
// It implements an update_builder that we can use to build
// update entries (an array of UpdateDef).
// Below are some examples of how to use it to build joins.
// 
// Example 1: Single entry
// const update_entry = update_builder
//     .begin()
//     .update('users', 'abc', 'string)
//     .build();
//
// This creates:
// [
//      {
//          field_name: "user",
//          value: "abc",
//          data_type: "string"
//      }
// ]
//
// Example 2: Multiple entries
// const update_entries = update_builder
//    .begin()
//    .modify('order_num', 12345, "int")
//    .modify('remarks', "Regenerated order number", "string")
//    .build();
//
// Created: 2025/12/15 by Chen Ding
///////////////////////////////////////////////////////

import type { OrderbyDef } from "$lib/types/CommonTypes";

// Base class for building joins
class OrderbyBuilder {
    protected orderbyDefs: OrderbyDef[] = [];

    constructor() {}

    orderby(fieldName: string, dataType: string, is_asc: boolean): this {
        this.orderbyDefs.push({
            field_name: fieldName,
            is_asc: is_asc,
            data_type: dataType
        })
        return this
    }

    // Build the final join definition
    build(): OrderbyDef[] {
        return this.orderbyDefs;
    }
}

class OrderbyConstructor {
    start(): OrderbyBuilder {
        return new OrderbyBuilder()
    }
}

// Global instance for easy access
const orderby_builder = new OrderbyConstructor();

export { 
    orderby_builder, 
    OrderbyBuilder
};