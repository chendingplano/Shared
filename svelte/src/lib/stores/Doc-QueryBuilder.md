# 1. Database Query Builders

This directory contains a set of fluent query builders for constructing type-safe database queries that execute via `DBStore::retrieveRecords()`.

## 1.1 Overview

The query builders provide a Drizzle-ORM-inspired API for building database queries:

- **query_builder** - Main query builder for SELECT operations
- **cond_builder** - Condition builder for WHERE clauses
- **join_builder** - Join builder for table joins
- **update_builder** - Update builder for UPDATE operations
- **delete_builder** - Delete builder for DELETE operations (not implemented yet)

## 1.2 Quick Start

```typescript
// If you are in Shared/svelte
import { query_builder, cond_builder, join_builder } from '$lib/stores';

// If you are in a different project
import { query_builder, cond_builder, join_builder } from '@chendingplano/shared';

// Simple query
const users = await query_builder
    .select('id', 'userame', 'email')
    .from('users')
    .where(cond_builder.filter().condEq('status', 'active'))
    .execute();
```

## 1.3 Query Builder

### 1.3.1 Basic Usage

```typescript
// Select all fields
const results = await query_builder
    .select()
    .from('users')
    .execute();

// Select specific fields
const results = await query_builder
    .select('id', 'name', 'email')
    .from('users')
    .execute();
```

### 1.3.2 WHERE Conditions

```typescript
// Simple condition
const results = await query_builder
    .select()
    .from('users')
    .where(cond_builder.filter().condEq('status', 'active'))
    .execute();

// Multiple conditions with AND
const results = await query_builder
    .select()
    .from('users')
    .where(
        cond_builder.and()
            .condEq('status', 'active')
            .condGt('age', 18)
    )
    .execute();

// Complex nested conditions
const results = await query_builder
    .select()
    .from('users')
    .where(
        cond_builder.or()
            .addCond(
                cond_builder.and()
                    .condEq('status', 'active')
                    .condGt('age', 18)
            )
            .addCond(
                cond_builder.and()
                    .condEq('role', 'admin')
                    .condGt('level', 5)
            )
    )
    .execute();
```

### 1.3.3 JOINs

```typescript
// LEFT JOIN
const results = await query_builder
    .select()
    .from('posts')
    .leftJoin(
        join_builder.from('posts')
            .join('users', 'left_join')
            .on('posts.author_id', 'users.id', '=', 'string')
            .select('username', 'email', 'avatar')
            .embedAs('author')
            .build()
    )
    .execute();

// Multiple JOINs
const results = await query_builder
    .select()
    .from('posts')
    .leftJoin(
        join_builder.from('posts')
            .join('users', 'left_join')
            .on('posts.author_id', 'users.id', '=', 'string')
            .select('username', 'email')
            .embedAs('author')
            .build()
    )
    .leftJoin(
        join_builder.from('posts')
            .join('categories', 'left_join')
            .on('posts.category_id', 'categories.id', '=', 'string')
            .select('category_name', 'slug')
            .embedAs('category')
            .build()
    )
    .execute();
```

### 1.3.4 Ordering

```typescript
// Single ORDER BY
const results = await query_builder
    .select()
    .from('users')
    .orderBy('created_at', false) // DESC
    .execute();

// Multiple ORDER BY
const results = await query_builder
    .select()
    .from('posts')
    .orderBy('view_count', false)  // DESC
    .orderBy('created_at', false)  // DESC
    .execute();
```

### 1.3.5 Pagination

```typescript
// Using limit and offset
const results = await query_builder
    .select()
    .from('articles')
    .orderBy('published_at', false)
    .offset(20)
    .limit(10)
    .execute();

// Helper function for pagination
function paginate(page: number, pageSize: number) {
    return query_builder
        .select()
        .from('articles')
        .offset((page - 1) * pageSize)
        .limit(pageSize);
}
```

### 1.3.6 Schema Validation

```typescript
import { z } from 'zod';

const UserSchema = z.object({
    id: z.string(),
    username: z.string(),
    email: z.string().email(),
    status: z.enum(['active', 'inactive'])
});

const results = await query_builder
    .select()
    .from('users')
    .withSchema(UserSchema)
    .execute();
```

### 1.3.7 Embedded Schema Validation

```typescript
const PostSchema = z.object({
    id: z.string(),
    title: z.string(),
    content: z.string()
});

const AuthorSchema = z.object({
    username: z.string(),
    email: z.string().email()
});

const results = await query_builder
    .select()
    .from('posts')
    .leftJoin(
        join_builder.from('posts')
            .join('users', 'left_join')
            .on('posts.author_id', 'users.id', '=', 'string')
            .select('username', 'email')
            .embedAs('author')
            .build()
    )
    .withSchema(PostSchema)
    .withEmbedSchema('author', AuthorSchema)
    .execute();
```

## 1.4 Condition Builder

The condition builder supports various operators:

```typescript
// Equality
cond_builder.filter().condEq('status', 'active', 'string')

// Inequality
cond_builder.filter().condNe('role', 'guest', 'string')

// Greater than
cond_builder.filter().condGt('age', 18, 'number')

// Greater than or equal
cond_builder.filter().condGte('score', 100, 'number')

// Less than
cond_builder.filter().condLt('price', 50, 'number')

// Less than or equal
cond_builder.filter().condLte('quantity', 10, 'number')

// Contains
cond_builder.filter().condContains('name', 'John', 'string')

// Prefix (starts with)
cond_builder.filter().condPrefix('email', 'admin', 'string')
```

### 1.4.1 String-based Condition Parser

You can also use string-based conditions:

```typescript
import { parseCondition } from '$lib/stores';

const condition = parseCondition("status = 'active' AND age > 18");

const results = await query_builder
    .select()
    .from('users')
    .where(condition)
    .execute();
```

