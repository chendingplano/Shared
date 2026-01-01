///////////////////////////////////////////////////////
// Description:
// query_builder implements a fluent query builder interface
// similar to Drizzle ORM, which converts to DBStore::retrieveRecords calls.
//
// Example 1: Simple query
// const results = await query_builder
//     .select()
//     .from('users')
//     .where(cond_builder.and()
//         .condEq('status', 'active')
//         .condGt('age', 18)
//     )
//     .execute();
//
// Example 2: Query with joins
// const results = await query_builder
//     .select()
//     .from('posts')
//     .leftJoin(
//         join_builder.from('posts')
//             .join('users', 'left_join')
//             .on('posts.author_id', 'users.id', '=', 'string')
//             .select('username', 'email')
//             .embedAs('author')
//     )
//     .where(cond_builder.filter()
//         .condEq('posts.published', true)
//     )
//     .orderBy('posts.created_at', false) // DESC
//     .limit(10)
//     .execute();
//
// Example 3: Query with schema validation
// const results = await query_builder
//     .select()
//     .from('products')
//     .where(cond_builder.filter().condEq('category', 'electronics'))
//     .withSchema(ProductSchema)
//     .limit(50)
//     .execute();
//
// Example 4: Query with embedded schema
// const results = await query_builder
//     .select()
//     .from('orders')
//     .leftJoin(
//         join_builder.from('orders')
//             .join('customers', 'left_join')
//             .on('orders.customer_id', 'customers.id', '=', 'string')
//             .select('name', 'email')
//             .embedAs('customer')
//     )
//     .withSchema(OrderSchema)
//     .withEmbedSchema('customer', CustomerSchema)
//     .execute();
//
// Example 5: Selecting specific fields
// const results = await query_builder
//     .select('id', 'name', 'email')
//     .from('users')
//     .where(cond_builder.filter().condEq('active', true))
//     .execute();
//
// Example 6: Multiple joins
// const results = await query_builder
//     .select()
//     .from('posts')
//     .leftJoin(
//         join_builder.from('posts')
//             .join('users', 'left_join')
//             .on('posts.author_id', 'users.id', '=', 'string')
//             .select('username', 'avatar')
//             .embedAs('author')
//     )
//     .leftJoin(
//         join_builder.from('posts')
//             .join('categories', 'left_join')
//             .on('posts.category_id', 'categories.id', '=', 'string')
//             .select('category_name', 'slug')
//             .embedAs('category')
//     )
//     .execute();
//
// Example 7: Pagination
// const results = await query_builder
//     .select()
//     .from('articles')
//     .where(cond_builder.filter().condEq('status', 'published'))
//     .orderBy('published_at', false)
//     .offset(20)
//     .limit(10)
//     .execute();
//
// Created: 2026/01/01 by Chen Ding
///////////////////////////////////////////////////////

import type {
    CondDef,
    JoinDef,
    OrderbyDef,
    JimoResponse
} from '$lib/types/CommonTypes';
import { db_store } from './dbstore';
import { cond_builder } from './cond_builder';

/**
 * Query builder class that provides a fluent interface for building
 * and executing database queries using DBStore::retrieveRecords.
 */
class QueryBuilder {
    private _dbName: string = '';
    private _tableName: string = '';
    private _fieldNames: string[] = [];
    private _fieldDefs: Record<string, unknown>[] = [];
    private _condition: CondDef = cond_builder.null();
    private _joins: JoinDef[] = [];
    private _orderBy: OrderbyDef[] = [];
    private _recordSchema: unknown | null = null;
    private _embedName: string = '';
    private _embedSchema: unknown | null = null;
    private _start: number = 0;
    private _limit: number = 100;
    private _loc: string = 'query_builder';

    constructor() {}

    /**
     * Start building a SELECT query.
     * @param fields - Optional field names to select. If not provided, selects all fields.
     */
    select(...fields: string[]): this {
        this._fieldNames = fields;
        return this;
    }

    /**
     * Specify the database name.
     * @param dbName - The database name
     */
    database(dbName: string): this {
        this._dbName = dbName;
        return this;
    }

    /**
     * Specify the table to query from.
     * @param tableName - The table name
     */
    from(tableName: string): this {
        this._tableName = tableName;
        return this;
    }

    /**
     * Add WHERE conditions.
     * @param condition - A CondDef built using cond_builder
     */
    where(condition: CondDef): this {
        this._condition = condition;
        return this;
    }

    /**
     * Add a LEFT JOIN.
     * @param joinDef - A JoinDef built using join_builder
     */
    leftJoin(joinDef: JoinDef): this {
        // Ensure join_type is set to left_join
        joinDef.join_type = 'left_join';
        this._joins.push(joinDef);
        return this;
    }

