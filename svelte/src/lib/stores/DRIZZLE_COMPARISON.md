# Query Builder vs Drizzle ORM - Comparison Guide

This document shows how the query builder API compares to Drizzle ORM, highlighting similarities and differences.

## Basic Queries

### Drizzle ORM
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

### Our Query Builder
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

## WHERE Clauses

### Drizzle ORM
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

### Our Query Builder
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

## JOINs

### Drizzle ORM
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

### Our Query Builder
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

## ORDER BY

### Drizzle ORM
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

### Our Query Builder
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

## Pagination

### Drizzle ORM
```typescript
const result = await db
  .select()
  .from(users)
  .limit(10)
  .offset(20);
```

### Our Query Builder
```typescript
const result = await query_builder
  .select()
  .from('users')
  .limit(10)
  .offset(20)
  .execute();
```

## Key Differences

### 1. Schema Definition

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

### 2. Type Safety

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

### 3. Data Types

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

### 4. Embedded Objects

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

### 5. Query Execution

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

## Advantages of Our Query Builder

1. **No Schema Required**: Works with dynamic tables without predefined schemas
2. **Embedded Objects**: Automatic nesting of joined data with `embedAs()`
3. **Runtime Validation**: Zod schemas for flexible runtime type checking
4. **String Conditions**: Support for parsing string-based conditions
5. **Backend Abstraction**: Works through DBStore, allowing backend switching

## Advantages of Drizzle ORM

1. **Type Safety**: Full compile-time type checking
2. **Type Inference**: Automatic TypeScript types from schema
3. **Migrations**: Built-in schema migration tools
4. **Relational Queries**: Sophisticated relation handling
5. **Direct Database**: Direct connection to PostgreSQL/MySQL/SQLite

## When to Use Each

### Use Drizzle ORM when:
- You have a fixed schema
- You want compile-time type safety
- You need database migrations
- You're building a traditional web application
- You control the database structure

### Use Our Query Builder when:
- You have dynamic schemas
- You need flexible runtime validation
- You want embedded/nested result objects
- You're working through an API abstraction layer
- You need to work with user-defined tables/fields

## Migration Guide

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

## Conclusion

Our query builder takes inspiration from Drizzle's fluent API design while adapting it to work with the existing DBStore architecture. It provides a similar developer experience with the flexibility needed for dynamic schemas and embedded object structures.
