const BASE = '/shared_api/v1/emojis';

// Mirrors emojilib.Emoji in shared/go.
export interface LibraryEmoji {
	unicode_id: string; unicode_value: string; name_en: string; name_zh: string;
	category_id: string; category_en: string; category_zh: string;
	keywords_en: string; keywords_zh: string; description_en: string; description_zh: string;
	version: number; score: number;
}
export interface EmojiCategory { id: string; name_en: string; name_zh: string }
export interface EmojiPage { items: LibraryEmoji[]; next_offset?: number }

export function emojiId(value: string): string {
	return Array.from(value, (c) => c.codePointAt(0)!.toString(16)).join('-');
}
export function emojiSvgUrl(value: string): string {
	return `${BASE}/by-id/${encodeURIComponent(emojiId(value))}/svg`;
}
async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
	const res = await fetch(`${BASE}${path}`, { signal });
	if (!res.ok) throw new Error(`emoji request failed (${res.status})`);
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg ?? 'emoji request failed');
	return data.results as T;
}
export function searchEmojis(query: string, options: {category?: string; limit?: number; offset?: number; signal?: AbortSignal} = {}): Promise<EmojiPage> {
	const p = new URLSearchParams({ q: query });
	if (options.category) p.set('category', options.category);
	if (options.limit) p.set('limit', String(options.limit));
	if (options.offset) p.set('offset', String(options.offset));
	return get(`/search?${p}`, options.signal);
}
export function emojiCategories(signal?: AbortSignal): Promise<EmojiCategory[]> { return get('/categories', signal); }
export function getEmoji(value: string, signal?: AbortSignal): Promise<LibraryEmoji> { return get(`/by-id/${encodeURIComponent(emojiId(value))}`, signal); }