Supported syntax:
- Operators: `=`, `==`, `!=`, `>`, `>=`, `<`, `<=`, `CONTAINS`, `PREFIX`
- Logical: `AND`, `&&`, `OR`, `||`
- Grouping: `( )`

Examples:
```typescript
"status = 'active' AND age > 18"
"name CONTAINS 'John' OR email PREFIX 'admin'"
"(role = 'admin' AND level >= 5) OR status = 'superuser'"
```

## 1.5 Join Builder

```typescript
// Basic join
const joinDef = join_builder
    .from('posts')
    .join('users', 'left_join')
    .on('posts.author_id', 'users.id', '=', 'string')
    .select('username', 'email', 'avatar')
    .embedAs('author')
    .build();

// Join with multiple ON conditions
const joinDef = join_builder
    .from('orders')
    .join('customers', 'left_join')
    .on('orders.customer_id', 'customers.id', '=', 'string')
    .on('orders.status', 'customers.default_status', '=', 'string')
    .select('name', 'email')
    .embedAs('customer')
    .build();
```

Join types:
- `'left_join'` - LEFT JOIN
- `'right_join'` - RIGHT JOIN
- `'inner_join'` - INNER JOIN
- `'join'` - Generic JOIN (defaults to INNER)

## 1.6 Update Builder

```typescript
import { update_builder } from '$lib/stores';

// Single field update
const updates = update_builder
    .start()
    .modify('status', 'active', 'string')
    .build();

// Multiple field updates
const updates = update_builder
    .start()
    .modify('order_num', 12345, 'int')
    .modify('remarks', 'Updated order', 'string')
    .modify('updated_at', new Date().toISOString(), 'timestamp')
    .build();
```

## 1.7 Complete Examples

### 1.7.1 Example 1: User Profile Query

```typescript
const UserSchema = z.object({
    id: z.string(),
    username: z.string(),
    email: z.string().email()
});

const ProfileSchema = z.object({
    bio: z.string(),
    avatar: z.string().optional()
});

const results = await query_builder
    .select()
    .from('users')
    .leftJoin(
        join_builder.from('users')
            .join('profiles', 'left_join')
            .on('users.id', 'profiles.user_id', '=', 'string')
            .select('bio', 'avatar', 'location')
            .embedAs('profile')
            .build()
    )
    .where(cond_builder.filter().condEq('users.status', 'active'))
    .withSchema(UserSchema)
    .withEmbedSchema('profile', ProfileSchema)
    .limit(50)
    .execute();
```

### 1.7.2 Example 2: Blog Posts with Author and Category

```typescript
const results = await query_builder
    .select()
    .from('posts')
    .leftJoin(
        join_builder.from('posts')
            .join('users', 'left_join')
            .on('posts.author_id', 'users.id', '=', 'string')
            .select('username', 'email', 'avatar')
            .embedAs('author')
            .build()
    )
    .leftJoin(
        join_builder.from('posts')
            .join('categories', 'left_join')
            .on('posts.category_id', 'categories.id', '=', 'string')
            .select('category_name', 'slug')
            .embedAs('category')
            .build()
    )
    .where(
        cond_builder.and()
            .condEq('posts.published', true, 'boolean')
            .condGt('posts.view_count', 100, 'number')
    )
    .orderBy('posts.view_count', false)
    .orderBy('posts.created_at', false)
    .limit(20)
    .execute();
```

### 1.7.3 Example 3: Search with Pagination

```typescript
async function searchPosts(searchTerm: string, page: number = 1, pageSize: number = 10) {
    const offset = (page - 1) * pageSize;

    return await query_builder
        .select()
        .from('posts')
        .where(
            cond_builder.or()
                .condContains('title', searchTerm, 'string')
                .condContains('content', searchTerm, 'string')
        )
        .orderBy('created_at', false)
        .offset(offset)
        .limit(pageSize)
        .execute();
}
```

## 1.8 API Reference

### 1.8.1 QueryBuilder Methods

| Method | Parameters | Description |
|--------|------------|-------------|
| `select(...fields)` | `fields: string[]` | Select specific fields (empty = all fields) |
| `database(name)` | `name: string` | Set database name |
| `from(table)` | `table: string` | Set table name |
| `where(condition)` | `condition: CondDef` | Add WHERE clause |
| `leftJoin(join)` | `join: JoinDef` | Add LEFT JOIN |
| `rightJoin(join)` | `join: JoinDef` | Add RIGHT JOIN |
| `innerJoin(join)` | `join: JoinDef` | Add INNER JOIN |
| `join(join)` | `join: JoinDef` | Add generic JOIN |
| `orderBy(field, asc, type)` | `field: string, asc: boolean, type: string` | Add ORDER BY |
| `limit(n)` | `n: number` | Set LIMIT |
| `offset(n)` | `n: number` | Set OFFSET |
| `withSchema(schema)` | `schema: ZodType` | Set validation schema |
| `withEmbedSchema(name, schema)` | `name: string, schema: ZodType` | Set embedded validation schema |
| `location(loc)` | `loc: string` | Set location identifier |
| `execute()` | - | Execute query and return results |
| `reset()` | - | Reset builder to initial state |

### 1.8.2 Condition Builder Methods

| Method | Parameters | Description |
|--------|------------|-------------|
| `condEq(field, value, type)` | Field equals value |
| `condNe(field, value, type)` | Field not equals value |
| `condGt(field, value, type)` | Field greater than value |
| `condGte(field, value, type)` | Field greater than or equal |
| `condLt(field, value, type)` | Field less than value |
| `condLte(field, value, type)` | Field less than or equal |
| `condContains(field, value, type)` | Field contains value |
| `condPrefix(field, value, type)` | Field starts with value |
| `addCond(condition)` | Add nested condition |

