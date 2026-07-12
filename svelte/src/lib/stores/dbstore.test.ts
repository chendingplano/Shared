/**
 * Tests for db_store error paths — the outer catch blocks must return a
 * JimoResponse carrying the real error message (regression: the non-Error
 * branch used to fall through to a generic "Unknown error occurred").
 */

import { describe, it, expect, vi, afterEach } from 'vitest';
import { db_store } from './dbstore';
import { cond_builder } from './cond_builder';
import { CustomHttpStatus } from '../types/CommonTypes';

function callRetrieve() {
	return db_store.retrieveRecords(
		'tax_db',
		'users',
		['id'],
		[],
		'TEST_LOC',
		cond_builder.null(),
		[],
		[],
		null,
		'',
		null,
		0,
		10
	);
}

describe('db_store.retrieveRecords error paths', () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('returns the error message when fetch rejects with an Error', async () => {
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('connection refused')));

		const resp = await callRetrieve();

		expect(resp.status).toBe(false);
		expect(resp.error_code).toBe(CustomHttpStatus.ServerException);
		expect(resp.loc).toBe('SHD_DST_176');
		expect(resp.error_msg).toContain('connection refused');
	});

	it('returns the error message when fetch rejects with a non-Error value', async () => {
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue('kaboom'));

		const resp = await callRetrieve();

		expect(resp.status).toBe(false);
		expect(resp.error_code).toBe(CustomHttpStatus.ServerException);
		expect(resp.loc).toBe('SHD_DST_176');
		expect(resp.error_msg).toContain('kaboom');
	});
});
