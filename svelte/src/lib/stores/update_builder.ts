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

import type { UpdateDef } from "$lib/types/CommonTypes";

// Base class for building joins
class UpdateBuilder {
    protected updateDefs: UpdateDef[] = [];

    constructor() {}

    modify(fieldName: string, value: unknown, dataType: string): this {
        this.updateDefs.push({
            field_name: fieldName,
            value: value,
            data_type: dataType
        })
        return this
    }

    // Build the final join definition
    build(): UpdateDef[] {
        return this.updateDefs;
    }
}

class UpdateConstructor {
    start(): UpdateBuilder {
        return new UpdateBuilder()
    }
}

// Global instance for easy access
const update_builder = new UpdateConstructor();

export { 
    update_builder, 
    UpdateBuilder
};