## 1.9 See Also

- [query_builder_examples.ts](./query_builder_examples.ts) - More usage examples
- [cond_builder.ts](./cond_builder.ts) - Condition builder implementation
- [join_builder.ts](./join_builder.ts) - Join builder implementation
- [update_builder.ts](./update_builder.ts) - Update builder implementation



# 2. Query Builder Architecture

## 2.1 Overview

The query builder provides a fluent, type-safe interface for building database queries that execute through `DBStore::retrieveRecords()`.

## 2.2. Component Diagram

```
┌────────────────────────────────────────────────────────────-─┐
│                     Application Layer                        │
│                                                              │
│  import { query_builder, cond_builder, join_builder }        │
│          from '$lib/stores';                                 │
│                                                              │
│  const results = await query_builder                         │
│    .select()                                                 │
│    .from('users')                                            │
│    .where(cond_builder.filter().condEq('status', 'active'))  │
│    .execute();                                               │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────────-─┐
│                    Query Builder Layer                       │
│                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐  │
│  │ QueryBuilder    │  │ ConditionBuilder│  │ JoinBuilder  │  │
│  │                 │  │                 │  │              │  │
│  │ - select()      │  │ - condEq()      │  │ - from()     │  │
│  │ - from()        │  │ - condGt()      │  │ - join()     │  │
│  │ - where()       │  │ - condLt()      │  │ - on()       │  │
│  │ - leftJoin()    │  │ - and()         │  │ - select()   │  │
│  │ - orderBy()     │  │ - or()          │  │ - embedAs()  │  │
│  │ - limit()       │  │ - build()       │  │ - build()    │  │
│  │ - execute()     │  │                 │  │              │  │
│  └─────────────────┘  └─────────────────┘  └──────────────┘  │
│                                                              │
│  ┌─────────────────┐  ┌─────────────────┐                    │
│  │ UpdateBuilder   │  │ DeleteBuilder   │                    │
│  │                 │  │ (not impl yet)  │                    │
│  │ - start()       │  │                 │                    │
│  │ - modify()      │  │                 │                    │
│  │ - build()       │  │                 │                    │
│  └─────────────────┘  └─────────────────┘                    │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        │ execute()
                        ▼
┌───────────────────────────────────────────────────────────-──┐
│                       DBStore Layer                          │
│                                                              │
│  class DBStore {                                             │
│    async retrieveRecords(                                    │
│      db_name: string,                                        │
│      table_name: string,                                     │
│      field_names: string[],                                  │
│      field_defs: Record<string, unknown>[],                  │
│      loc: string,                                            │
│      conds: CondDef,              ◄─── From ConditionBuilder │
│      join_def: JoinDef[],         ◄─── From JoinBuilder      │
│      orderby_def: OrderbyDef[],   ◄─── From QueryBuilder     │
│      record_schema: unknown,      ◄─── Zod Schema            │
│      embed_name: string,                                     │
│      embed_schema: unknown,       ◄─── Zod Schema            │
│      start: number,                                          │
│      num_records: number                                     │
│    ): Promise<JimoResponse>                                  │
│  }                                                           │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        │ HTTP POST
                        ▼
┌───────────────────────────────────────────────────────────-──┐
│                      Backend API                             │
│                                                              │
│  POST /shared_api/v1/jimo_req                                │
│                                                              │
│  Request: QueryRequest {                                     │
│    request_type: 'query',                                    │
│    db_name: string,                                          │
│    table_name: string,                                       │
│    field_names: string[],                                    │
│    condition: CondDef,                                       │
│    join_def: JoinDef[],                                      │
│    orderby_def: OrderbyDef[],                                │
│    start: number,                                            │
│    page_size: number                                         │
│  }                                                           │
│                                                              │
│  Response: JimoResponse {                                    │
│    status: boolean,                                          │
│    error_msg: string,                                        │
│    num_records: number,                                      │
│    results: JsonObjectOrArray                                │
│  }                                                           │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────────-───┐
│                        Database                              │
│                                                              │
│  PostgreSQL / MySQL / SQLite                                 │
│  (Abstracted by backend)                                     │
└─────────────────────────────────────────────────────────-────┘
```

## 2.3 Data Flow

### 2.3.1. Query Construction Phase

```typescript
query_builder
  .select('id', 'name')
  .from('users')
  .where(cond_builder.filter().condEq('status', 'active'))
```

Internal state being built:
```typescript
{
  _tableName: 'users',
  _fieldNames: ['id', 'name'],
  _condition: {
    type: 'atomic',
    field_name: 'status',
    opr: '=',
    value: 'active',
    data_type: 'string'
  },
  _joins: [],
  _orderBy: [],
  _start: 0,
  _limit: 100
}
```

### 2.3.2. Execution Phase

```typescript
.execute()  // Calls db_store.retrieveRecords()
```

Converts to:
```typescript
db_store.retrieveRecords(
  '',              // db_name
  'users',         // table_name
  ['id', 'name'],  // field_names
  [],              // field_defs
  'query_builder', // loc
  { type: 'atomic', field_name: 'status', ... }, // conds
  [],              // join_def
  [],              // orderby_def
  null,            // record_schema
  '',              // embed_name
  null,            // embed_schema
  0,               // start
  100              // num_records
)
```

### 2.3.3. HTTP Request Phase

```typescript
fetch("/shared_api/v1/jimo_req", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    request_type: 'query',
    table_name: 'users',
    field_names: ['id', 'name'],
    condition: { type: 'atomic', ... },
    // ...
  })
})
```

### 2.3.4. Response Processing Phase

```typescript
{
  status: true,
  error_msg: '',
  num_records: 42,
  results: [
    { id: '1', name: 'Alice' },
    { id: '2', name: 'Bob' },
    // ...
  ]
}
```

