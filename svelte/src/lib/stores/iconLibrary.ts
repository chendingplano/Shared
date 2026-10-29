/** Client for the icon library (shared.icon_library): search and SVG fetch with caching. */
const BASE_URL = '/shared_api/v1/icons';

export interface LibraryIcon {
	name: string;
	source_set: 'lucide' | 'tabler' | 'custom';
	category_en: string;
	category_zh: string;
	tags_en: string;
	tags_zh: string;
	description_en: string;
	description_zh: string;
	version: number;
	score: number;
}

const svgCache = new Map<string, Promise<string | null>>();

/** Search by keywords or a short description (English or Chinese). */
export async function searchIcons(
	query: string,
	opts: { category?: string; limit?: number; signal?: AbortSignal } = {}
): Promise<LibraryIcon[]> {
	const params = new URLSearchParams({ q: query });
	if (opts.category) params.set('category', opts.category);
	if (opts.limit) params.set('limit', String(opts.limit));
	const resp = await fetch(`${BASE_URL}/search?${params}`, { signal: opts.signal });
	const json = await resp.json();
	if (!json.status) throw new Error(json.error_msg ?? 'icon search failed');
	return (json.results ?? []) as LibraryIcon[];
}

/** Returns the SVG markup of an icon, or null when it does not exist. Cached per name. */
export function getIconSvg(name: string): Promise<string | null> {
	let p = svgCache.get(name);
	if (!p) {
		p = fetch(`${BASE_URL}/by-name/${encodeURIComponent(name)}/svg`)
			.then((r) => (r.ok ? r.text() : null))
			.catch(() => null);
		svgCache.set(name, p);
	}
	return p;
}
