/**
 * Tests for RetrieveUsers — pins the 13-argument db_store.retrieveRecords
 * contract (regression: the call drifted to 10 args and read a nonexistent
 * `records` field instead of `results`).
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';

const mocks = vi.hoisted(() => ({
	retrieveRecords: vi.fn()
}));

vi.mock('@chendingplano/shared', async () => {
	const { cond_builder } = await import('../stores/cond_builder');
	const { orderby_builder } = await import('../stores/orderby_builder');
	return {
		cond_builder,
		orderby_builder,
		db_store: { retrieveRecords: mocks.retrieveRecords }
	};
});

import { RetrieveUsers } from './table-users';

const validUser = {
	user_id: 'u1',
	user_name: 'alice',
	email: 'alice@example.com',
	user_type: 'member',
	user_status: 'active'
};

const invalidUser = { user_id: 'u2' };

describe('RetrieveUsers', () => {
	beforeEach(() => {
		mocks.retrieveRecords.mockReset();
	});

	it('passes all 13 positional arguments to retrieveRecords', async () => {
		mocks.retrieveRecords.mockResolvedValue({ status: true, results: [] });

		await RetrieveUsers('TEST_LOC', false);

		expect(mocks.retrieveRecords).toHaveBeenCalledTimes(1);
		const args = mocks.retrieveRecords.mock.calls[0];
		expect(args).toHaveLength(13);
		const [
			,
			table_name,
			field_names,
			,
			loc,
			,
			join_def,
			,
			record_schema,
			embed_name,
			embed_schema,
			start,
			num_records
		] = args;
		expect(table_name).toBeTruthy();
		expect(field_names).toContain('email');
		expect(loc).toBe('TEST_LOC');
		expect(join_def).toEqual([]);
		expect(record_schema).toBeNull();
		expect(embed_name).toBe('');
		expect(embed_schema).toBeNull();
		expect(start).toBe(0);
		expect(num_records).toBe(200);
	});

	it('keeps valid records and skips invalid ones', async () => {
		vi.stubGlobal('console', { ...console, warn: vi.fn() });
		mocks.retrieveRecords.mockResolvedValue({
			status: true,
			results: [validUser, invalidUser]
		});

		const [users, error_msg] = await RetrieveUsers('TEST_LOC', false);

		expect(error_msg).toBe('');
		expect(users).toHaveLength(1);
		expect(users[0]).toMatchObject({ user_id: 'u1' });
		vi.unstubAllGlobals();
	});

	it('returns the error message when the response has status false', async () => {
		vi.stubGlobal('console', { ...console, warn: vi.fn() });
		mocks.retrieveRecords.mockResolvedValue({
			status: false,
			error_msg: 'db down',
			error_code: 500,
			loc: 'SRV_LOC'
		});

		const [users, error_msg] = await RetrieveUsers('TEST_LOC', false);

		expect(users).toEqual([]);
		expect(error_msg).toContain('db down');
		vi.unstubAllGlobals();
	});

	it('rejects a non-array results payload', async () => {
		vi.stubGlobal('console', { ...console, warn: vi.fn() });
		mocks.retrieveRecords.mockResolvedValue({ status: true, results: 'not-an-array' });

		const [users, error_msg] = await RetrieveUsers('TEST_LOC', false);

		expect(users).toEqual([]);
		expect(error_msg).not.toBe('');
		vi.unstubAllGlobals();
	});
});