### 2.3.5. Schema Validation Phase (if withSchema used)

```typescript
const UserSchema = z.object({
  id: z.string(),
  name: z.string()
});

// Each record validated against schema
for (const record of results) {
  const result = UserSchema.safeParse(record);
  if (result.success) {
    valid_records.push(record);
  }
}
```

## 2.4 Type Definitions Flow

```
┌─────────────────────┐
│   CommonTypes.ts    │
│                     │
│  - CondDef          │◄─── Used by ConditionBuilder
│  - AtomicCondition  │
│  - GroupCondition   │
│  - NullCondition    │
│                     │
│  - JoinDef          │◄─── Used by JoinBuilder
│  - OnClauseDef      │
│                     │
│  - OrderbyDef       │◄─── Used by QueryBuilder
│                     │
│  - UpdateDef        │◄─── Used by UpdateBuilder
│                     │
│  - QueryRequest     │◄─── Sent to backend
│  - JimoResponse     │◄─── Received from backend
└─────────────────────┘
```

## 2.5 Builder Instances

```typescript
// Global singleton instances for convenience
const query_builder = new QueryConstructor();
const cond_builder = new ConditionConstructor();
const join_builder = new JoinConstructor();
const update_builder = new UpdateConstructor();
```

Each method call returns `this` for fluent chaining:
```typescript
class QueryBuilder {
  select(...fields: string[]): this {
    this._fieldNames = fields;
    return this;  // ◄─── Enables chaining
  }

  from(table: string): this {
    this._tableName = table;
    return this;  // ◄─── Enables chaining
  }
}
```

## 2.6 Condition Builder Tree Structure

Example condition:
```typescript
cond_builder.or()
  .addCond(
    cond_builder.and()
      .condEq('status', 'active')
      .condGt('age', 18)
  )
  .condEq('role', 'admin')
```

Builds this tree:
```
OR
├── AND
│   ├── ATOMIC (status = 'active')
│   └── ATOMIC (age > 18)
└── ATOMIC (role = 'admin')
```

Serialized as:
```typescript
{
  type: 'or',
  conditions: [
    {
      type: 'and',
      conditions: [
        { type: 'atomic', field_name: 'status', opr: '=', value: 'active' },
        { type: 'atomic', field_name: 'age', opr: '>', value: 18 }
      ]
    },
    { type: 'atomic', field_name: 'role', opr: '=', value: 'admin' }
  ]
}
```

## 2.7 Join Builder Structure

Example join:
```typescript
join_builder.from('posts')
  .join('users', 'left_join')
  .on('posts.author_id', 'users.id', '=', 'string')
  .select('username', 'email')
  .embedAs('author')
  .build()
```

Creates:
```typescript
{
  from_table_name: 'posts',
  joined_table_name: 'users',
  join_type: 'left_join',
  on_clause: [
    {
      source_field_name: 'posts.author_id',
      joined_field_name: 'users.id',
      join_opr: '=',
      data_type: 'string'
    }
  ],
  selected_fields: ['username', 'email'],
  embed_name: 'author',
  from_field_defs: [],
  joined_field_defs: []
}
```

Result structure:
```typescript
{
  id: '1',
  title: 'My Post',
  author: {           // ◄─── Embedded object
    username: 'alice',
    email: 'alice@example.com'
  }
}
```

## 2.8 Extension Points

### 2.8.1 Add New Condition Operators

```typescript
// In ConditionBuilder class
condBetween(field: string, min: any, max: any, type: string): this {
  this.conditions.push({
    type: 'atomic',
    field_name: field,
    opr: 'between',
    value: [min, max],
    data_type: type
  });
  return this;
}
```

### 2.8.2 Add New Query Methods

```typescript
// In QueryBuilder class
distinct(): this {
  this._distinct = true;
  return this;
}
```

### 2.8.3 Add Custom Validators

```typescript
// In QueryBuilder class
validate(validator: (query: QueryBuilder) => boolean): this {
  if (!validator(this)) {
    throw new Error('Query validation failed');
  }
  return this;
}
```

## 2.9 Performance Considerations

1. **Builder Reuse**: Use `.reset()` to reuse builder instances
2. **Schema Validation**: Only use when necessary (adds overhead)
3. **Field Selection**: Select only needed fields to reduce payload
4. **Pagination**: Always use `.limit()` to prevent large result sets
5. **Join Optimization**: Minimize number of joins when possible

## 2.10 Error Handling

```typescript
const result = await query_builder
  .select()
  .from('users')
  .execute();

if (!result.status) {
  console.error(`Query failed: ${result.error_msg}`);
  console.error(`Error code: ${result.error_code}`);
  console.error(`Location: ${result.loc}`);
}
```

## 2.11 Debugging

Use `.location()` to add identifiers:
```typescript
query_builder
  .select()
  .from('users')
  .location('UserList.loadUsers')  // ◄─── Appears in error messages
  .execute();
```

# 3 Query Builder vs Drizzle ORM - Comparison Guide

This document shows how the query builder API compares to Drizzle ORM, highlighting similarities and differences.

## 3.1 Basic Queries

### 3.1.1 Drizzle ORM
```typescript
import { drizzle } from 'drizzle-orm/postgres-js';
import { users } from './schema';

const db = drizzle(postgres());

// Select all
const result = await db.select().from(users);

// Select specific fields
const result = await db
  .select({
    id: users.id,
    name: users.name,
    email: users.email
  })
  .from(users);
```

### 3.1.2 Our Query Builder
```typescript
import { query_builder } from '$lib/stores';

// Select all
const result = await query_builder
  .select()
  .from('users')
  .execute();

// Select specific fields
const result = await query_builder
  .select('id', 'name', 'email')
  .from('users')
  .execute();
```

