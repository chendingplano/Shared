/**
 * Tests for activityLogStore — pins the logout/isLoggedIn contract
 * (regression: logout() used to null the user without clearing isLoggedIn).
 */

import { describe, it, expect, vi, afterEach } from 'vitest';
import { activityLogStore } from './activity-log.svelte';
import type { UserInfo } from '../types/CommonTypes';

const user = {
	user_id: 'u1',
	user_name: 'alice',
	email: 'alice@example.com',
	user_type: 'member',
	user_status: 'active'
} as unknown as UserInfo;

describe('activityLogStore', () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		activityLogStore.setUserInfo(null);
	});

	it('logout clears both user and isLoggedIn', () => {
		activityLogStore.setUserInfo(user);
		expect(activityLogStore.isLoggedIn).toBe(true);

		activityLogStore.logout();

		expect(activityLogStore.isLoggedIn).toBe(false);
		expect(activityLogStore.user).toBeNull();
	});

	it('register sets status to error on password mismatch', async () => {
		await activityLogStore.register({
			email: 'a@b.c',
			password: 'one',
			passwordConfirm: 'two',
			first_name: 'A',
			last_name: 'B'
		});

		expect(activityLogStore.status).toBe('error');
	});

	it('register sets status to error when signup fails', async () => {
		vi.stubGlobal('alert', vi.fn());
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, text: async () => 'nope' }));

		await activityLogStore.register({
			email: 'a@b.c',
			password: 'same',
			passwordConfirm: 'same',
			first_name: 'A',
			last_name: 'B'
		});

		expect(activityLogStore.status).toBe('error');
	});

	it('register sets status to pending on success', async () => {
		vi.stubGlobal('alert', vi.fn());
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));

		await activityLogStore.register({
			email: 'a@b.c',
			password: 'same',
			passwordConfirm: 'same',
			first_name: 'A',
			last_name: 'B'
		});

		expect(activityLogStore.status).toBe('pending');
	});
});