    /**
     * Add a RIGHT JOIN.
     * @param joinDef - A JoinDef built using join_builder
     */
    rightJoin(joinDef: JoinDef): this {
        // Ensure join_type is set to right_join
        joinDef.join_type = 'right_join';
        this._joins.push(joinDef);
        return this;
    }

    /**
     * Add an INNER JOIN.
     * @param joinDef - A JoinDef built using join_builder
     */
    innerJoin(joinDef: JoinDef): this {
        // Ensure join_type is set to inner_join
        joinDef.join_type = 'inner_join';
        this._joins.push(joinDef);
        return this;
    }

    /**
     * Add a generic JOIN (defaults to INNER JOIN).
     * @param joinDef - A JoinDef built using join_builder
     */
    join(joinDef: JoinDef): this {
        // If join_type is not already set, default to inner_join
        if (!joinDef.join_type) {
            joinDef.join_type = 'inner_join';
        }
        this._joins.push(joinDef);
        return this;
    }

    /**
     * Add ORDER BY clause.
     * @param fieldName - The field to order by
     * @param ascending - True for ASC, false for DESC (default: true)
     * @param dataType - The data type of the field (default: 'string')
     */
    orderBy(fieldName: string, ascending: boolean = true, dataType: string = 'string'): this {
        this._orderBy.push({
            field_name: fieldName,
            data_type: dataType,
            is_asc: ascending
        });
        return this;
    }

    /**
     * Add multiple ORDER BY clauses.
     * @param orderDefs - Array of OrderbyDef objects
     */
    orderByMultiple(orderDefs: OrderbyDef[]): this {
        this._orderBy.push(...orderDefs);
        return this;
    }

    /**
     * Set the LIMIT (page size).
     * @param limit - Maximum number of records to return
     */
    limit(limit: number): this {
        this._limit = limit;
        return this;
    }

    /**
     * Set the OFFSET (start position).
     * @param offset - Number of records to skip
     */
    offset(offset: number): this {
        this._start = offset;
        return this;
    }

    /**
     * Set field definitions for the query.
     * @param fieldDefs - Array of field definition objects
     */
    fieldDefs(fieldDefs: Record<string, unknown>[]): this {
        this._fieldDefs = fieldDefs;
        return this;
    }

    /**
     * Set the Zod schema for validating returned records.
     * @param schema - A Zod schema object
     */
    withSchema(schema: unknown): this {
        this._recordSchema = schema;
        return this;
    }

    /**
     * Set the Zod schema for validating embedded objects in returned records.
     * @param embedName - The name of the embedded field
     * @param schema - A Zod schema object
     */
    withEmbedSchema(embedName: string, schema: unknown): this {
        this._embedName = embedName;
        this._embedSchema = schema;
        return this;
    }

    /**
     * Set a custom location identifier for debugging/tracing.
     * @param loc - Location identifier string
     */
    location(loc: string): this {
        this._loc = loc;
        return this;
    }

    /**
     * Execute the query and return results.
     * @returns Promise resolving to JimoResponse
     */
    async execute(): Promise<JimoResponse> {
        return await db_store.retrieveRecords(
            this._dbName,
            this._tableName,
            this._fieldNames,
            this._fieldDefs,
            this._loc,
            this._condition,
            this._joins,
            this._orderBy,
            this._recordSchema,
            this._embedName,
            this._embedSchema,
            this._start,
            this._limit
        );
    }

    /**
     * Reset the query builder to initial state for reuse.
     */
    reset(): this {
        this._dbName = '';
        this._tableName = '';
        this._fieldNames = [];
        this._fieldDefs = [];
        this._condition = cond_builder.null();
        this._joins = [];
        this._orderBy = [];
        this._recordSchema = null;
        this._embedName = '';
        this._embedSchema = null;
        this._start = 0;
        this._limit = 100;
        this._loc = 'query_builder';
        return this;
    }
}

/**
 * Query constructor class to create new query builder instances.
 */
class QueryConstructor {
    /**
     * Create a new query builder instance.
     */
    select(...fields: string[]): QueryBuilder {
        const builder = new QueryBuilder();
        return builder.select(...fields);
    }

    /**
     * Create a new query builder starting with FROM clause.
     */
    from(tableName: string): QueryBuilder {
        const builder = new QueryBuilder();
        return builder.from(tableName);
    }
}

// Global instance for easy access
const query_builder = new QueryConstructor();

export {
    query_builder,
    QueryConstructor,
    QueryBuilder
};