## 3.2 WHERE Clauses

### 3.2.1 Drizzle ORM
```typescript
import { eq, and, or, gt, lt } from 'drizzle-orm';

// Simple condition
const result = await db
  .select()
  .from(users)
  .where(eq(users.status, 'active'));

// Multiple conditions (AND)
const result = await db
  .select()
  .from(users)
  .where(and(
    eq(users.status, 'active'),
    gt(users.age, 18)
  ));

// Complex conditions (OR with nested AND)
const result = await db
  .select()
  .from(users)
  .where(or(
    and(
      eq(users.status, 'active'),
      gt(users.age, 18)
    ),
    eq(users.role, 'admin')
  ));
```

### 3.2.2 Our Query Builder
```typescript
import { query_builder, cond_builder } from '$lib/stores';

// Simple condition
const result = await query_builder
  .select()
  .from('users')
  .where(cond_builder.filter().condEq('status', 'active'))
  .execute();

// Multiple conditions (AND)
const result = await query_builder
  .select()
  .from('users')
  .where(
    cond_builder.and()
      .condEq('status', 'active', 'string')
      .condGt('age', 18, 'number')
  )
  .execute();

// Complex conditions (OR with nested AND)
const result = await query_builder
  .select()
  .from('users')
  .where(
    cond_builder.or()
      .addCond(
        cond_builder.and()
          .condEq('status', 'active', 'string')
          .condGt('age', 18, 'number')
      )
      .condEq('role', 'admin', 'string')
  )
  .execute();
```

## 3.3 JOINs

### 3.3.1 Drizzle ORM
```typescript
// LEFT JOIN
const result = await db
  .select({
    id: posts.id,
    title: posts.title,
    authorName: users.name,
    authorEmail: users.email
  })
  .from(posts)
  .leftJoin(users, eq(posts.authorId, users.id));

// Multiple JOINs
const result = await db
  .select()
  .from(posts)
  .leftJoin(users, eq(posts.authorId, users.id))
  .leftJoin(categories, eq(posts.categoryId, categories.id));
```

### 3.3.2 Our Query Builder
```typescript
import { query_builder, join_builder } from '$lib/stores';

// LEFT JOIN
const result = await query_builder
  .select()
  .from('posts')
  .leftJoin(
    join_builder.from('posts')
      .join('users', 'left_join')
      .on('posts.author_id', 'users.id', '=', 'string')
      .select('name', 'email')
      .embedAs('author')
      .build()
  )
  .execute();

// Multiple JOINs
const result = await query_builder
  .select()
  .from('posts')
  .leftJoin(
    join_builder.from('posts')
      .join('users', 'left_join')
      .on('posts.author_id', 'users.id', '=', 'string')
      .select('name', 'email')
      .embedAs('author')
      .build()
  )
  .leftJoin(
    join_builder.from('posts')
      .join('categories', 'left_join')
      .on('posts.category_id', 'categories.id', '=', 'string')
      .select('category_name', 'slug')
      .embedAs('category')
      .build()
  )
  .execute();
```

## 3.4 ORDER BY

### 3.4.1 Drizzle ORM
```typescript
import { asc, desc } from 'drizzle-orm';

// Single ORDER BY
const result = await db
  .select()
  .from(users)
  .orderBy(desc(users.createdAt));

// Multiple ORDER BY
const result = await db
  .select()
  .from(posts)
  .orderBy(desc(posts.viewCount), desc(posts.createdAt));
```

### 3.4.2 Our Query Builder
```typescript
// Single ORDER BY
const result = await query_builder
  .select()
  .from('users')
  .orderBy('created_at', false) // false = DESC
  .execute();

// Multiple ORDER BY
const result = await query_builder
  .select()
  .from('posts')
  .orderBy('view_count', false)  // DESC
  .orderBy('created_at', false)  // DESC
  .execute();
```

## 3.5 Pagination

### 3.5.1 Drizzle ORM
```typescript
const result = await db
  .select()
  .from(users)
  .limit(10)
  .offset(20);
```

### 3.5.2 Our Query Builder
```typescript
const result = await query_builder
  .select()
  .from('users')
  .limit(10)
  .offset(20)
  .execute();
```

## 3.6 Key Differences

### 3.6.1 Schema Definition

**Drizzle:** Requires explicit schema definition
```typescript
import { pgTable, serial, text, varchar } from 'drizzle-orm/pg-core';

export const users = pgTable('users', {
  id: serial('id').primaryKey(),
  name: text('name'),
  email: varchar('email', { length: 256 })
});
```

**Our Builder:** No schema definition required, works with dynamic table/field names
```typescript
// No schema definition needed - just use table and field names directly
query_builder.select().from('users').execute();
```

### 3.6.2 Type Safety

**Drizzle:** Full TypeScript type inference from schema
```typescript
// TypeScript knows the exact shape of the result
const result: { id: number; name: string }[] = await db
  .select({ id: users.id, name: users.name })
  .from(users);
```

**Our Builder:** Uses Zod schemas for runtime validation
```typescript
import { z } from 'zod';

const UserSchema = z.object({
  id: z.string(),
  name: z.string()
});

const result = await query_builder
  .select('id', 'name')
  .from('users')
  .withSchema(UserSchema)  // Runtime validation
  .execute();
```

### 3.6.3 Data Types

**Drizzle:** Types inferred from schema
```typescript
// Type automatically known from schema
where(eq(users.age, 18))  // age is number
```

**Our Builder:** Explicit data type specification
```typescript
// Must specify data type
where(cond_builder.filter().condEq('age', 18, 'number'))
```

### 3.6.4 Embedded Objects

