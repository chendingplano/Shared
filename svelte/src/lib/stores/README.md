# Database Query Builders

This directory contains a set of fluent query builders for constructing type-safe database queries that execute via `DBStore::retrieveRecords()`.

## Overview

The query builders provide a Drizzle-ORM-inspired API for building database queries:

- **query_builder** - Main query builder for SELECT operations
- **cond_builder** - Condition builder for WHERE clauses
- **join_builder** - Join builder for table joins
- **update_builder** - Update builder for UPDATE operations
- **delete_builder** - Delete builder for DELETE operations (not implemented yet)

## Quick Start

```typescript
import { query_builder, cond_builder, join_builder } from '$lib/stores';

// Simple query
const users = await query_builder
    .select('id', 'username', 'email')
    .from('users')
    .where(cond_builder.filter().condEq('status', 'active'))
    .execute();
```

## Query Builder

### Basic Usage

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

### WHERE Conditions

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

### JOINs

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

### Ordering

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

### Pagination

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

### Schema Validation

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

### Embedded Schema Validation

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

## Condition Builder

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

### String-based Condition Parser

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

## Join Builder

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

## Update Builder

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

## Complete Examples

### Example 1: User Profile Query

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

### Example 2: Blog Posts with Author and Category

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

### Example 3: Search with Pagination

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

## API Reference

### QueryBuilder Methods

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

### Condition Builder Methods

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

## See Also

- [query_builder_examples.ts](./query_builder_examples.ts) - More usage examples
- [cond_builder.ts](./cond_builder.ts) - Condition builder implementation
- [join_builder.ts](./join_builder.ts) - Join builder implementation
- [update_builder.ts](./update_builder.ts) - Update builder implementation
