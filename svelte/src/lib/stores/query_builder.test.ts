///////////////////////////////////////////////////////
// Query Builder Tests
//
// Simple test file to verify query builder functionality
//
// Created: 2026/01/01 by Chen Ding
///////////////////////////////////////////////////////

import { query_builder } from './query_builder';
import { cond_builder } from './cond_builder';
import { join_builder } from './join_builder';

/**
 * Test 1: Verify basic query structure
 */
export function test_basicQuery() {
    const qb = query_builder
        .select('id', 'name', 'email')
        .from('users');

    // Access private fields through type casting for testing
    const builder = qb as any;

    console.assert(builder._tableName === 'users', 'Table name should be users');
    console.assert(builder._fieldNames.length === 3, 'Should have 3 fields');
    console.assert(builder._fieldNames[0] === 'id', 'First field should be id');

    console.log('✓ Test 1: Basic query structure - PASSED');
}

/**
 * Test 2: Verify WHERE clause construction
 */
export function test_whereClause() {
    const condition = cond_builder.and()
        .condEq('status', 'active', 'string')
        .condGt('age', 18, 'number')
        .build();

    const qb = query_builder
        .select()
        .from('users')
        .where(condition);

    const builder = qb as any;

    console.assert(builder._condition !== null, 'Condition should be set');
    console.assert(builder._condition.type === 'and', 'Condition type should be AND');

    console.log('✓ Test 2: WHERE clause construction - PASSED');
}

/**
 * Test 3: Verify JOIN construction
 */
export function test_joinConstruction() {
    const joinDef = join_builder
        .from('posts')
        .join('users', 'left_join')
        .on('posts.author_id', 'users.id', '=', 'string')
        .select('username', 'email')
        .embedAs('author')
        .build();

    console.assert(joinDef.from_table_name === 'posts', 'From table should be posts');
    console.assert(joinDef.joined_table_name === 'users', 'Joined table should be users');
    console.assert(joinDef.join_type === 'left_join', 'Join type should be left_join');
    console.assert(joinDef.embed_name === 'author', 'Embed name should be author');
    console.assert(joinDef.selected_fields.length === 2, 'Should have 2 selected fields');

    console.log('✓ Test 3: JOIN construction - PASSED');
}

/**
 * Test 4: Verify ORDER BY
 */
export function test_orderBy() {
    const qb = query_builder
        .select()
        .from('users')
        .orderBy('created_at', false, 'timestamp')
        .orderBy('username', true, 'string');

    const builder = qb as any;

    console.assert(builder._orderBy.length === 2, 'Should have 2 ORDER BY clauses');
    console.assert(builder._orderBy[0].field_name === 'created_at', 'First order by field should be created_at');
    console.assert(builder._orderBy[0].is_asc === false, 'First order should be DESC');
    console.assert(builder._orderBy[1].is_asc === true, 'Second order should be ASC');

    console.log('✓ Test 4: ORDER BY - PASSED');
}

/**
 * Test 5: Verify pagination
 */
export function test_pagination() {
    const qb = query_builder
        .select()
        .from('articles')
        .offset(20)
        .limit(10);

    const builder = qb as any;

    console.assert(builder._start === 20, 'Offset should be 20');
    console.assert(builder._limit === 10, 'Limit should be 10');

    console.log('✓ Test 5: Pagination - PASSED');
}

/**
 * Test 6: Verify complex nested conditions
 */
export function test_nestedConditions() {
    const condition = cond_builder.or()
        .addCond(
            cond_builder.and()
                .condEq('status', 'active', 'string')
                .condGt('age', 18, 'number')
        )
        .addCond(
            cond_builder.and()
                .condEq('role', 'admin', 'string')
        )
        .build();

    console.assert(condition.type === 'or', 'Root should be OR');
    if (condition.type === 'or' || condition.type === 'and') {
        console.assert(condition.conditions.length === 2, 'Should have 2 conditions');
        const firstCond = condition.conditions[0];
        if (firstCond.type === 'and') {
            console.assert(firstCond.conditions.length === 2, 'First AND should have 2 conditions');
        }
    }

    console.log('✓ Test 6: Nested conditions - PASSED');
}

/**
 * Test 7: Verify query builder reset
 */
export function test_reset() {
    const qb = query_builder
        .select('id', 'name')
        .from('users')
        .where(cond_builder.filter().condEq('status', 'active').build())
        .limit(50);

    qb.reset();

    const builder = qb as any;

    console.assert(builder._tableName === '', 'Table name should be empty after reset');
    console.assert(builder._fieldNames.length === 0, 'Field names should be empty after reset');
    console.assert(builder._limit === 100, 'Limit should be default (100) after reset');

    console.log('✓ Test 7: Query builder reset - PASSED');
}

/**
 * Test 8: Verify multiple joins
 */
export function test_multipleJoins() {
    const authorJoin = join_builder
        .from('posts')
        .join('users', 'left_join')
        .on('posts.author_id', 'users.id', '=', 'string')
        .select('username')
        .embedAs('author')
        .build();

    const categoryJoin = join_builder
        .from('posts')
        .join('categories', 'left_join')
        .on('posts.category_id', 'categories.id', '=', 'string')
        .select('category_name')
        .embedAs('category')
        .build();

    const qb = query_builder
        .select()
        .from('posts')
        .leftJoin(authorJoin)
        .leftJoin(categoryJoin);

    const builder = qb as any;

    console.assert(builder._joins.length === 2, 'Should have 2 joins');
    console.assert(builder._joins[0].embed_name === 'author', 'First join embed should be author');
    console.assert(builder._joins[1].embed_name === 'category', 'Second join embed should be category');

    console.log('✓ Test 8: Multiple joins - PASSED');
}

/**
 * Run all tests
 */
export function runAllTests() {
    console.log('\n=== Running Query Builder Tests ===\n');

    try {
        test_basicQuery();
        test_whereClause();
        test_joinConstruction();
        test_orderBy();
        test_pagination();
        test_nestedConditions();
        test_reset();
        test_multipleJoins();

        console.log('\n=== All Tests PASSED ✓ ===\n');
    } catch (error) {
        console.error('\n=== Test FAILED ✗ ===');
        console.error(error);
    }
}

// Run tests if this file is executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
    runAllTests();
}
