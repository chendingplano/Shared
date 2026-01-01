///////////////////////////////////////////////////////
// Query Builder Usage Examples
//
// This file demonstrates various ways to use the query_builder
// to construct database queries that convert to DBStore::retrieveRecords calls.
//
// Created: 2026/01/01 by Chen Ding
///////////////////////////////////////////////////////

import { query_builder } from './query_builder';
import { cond_builder } from './cond_builder';
import { join_builder } from './join_builder';
import { z } from 'zod';

// Example Zod schemas for validation
const UserSchema = z.object({
    id: z.string(),
    username: z.string(),
    email: z.string().email(),
    age: z.number(),
    status: z.enum(['active', 'inactive', 'pending'])
});

const ProfileSchema = z.object({
    bio: z.string(),
    avatar: z.string().url().optional(),
    location: z.string().optional()
});

const PostSchema = z.object({
    id: z.string(),
    title: z.string(),
    content: z.string(),
    author_id: z.string(),
    published: z.boolean(),
    created_at: z.string()
});

const AuthorSchema = z.object({
    username: z.string(),
    email: z.string().email(),
    avatar: z.string().optional()
});

///////////////////////////////////////////////////////
// Example 1: Basic Query
///////////////////////////////////////////////////////
export async function example1_basicQuery() {
    const results = await query_builder
        .select()
        .from('users')
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 2: Query with WHERE Conditions
///////////////////////////////////////////////////////
export async function example2_queryWithConditions() {
    const results = await query_builder
        .select('id', 'username', 'email', 'status')
        .from('users')
        .where(
            cond_builder.and()
                .condEq('status', 'active', 'string')
                .condGt('age', 18, 'number')
        )
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 3: Query with Complex Conditions
///////////////////////////////////////////////////////
export async function example3_complexConditions() {
    const results = await query_builder
        .select()
        .from('users')
        .where(
            cond_builder.or()
                .addCond(
                    cond_builder.and()
                        .condEq('status', 'active', 'string')
                        .condGt('age', 18, 'number')
                )
                .addCond(
                    cond_builder.and()
                        .condEq('role', 'admin', 'string')
                        .condGt('level', 5, 'number')
                )
        )
        .orderBy('username', true, 'string')
        .limit(50)
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 4: Query with Schema Validation
///////////////////////////////////////////////////////
export async function example4_withSchema() {
    const results = await query_builder
        .select()
        .from('users')
        .where(cond_builder.filter().condEq('status', 'active', 'string'))
        .withSchema(UserSchema)
        .orderBy('created_at', false, 'timestamp') // DESC
        .limit(100)
        .execute();

    // Results will be validated against UserSchema
    return results;
}

///////////////////////////////////////////////////////
// Example 5: Query with LEFT JOIN
///////////////////////////////////////////////////////
export async function example5_leftJoin() {
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
        .where(cond_builder.filter().condEq('users.status', 'active', 'string'))
        .withSchema(UserSchema)
        .withEmbedSchema('profile', ProfileSchema)
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 6: Query with Multiple JOINs
///////////////////////////////////////////////////////
export async function example6_multipleJoins() {
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
        .where(cond_builder.filter().condEq('posts.published', true, 'boolean'))
        .orderBy('posts.created_at', false, 'timestamp')
        .limit(20)
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 7: Pagination
///////////////////////////////////////////////////////
export async function example7_pagination(page: number = 1, pageSize: number = 10) {
    const offset = (page - 1) * pageSize;

    const results = await query_builder
        .select()
        .from('articles')
        .where(cond_builder.filter().condEq('status', 'published', 'string'))
        .orderBy('published_at', false, 'timestamp')
        .offset(offset)
        .limit(pageSize)
        .location('example7_pagination')
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 8: Search Query with CONTAINS
///////////////////////////////////////////////////////
export async function example8_searchQuery(searchTerm: string) {
    const results = await query_builder
        .select('id', 'title', 'content', 'author_id')
        .from('posts')
        .where(
            cond_builder.or()
                .condContains('title', searchTerm, 'string')
                .condContains('content', searchTerm, 'string')
        )
        .orderBy('created_at', false, 'timestamp')
        .limit(50)
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 9: Date Range Query
///////////////////////////////////////////////////////
export async function example9_dateRange(startDate: string, endDate: string) {
    const results = await query_builder
        .select()
        .from('orders')
        .where(
            cond_builder.and()
                .condGte('created_at', startDate, 'timestamp')
                .condLte('created_at', endDate, 'timestamp')
                .condEq('status', 'completed', 'string')
        )
        .orderBy('created_at', true, 'timestamp')
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 10: Complex Query with Everything
///////////////////////////////////////////////////////
export async function example10_complexQuery() {
    const results = await query_builder
        .database('my_database')
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
        .where(
            cond_builder.and()
                .condEq('posts.published', true, 'boolean')
                .condGt('posts.view_count', 100, 'number')
                .addCond(
                    cond_builder.or()
                        .condContains('posts.tags', 'tutorial', 'string')
                        .condContains('posts.tags', 'guide', 'string')
                )
        )
        .orderBy('posts.view_count', false, 'number')
        .orderBy('posts.created_at', false, 'timestamp')
        .offset(0)
        .limit(25)
        .withSchema(PostSchema)
        .withEmbedSchema('author', AuthorSchema)
        .location('example10_complexQuery')
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 11: Drizzle-style Chaining
///////////////////////////////////////////////////////
export async function example11_drizzleStyle() {
    // This style mimics Drizzle ORM's fluent API
    const results = await query_builder
        .select('id', 'username', 'email')
        .from('users')
        .where(
            cond_builder.and()
                .condEq('status', 'active', 'string')
                .condNe('role', 'guest', 'string')
        )
        .orderBy('username', true, 'string')
        .limit(100)
        .execute();

    return results;
}

///////////////////////////////////////////////////////
// Example 12: Using String-based Condition Parser
///////////////////////////////////////////////////////
import { parseCondition } from './cond_builder';

export async function example12_stringConditions() {
    // You can also use the string parser for simpler conditions
    const conditionStr = "status = 'active' AND age > 18";
    const condition = parseCondition(conditionStr);

    const results = await query_builder
        .select()
        .from('users')
        .where(condition)
        .execute();

    return results;
}