**Drizzle:** Flat result structure
```typescript
const result = await db
  .select({
    postId: posts.id,
    postTitle: posts.title,
    authorName: users.name,
    authorEmail: users.email
  })
  .from(posts)
  .leftJoin(users, eq(posts.authorId, users.id));

// Result: { postId, postTitle, authorName, authorEmail }
```

**Our Builder:** Nested embedded objects
```typescript
const result = await query_builder
  .select()
  .from('posts')
  .leftJoin(
    join_builder.from('posts')
      .join('users', 'left_join')
      .on('posts.author_id', 'users.id', '=', 'string')
      .select('name', 'email')
      .embedAs('author')  // Creates nested object
      .build()
  )
  .execute();

// Result: { id, title, author: { name, email } }
```

### 3.6.5 Query Execution

**Drizzle:** Direct database connection
```typescript
const db = drizzle(postgres('connection-string'));
const result = await db.select().from(users);
```

**Our Builder:** Goes through DBStore abstraction
```typescript
const result = await query_builder
  .select()
  .from('users')
  .execute();  // Calls db_store.retrieveRecords internally
```

## 3.6 Advantages of Our Query Builder

1. **No Schema Required**: Works with dynamic tables without predefined schemas
2. **Embedded Objects**: Automatic nesting of joined data with `embedAs()`
3. **Runtime Validation**: Zod schemas for flexible runtime type checking
4. **String Conditions**: Support for parsing string-based conditions
5. **Backend Abstraction**: Works through DBStore, allowing backend switching

## 3.7 Advantages of Drizzle ORM

1. **Type Safety**: Full compile-time type checking
2. **Type Inference**: Automatic TypeScript types from schema
3. **Migrations**: Built-in schema migration tools
4. **Relational Queries**: Sophisticated relation handling
5. **Direct Database**: Direct connection to PostgreSQL/MySQL/SQLite

## 3.8 When to Use Each

### 3.8.1 Use Drizzle ORM when:
- You have a fixed schema
- You want compile-time type safety
- You need database migrations
- You're building a traditional web application
- You control the database structure

### 3.8.2 Use Our Query Builder when:
- You have dynamic schemas
- You need flexible runtime validation
- You want embedded/nested result objects
- You're working through an API abstraction layer
- You need to work with user-defined tables/fields

## 3.9 Migration Guide

If you want Drizzle-like syntax but using our builders:

```typescript
// Instead of Drizzle's:
// import { eq, and } from 'drizzle-orm';
// db.select().from(users).where(and(eq(users.status, 'active'), gt(users.age, 18)))

// Use:
import { query_builder, cond_builder } from '$lib/stores';

query_builder
  .select()
  .from('users')
  .where(
    cond_builder.and()
      .condEq('status', 'active', 'string')
      .condGt('age', 18, 'number')
  )
  .execute();
```

## 3.10 Conclusion

Our query builder takes inspiration from Drizzle's fluent API design while adapting it to work with the existing DBStore architecture. It provides a similar developer experience with the flexibility needed for dynamic schemas and embedded object structures.

# 4. Query Builder - Quick Reference Card

## 4.1 Import

```typescript
import { query_builder, cond_builder, join_builder } from '$lib/stores';
```

## 4.2 Basic Query

```typescript
await query_builder
  .select('field1', 'field2')  // or .select() for all fields
  .from('table_name')
  .execute();
```

## 4.3 WHERE Conditions

```typescript
// Single condition
.where(cond_builder.filter().condEq('field', 'value'))

// AND conditions
.where(
  cond_builder.and()
    .condEq('status', 'active', 'string')
    .condGt('age', 18, 'number')
)

// OR conditions
.where(
  cond_builder.or()
    .condEq('role', 'admin', 'string')
    .condEq('role', 'moderator', 'string')
)

// Nested conditions
.where(
  cond_builder.or()
    .addCond(
      cond_builder.and()
        .condEq('status', 'active')
        .condGt('age', 18)
    )
    .condEq('role', 'admin')
)
```

## 4.4 Condition Operators

```typescript
.condEq('field', value, 'type')       // equals =
.condNe('field', value, 'type')       // not equal <>
.condGt('field', value, 'type')       // greater than >
.condGte('field', value, 'type')      // greater than or equal >=
.condLt('field', value, 'type')       // less than <
.condLte('field', value, 'type')      // less than or equal <=
.condContains('field', value, 'type') // contains (LIKE %value%)
.condPrefix('field', value, 'type')   // starts with (LIKE value%)
```

## 4.5 Data Types

Common values: `'string'`, `'number'`, `'boolean'`, `'timestamp'`, `'int'`

## 4.6 JOINs

```typescript
// LEFT JOIN
.leftJoin(
  join_builder.from('source_table')
    .join('joined_table', 'left_join')
    .on('source.field', 'joined.field', '=', 'string')
    .select('field1', 'field2')
    .embedAs('embed_name')
    .build()
)

// Other JOIN types
.rightJoin(...)   // RIGHT JOIN
.innerJoin(...)   // INNER JOIN
.join(...)        // Generic JOIN (defaults to INNER)
```

## 4.7 ORDER BY

```typescript
// Single order
.orderBy('field_name', true)   // ASC
.orderBy('field_name', false)  // DESC

// Multiple orders
.orderBy('field1', false)
.orderBy('field2', true)
```

## 4.8 Pagination

```typescript
.limit(10)      // LIMIT 10
.offset(20)     // OFFSET 20

// Page-based
const page = 2;
const pageSize = 10;
.offset((page - 1) * pageSize)
.limit(pageSize)
```

## 4.9 Schema Validation

```typescript
import { z } from 'zod';

const Schema = z.object({
  id: z.string(),
  name: z.string()
});

.withSchema(Schema)
```

## 4.10 Embedded Schema

```typescript
const EmbedSchema = z.object({
  field: z.string()
});

.withEmbedSchema('embed_name', EmbedSchema)
```

