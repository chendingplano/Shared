# Query Builder - Quick Reference Card

## Import

```typescript
import { query_builder, cond_builder, join_builder } from '$lib/stores';
```

## Basic Query

```typescript
await query_builder
  .select('field1', 'field2')  // or .select() for all fields
  .from('table_name')
  .execute();
```

## WHERE Conditions

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

## Condition Operators

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

## Data Types

Common values: `'string'`, `'number'`, `'boolean'`, `'timestamp'`, `'int'`

## JOINs

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

## ORDER BY

```typescript
// Single order
.orderBy('field_name', true)   // ASC
.orderBy('field_name', false)  // DESC

// Multiple orders
.orderBy('field1', false)
.orderBy('field2', true)
```

## Pagination

```typescript
.limit(10)      // LIMIT 10
.offset(20)     // OFFSET 20

// Page-based
const page = 2;
const pageSize = 10;
.offset((page - 1) * pageSize)
.limit(pageSize)
```

## Schema Validation

```typescript
import { z } from 'zod';

const Schema = z.object({
  id: z.string(),
  name: z.string()
});

.withSchema(Schema)
```

## Embedded Schema

```typescript
const EmbedSchema = z.object({
  field: z.string()
});

.withEmbedSchema('embed_name', EmbedSchema)
```

## Complete Example

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

## String-based Conditions

```typescript
import { parseCondition } from '$lib/stores';

const cond = parseCondition("status = 'active' AND age > 18");

.where(cond)
```

## Method Chaining Order

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

## Common Patterns

### Search
```typescript
.where(
  cond_builder.or()
    .condContains('title', searchTerm)
    .condContains('content', searchTerm)
)
```

### Date Range
```typescript
.where(
  cond_builder.and()
    .condGte('created_at', startDate, 'timestamp')
    .condLte('created_at', endDate, 'timestamp')
)
```

### Active Records Only
```typescript
.where(cond_builder.filter().condEq('status', 'active'))
```

### Latest First
```typescript
.orderBy('created_at', false)  // DESC
```

### Exclude Deleted
```typescript
.where(cond_builder.filter().condNe('deleted', true, 'boolean'))
```

## Return Type

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

## Tips

- Use `.select()` without arguments to get all fields
- Chain multiple `.orderBy()` calls for multi-column sorting
- Use `embedAs()` to nest joined data as objects
- Specify data types for proper backend handling
- Use `.location()` to add debugging identifiers
- Call `.reset()` to reuse a query builder instance
