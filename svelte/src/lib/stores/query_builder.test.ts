///////////////////////////////////////////////////////
// Query Builder Tests
//
// Simple test file to verify query builder functionality
//
// Created: 2026/01/01 by Chen Ding
///////////////////////////////////////////////////////
/* eslint-disable @typescript-eslint/no-explicit-any -- tests inspect private builder fields via `as any` */

import { describe, it, expect } from 'vitest';
import { query_builder } from './query_builder';
import { cond_builder } from './cond_builder';
import { join_builder } from './join_builder';

describe('query_builder', () => {
	it('builds basic query structure', () => {
		const qb = query_builder.select('id', 'name', 'email').from('users');

		// Access private fields through type casting for testing
		const builder = qb as any;

		expect(builder._tableName).toBe('users');
		expect(builder._fieldNames).toHaveLength(3);
		expect(builder._fieldNames[0]).toBe('id');
	});

	it('constructs WHERE clause', () => {
		const condition = cond_builder
			.and()
			.condEq('status', 'active', 'string')
			.condGt('age', 18, 'number')
			.build();

		const qb = query_builder.select().from('users').where(condition);

		const builder = qb as any;

		expect(builder._condition).not.toBeNull();
		expect(builder._condition.type).toBe('and');
	});

	it('constructs JOIN definition', () => {
		const joinDef = join_builder
			.from('posts')
			.join('users', 'left_join')
			.on('posts.author_id', 'users.id', '=', 'string')
			.select('username', 'email')
			.embedAs('author')
			.build();

		expect(joinDef.from_table_name).toBe('posts');
		expect(joinDef.joined_table_name).toBe('users');
		expect(joinDef.join_type).toBe('left_join');
		expect(joinDef.embed_name).toBe('author');
		expect(joinDef.selected_fields).toHaveLength(2);
	});

	it('builds ORDER BY clauses', () => {
		const qb = query_builder
			.select()
			.from('users')
			.orderBy('created_at', false, 'timestamp')
			.orderBy('username', true, 'string');

		const builder = qb as any;

		expect(builder._orderBy).toHaveLength(2);
		expect(builder._orderBy[0].field_name).toBe('created_at');
		expect(builder._orderBy[0].is_asc).toBe(false);
		expect(builder._orderBy[1].is_asc).toBe(true);
	});

	it('sets pagination offset and limit', () => {
		const qb = query_builder.select().from('articles').offset(20).limit(10);

		const builder = qb as any;

		expect(builder._start).toBe(20);
		expect(builder._limit).toBe(10);
	});

	it('builds complex nested conditions', () => {
		const condition = cond_builder
			.or()
			.addCond(cond_builder.and().condEq('status', 'active', 'string').condGt('age', 18, 'number'))
			.addCond(cond_builder.and().condEq('role', 'admin', 'string'))
			.build();

		expect(condition.type).toBe('or');
		if (condition.type === 'or' || condition.type === 'and') {
			expect(condition.conditions).toHaveLength(2);
			const firstCond = condition.conditions[0];
			expect(firstCond.type).toBe('and');
			if (firstCond.type === 'and') {
				expect(firstCond.conditions).toHaveLength(2);
			}
		}
	});

	it('sets the database name via the constructor entry point', () => {
		const qb = query_builder.database('tax_db').from('users');

		const builder = qb as any;

		expect(builder._dbName).toBe('tax_db');
		expect(builder._tableName).toBe('users');
	});

	it('resets builder state', () => {
		const qb = query_builder
			.select('id', 'name')
			.from('users')
			.database('tax_db')
			.where(cond_builder.filter().condEq('status', 'active').build())
			.limit(50);

		qb.reset();

		const builder = qb as any;

		expect(builder._tableName).toBe('');
		expect(builder._dbName).toBe('');
		expect(builder._fieldNames).toHaveLength(0);
		expect(builder._limit).toBe(100);
	});

	it('supports multiple joins', () => {
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

		const qb = query_builder.select().from('posts').leftJoin(authorJoin).leftJoin(categoryJoin);

		const builder = qb as any;

		expect(builder._joins).toHaveLength(2);
		expect(builder._joins[0].embed_name).toBe('author');
		expect(builder._joins[1].embed_name).toBe('category');
	});
});