## 4.11 Complete Example

```typescript
const result = await query_builder
  .select()
  .from('posts')
  .leftJoin(
    join_builder.from('posts')
      .join('users', 'left_join')
      .on('posts.author_id', 'users.id', '=', 'string')
      .select('username', 'email')
      .embedAs('author')
      .build()
  )
  .where(
    cond_builder.and()
      .condEq('posts.published', true, 'boolean')
      .condGt('posts.views', 100, 'number')
  )
  .orderBy('posts.created_at', false)
  .limit(20)
  .offset(0)
  .execute();
```

## 4.12 String-based Conditions

```typescript
import { parseCondition } from '$lib/stores';

const cond = parseCondition("status = 'active' AND age > 18");

.where(cond)
```

## 4.13 Method Chaining Order

The typical order (all optional except `.execute()`):

1. `.select()` or `.from()`
2. `.database()` (if needed)
3. `.where()`
4. `.leftJoin()` / `.rightJoin()` / `.innerJoin()`
5. `.orderBy()`
6. `.limit()` / `.offset()`
7. `.withSchema()` / `.withEmbedSchema()`
8. `.location()` (for debugging)
9. `.execute()` ← **Required**

## 4.14 Common Patterns

### 4.14.1 Search
```typescript
.where(
  cond_builder.or()
    .condContains('title', searchTerm)
    .condContains('content', searchTerm)
)
```

### 4.14.2 Date Range
```typescript
.where(
  cond_builder.and()
    .condGte('created_at', startDate, 'timestamp')
    .condLte('created_at', endDate, 'timestamp')
)
```

### 4.14.3 Active Records Only
```typescript
.where(cond_builder.filter().condEq('status', 'active'))
```

### 4.14.4 Latest First
```typescript
.orderBy('created_at', false)  // DESC
```

### 4.14.5 Exclude Deleted
```typescript
.where(cond_builder.filter().condNe('deleted', true, 'boolean'))
```

## 4.5 Return Type

```typescript
interface JimoResponse {
  status: boolean;
  error_msg: string;
  error_code: number;
  req_id?: string;
  result_type: string;
  table_name?: string;
  num_records: number;
  results: any;  // Array or object based on result_type
  loc?: string;
}
```

## 4.6 Tips

- Use `.select()` without arguments to get all fields
- Chain multiple `.orderBy()` calls for multi-column sorting
- Use `embedAs()` to nest joined data as objects
- Specify data types for proper backend handling
- Use `.location()` to add debugging identifiers
- Call `.reset()` to reuse a query builder instance

# 5. Query Builder - Implementation Summary

## 5.1 Created: 2026-01-01

This document summarizes the query builder implementation that was added to complement the existing condition, join, and update builders.

## 5.2 Files Created

### 5.2.1 Core Implementation
1. **query_builder.ts** - Main query builder implementation
   - `QueryBuilder` class with fluent API
   - `QueryConstructor` for creating instances
   - Global `query_builder` instance

### 5.2.2 Documentation
2. **README.md** - Comprehensive usage guide
3. **QUICK_REFERENCE.md** - Quick lookup reference
4. **DRIZZLE_COMPARISON.md** - Comparison with Drizzle ORM
5. **ARCHITECTURE.md** - Technical architecture documentation
6. **CHANGELOG.md** - This file

### 5.2.3 Examples & Tests
7. **query_builder_examples.ts** - 12 example usage patterns
8. **query_builder.test.ts** - Unit tests for functionality

### 5.2.4 Integration
9. **index.ts** - Central export point for all builders

## 5.2 Features Implemented

### 5.2.1 Query Builder Methods

#### 5.2.1.1 Selection & Source
- `select(...fields)` - Select specific fields or all fields
- `from(tableName)` - Specify source table
- `database(dbName)` - Specify database name

#### 5.2.1.2 Filtering
- `where(condition)` - Add WHERE clause using CondDef

#### 5.2.1.3 Joins
- `leftJoin(joinDef)` - Add LEFT JOIN
- `rightJoin(joinDef)` - Add RIGHT JOIN
- `innerJoin(joinDef)` - Add INNER JOIN
- `join(joinDef)` - Add generic JOIN

#### 5.2.1.4 Ordering & Pagination
- `orderBy(field, ascending, dataType)` - Add ORDER BY
- `orderByMultiple(orderDefs)` - Add multiple ORDER BY
- `limit(n)` - Set LIMIT
- `offset(n)` - Set OFFSET

#### 5.2.1.5 Validation
- `withSchema(schema)` - Set Zod schema for record validation
- `withEmbedSchema(name, schema)` - Set Zod schema for embedded objects

#### 5.2.1.6 Metadata
- `fieldDefs(defs)` - Set field definitions
- `location(loc)` - Set location identifier for debugging

#### 5.2.1.7 Execution
- `execute()` - Execute query via DBStore::retrieveRecords()
- `reset()` - Reset builder to initial state for reuse

## 5.3 Design Principles

### 5.3.1. Drizzle-Inspired API
The API design takes inspiration from Drizzle ORM's fluent interface:
```typescript
// Similar to Drizzle's style
query_builder
  .select()
  .from('users')
  .where(...)
  .execute();
```

### 5.3.2. Integration with Existing Builders
Works seamlessly with existing builders:
```typescript
.where(cond_builder.and().condEq(...))  // Uses cond_builder
.leftJoin(join_builder.from(...).build())  // Uses join_builder
```

### 5.3.3. Type Safety via Zod
Runtime type validation using Zod schemas:
```typescript
.withSchema(UserSchema)
.withEmbedSchema('profile', ProfileSchema)
```

