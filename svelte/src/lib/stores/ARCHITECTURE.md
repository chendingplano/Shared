# Query Builder Architecture

## Overview

The query builder provides a fluent, type-safe interface for building database queries that execute through `DBStore::retrieveRecords()`.

## Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     Application Layer                        │
│                                                               │
│  import { query_builder, cond_builder, join_builder }        │
│          from '$lib/stores';                                 │
│                                                               │
│  const results = await query_builder                         │
│    .select()                                                 │
│    .from('users')                                            │
│    .where(cond_builder.filter().condEq('status', 'active'))  │
│    .execute();                                               │
└───────────────────────┬───────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                    Query Builder Layer                       │
│                                                               │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │ QueryBuilder    │  │ ConditionBuilder│  │ JoinBuilder  │ │
│  │                 │  │                 │  │              │ │
│  │ - select()      │  │ - condEq()      │  │ - from()     │ │
│  │ - from()        │  │ - condGt()      │  │ - join()     │ │
│  │ - where()       │  │ - condLt()      │  │ - on()       │ │
│  │ - leftJoin()    │  │ - and()         │  │ - select()   │ │
│  │ - orderBy()     │  │ - or()          │  │ - embedAs()  │ │
│  │ - limit()       │  │ - build()       │  │ - build()    │ │
│  │ - execute()     │  │                 │  │              │ │
│  └─────────────────┘  └─────────────────┘  └──────────────┘ │
│                                                               │
│  ┌─────────────────┐  ┌─────────────────┐                   │
│  │ UpdateBuilder   │  │ DeleteBuilder   │                   │
│  │                 │  │ (not impl yet)  │                   │
│  │ - start()       │  │                 │                   │
│  │ - modify()      │  │                 │                   │
│  │ - build()       │  │                 │                   │
│  └─────────────────┘  └─────────────────┘                   │
└───────────────────────┬───────────────────────────────────────┘
                        │
                        │ execute()
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                       DBStore Layer                          │
│                                                               │
│  class DBStore {                                             │
│    async retrieveRecords(                                    │
│      db_name: string,                                        │
│      table_name: string,                                     │
│      field_names: string[],                                  │
│      field_defs: Record<string, unknown>[],                  │
│      loc: string,                                            │
│      conds: CondDef,              ◄─── From ConditionBuilder│
│      join_def: JoinDef[],         ◄─── From JoinBuilder     │
│      orderby_def: OrderbyDef[],   ◄─── From QueryBuilder    │
│      record_schema: unknown,      ◄─── Zod Schema           │
│      embed_name: string,                                     │
│      embed_schema: unknown,       ◄─── Zod Schema           │
│      start: number,                                          │
│      num_records: number                                     │
│    ): Promise<JimoResponse>                                  │
│  }                                                           │
└───────────────────────┬───────────────────────────────────────┘
                        │
                        │ HTTP POST
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                      Backend API                             │
│                                                               │
│  POST /shared_api/v1/jimo_req                                │
│                                                               │
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
│                                                               │
│  Response: JimoResponse {                                    │
│    status: boolean,                                          │
│    error_msg: string,                                        │
│    num_records: number,                                      │
│    results: JsonObjectOrArray                                │
│  }                                                           │
└───────────────────────┬───────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                        Database                              │
│                                                               │
│  PostgreSQL / MySQL / SQLite                                 │
│  (Abstracted by backend)                                     │
└─────────────────────────────────────────────────────────────┘
```

## Data Flow

### 1. Query Construction Phase

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

### 2. Execution Phase

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

### 3. HTTP Request Phase

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

### 4. Response Processing Phase

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

### 5. Schema Validation Phase (if withSchema used)

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

## Type Definitions Flow

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

## Builder Instances

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

## Condition Builder Tree Structure

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

## Join Builder Structure

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

## Extension Points

### Add New Condition Operators

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

### Add New Query Methods

```typescript
// In QueryBuilder class
distinct(): this {
  this._distinct = true;
  return this;
}
```

### Add Custom Validators

```typescript
// In QueryBuilder class
validate(validator: (query: QueryBuilder) => boolean): this {
  if (!validator(this)) {
    throw new Error('Query validation failed');
  }
  return this;
}
```

## Performance Considerations

1. **Builder Reuse**: Use `.reset()` to reuse builder instances
2. **Schema Validation**: Only use when necessary (adds overhead)
3. **Field Selection**: Select only needed fields to reduce payload
4. **Pagination**: Always use `.limit()` to prevent large result sets
5. **Join Optimization**: Minimize number of joins when possible

## Error Handling

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

## Debugging

Use `.location()` to add identifiers:
```typescript
query_builder
  .select()
  .from('users')
  .location('UserList.loadUsers')  // ◄─── Appears in error messages
  .execute();
```
