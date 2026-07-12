/**
 * Tests for the parseCondition function
 */

import { describe, it, expect } from 'vitest';
import { parseCondition } from './cond_builder';
import type { AtomicCondition, GroupCondition } from '../types/CommonTypes';

describe('parseCondition', () => {
	it('parses a simple AND condition', () => {
		const cond = parseCondition("field1 = 'xxx' AND field2 > 100") as GroupCondition;

		expect(cond.type).toBe('and');
		expect(cond.conditions).toHaveLength(2);
		expect((cond.conditions[0] as AtomicCondition).field_name).toBe('field1');
		expect((cond.conditions[1] as AtomicCondition).field_name).toBe('field2');
	});

	it('parses a simple OR condition', () => {
		const cond = parseCondition("status = 'active' OR status = 'pending'") as GroupCondition;

		expect(cond.type).toBe('or');
		expect(cond.conditions).toHaveLength(2);
	});

	it('parses nested conditions with parentheses', () => {
		const cond = parseCondition(
			"field1 = 'xxx' AND (field2 > 100 OR field3 < 50)"
		) as GroupCondition;

		expect(cond.type).toBe('and');
		expect(cond.conditions).toHaveLength(2);
		expect(cond.conditions[0].type).toBe('atomic');
		const nested = cond.conditions[1] as GroupCondition;
		expect(nested.type).toBe('or');
		expect(nested.conditions).toHaveLength(2);
	});

	it('parses comparison and CONTAINS operators', () => {
		const cond = parseCondition(
			"age >= 18 AND name CONTAINS 'John' AND score <= 100"
		) as GroupCondition;

		expect(cond.type).toBe('and');
		expect(cond.conditions).toHaveLength(3);
		const fields = cond.conditions.map((c) => (c as AtomicCondition).field_name);
		expect(fields).toEqual(['age', 'name', 'score']);
	});

	it('parses && and || operators', () => {
		const cond = parseCondition(
			"field1 == 'xxx' && field2 > 100 || field3 = 'yyy'"
		) as GroupCondition;

		// AND binds tighter than OR: (field1 == 'xxx' AND field2 > 100) OR field3 = 'yyy'
		expect(cond.type).toBe('or');
		expect(cond.conditions).toHaveLength(2);
		const left = cond.conditions[0] as GroupCondition;
		expect(left.type).toBe('and');
		expect(left.conditions).toHaveLength(2);
	});

	it('gives AND precedence over OR without parentheses', () => {
		const cond = parseCondition(
			"status = 'in_progress' AND app_type = 'tax_return' OR status = 'completed'"
		) as GroupCondition;

		// (status = 'in_progress' AND app_type = 'tax_return') OR (status = 'completed')
		expect(cond.type).toBe('or');
		expect(cond.conditions).toHaveLength(2);
		expect(cond.conditions[0].type).toBe('and');
		expect(cond.conditions[1].type).toBe('atomic');
	});

	it('returns a bare atomic condition when there is no AND/OR', () => {
		const cond = parseCondition("status = 'active'") as AtomicCondition;

		expect(cond.type).toBe('atomic');
		expect(cond.field_name).toBe('status');
		expect(cond.value).toBe('active');
	});

	it('throws on empty input', () => {
		expect(() => parseCondition('')).toThrow();
	});

	it('throws on unbalanced parentheses', () => {
		expect(() => parseCondition("field1 = 'x' AND (field2 > 1")).toThrow();
	});

	it('respects explicit grouping over default precedence', () => {
		const cond = parseCondition(
			"status = 'in_progress' AND (app_type = 'tax_return' OR status = 'completed')"
		) as GroupCondition;

		// status = 'in_progress' AND (app_type = 'tax_return' OR status = 'completed')
		expect(cond.type).toBe('and');
		expect(cond.conditions).toHaveLength(2);
		expect(cond.conditions[0].type).toBe('atomic');
		expect(cond.conditions[1].type).toBe('or');
	});
});