### 5.3.4. Fluent Method Chaining
All methods return `this` for chainability:
```typescript
query_builder
  .select()
  .from('users')
  .where(...)
  .orderBy(...)
  .limit(10)
  .execute();
```

### 5.3.5. Conversion to DBStore Call
The `.execute()` method converts the builder state to a `db_store.retrieveRecords()` call with all appropriate parameters.

## 5.4 Examples Provided

1. **Basic Query** - Simple SELECT
2. **Query with Conditions** - WHERE clauses
3. **Complex Conditions** - Nested AND/OR
4. **Schema Validation** - With Zod schemas
5. **LEFT JOIN** - Single join with embedding
6. **Multiple JOINs** - Multiple joined tables
7. **Pagination** - Using offset and limit
8. **Search Query** - CONTAINS operator
9. **Date Range** - Between dates
10. **Complex Query** - Everything combined
11. **Drizzle Style** - Drizzle-like syntax
12. **String Conditions** - Using parseCondition()

## 5.5 Integration Points

### 5.5.1 DBStore
Calls `db_store.retrieveRecords()` with:
- Database name
- Table name
- Field names (from SELECT)
- Field definitions
- Location identifier
- Conditions (from WHERE)
- Join definitions (from JOIN methods)
- Order by definitions (from ORDER BY)
- Schemas for validation
- Pagination parameters

### 5.5.2 Condition Builder
Uses `cond_builder` for WHERE clauses:
- `cond_builder.and()` - AND conditions
- `cond_builder.or()` - OR conditions
- `cond_builder.filter()` - Single condition
- `cond_builder.null()` - No condition

### 5.5.3 Join Builder
Uses `join_builder` for JOIN operations:
- `join_builder.from(table).join(other).on(...).build()`
- Returns `JoinDef` objects
- Supports embedding with `embedAs()`

## 5.4 Type Definitions Used

From `CommonTypes.ts`:
- `CondDef` - Condition definitions
- `JoinDef` - Join definitions
- `OrderbyDef` - Order by definitions
- `JimoResponse` - Response type from backend

## 5.5 Advantages Over Direct DBStore Calls

### 5.5.1 Before (Direct DBStore)
```typescript
await db_store.retrieveRecords(
  '',                    // db_name
  'users',              // table_name
  ['id', 'name'],       // field_names
  [],                   // field_defs
  'loc',                // loc
  cond,                 // conds
  [],                   // join_def
  [],                   // orderby_def
  null,                 // record_schema
  '',                   // embed_name
  null,                 // embed_schema
  0,                    // start
  100                   // num_records
);
```

### 5.5.2 After (Query Builder)
```typescript
await query_builder
  .select('id', 'name')
  .from('users')
  .where(cond)
  .execute();
```

Benefits:
- More readable and maintainable
- Self-documenting code
- Type-safe method chaining
- Default values handled automatically
- Similar to familiar ORMs (Drizzle, Prisma, etc.)

## 5.6 Backward Compatibility

- Does not modify existing code
- DBStore remains unchanged
- Existing builders (cond, join, update) unchanged
- Query builder is an optional wrapper

## 5.7 Future Enhancements

Potential additions:
1. **Delete Builder** - Complete delete_builder.ts implementation
2. **Aggregate Functions** - COUNT, SUM, AVG, etc.
3. **GROUP BY** - Grouping support
4. **HAVING** - Post-aggregation filtering
5. **Subqueries** - Nested query support
6. **UNION** - Combine query results
7. **Raw SQL** - Escape hatch for complex queries
8. **Query Explanation** - Debug query structure
9. **Query Builder Plugins** - Extensibility system
10. **TypeScript Codegen** - Generate types from schemas

## 5.8 Testing

Included tests verify:
- Basic query structure
- WHERE clause construction
- JOIN construction
- ORDER BY clauses
- Pagination
- Nested conditions
- Query builder reset
- Multiple joins

Run tests:
```typescript
import { runAllTests } from '$lib/stores/query_builder.test';
runAllTests();
```

## 5.9 Documentation Completeness

✅ Implementation guide (README.md)
✅ Quick reference (QUICK_REFERENCE.md)
✅ Architecture documentation (ARCHITECTURE.md)
✅ Comparison with Drizzle (DRIZZLE_COMPARISON.md)
✅ Usage examples (query_builder_examples.ts)
✅ Unit tests (query_builder.test.ts)
✅ Central exports (index.ts)
✅ This changelog

## 5.10 Developer Experience

### 5.10.1 Import Once, Use Everywhere
```typescript
import { query_builder, cond_builder, join_builder } from '$lib/stores';
```

### 5.10.2 IntelliSense Support
All methods have JSDoc comments for editor autocomplete

### 5.10.3 Error Messages
Location identifiers help trace errors:
```typescript
.location('UserList.loadUsers')
```

## 5.4 Conclusion

The query builder provides a modern, type-safe, fluent API for database queries while maintaining full compatibility with the existing DBStore infrastructure. It follows familiar ORM patterns (especially Drizzle) making it intuitive for developers while adapting to the unique requirements of the DBStore abstraction layer.

# 6 Query Builder - To-Do List

## 6.1 Create Custom Filters
1. Let users create custom query_name
2. Need to support the ternary operator. When the condition is not true, it is 'undefined', which will not generate the condition at all.

Source: https://orm.drizzle.team/docs/guides/conditional-filters-in-query

```typescript
// length less than
const lenlt = (column: AnyColumn, value: number) => {
  return sql`length(${column}) < ${value}`;
};
const searchPosts = async (maxLen = 0, views = 0) => {
  await db
    .select()
    .from(posts)
    .where(
      and(
        maxLen ? lenlt(posts.title, maxLen) : undefined,
        views > 100 ? gt(posts.views, views) : undefined,
      ),
    );
};
await searchPosts(8);
await searchPosts(8, 200);
```

