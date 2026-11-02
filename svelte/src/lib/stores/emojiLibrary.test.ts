import { describe, expect, it } from 'vitest';
import { emojiId, emojiSvgUrl } from './emojiLibrary';

describe('emoji Unicode identity', () => {
	it('preserves joiners and variation selectors', () => {
		expect(emojiId('👩‍❤️‍👩')).toBe('1f469-200d-2764-fe0f-200d-1f469');
	});
	it('uses a local asset URL', () => {
		expect(emojiSvgUrl('🎉')).toBe('/shared_api/v1/emojis/by-id/1f389/svg');
	});
});
