<script lang="ts">
	import { emojiCategories, getEmoji, searchEmojis, type EmojiCategory, type LibraryEmoji } from '../../stores/emojiLibrary';
	import Emoji from './Emoji.svelte';
	export interface EmojiPickerLabels { placeholder: string; empty: string; error: string; loading: string; all: string; category: string; more: string; attribution: string }
	let { value = '', onSelect = () => {}, locale = 'en', limit = 30, labels }: { value?: string; onSelect?: (value: string, emoji: LibraryEmoji) => void; locale?: string; limit?: number; labels: EmojiPickerLabels } = $props();
	let query = $state('');
	let category = $state('');
	let categories = $state<EmojiCategory[]>([]);
	let results = $state<LibraryEmoji[]>([]);
	let next = $state<number | undefined>();
	let loading = $state(false);
	let failed = $state(false);
	let selectedEmoji = $state<LibraryEmoji | undefined>();
	let generation = 0;
	let active: AbortController | undefined;
	const zh = $derived(locale.startsWith('zh'));
	$effect(() => {
		const current = value;
		selectedEmoji = undefined;
		if (!current) return;
		const ctl = new AbortController();
		getEmoji(current, ctl.signal).then((emoji) => {if (!ctl.signal.aborted) selectedEmoji = emoji;}).catch(() => {});
		return () => ctl.abort();
	});
	$effect(() => {
		const controller = new AbortController();
		emojiCategories(controller.signal).then((c) => categories = c).catch(() => {});
		return () => controller.abort();
	});
	$effect(() => {
		const q = query, cat = category, size = limit;
		const current = ++generation;
		active?.abort();
		const ctl = new AbortController(); active = ctl;
		results = []; next = undefined; loading = true; failed = false;
		const timer = setTimeout(async () => {
			try { const page = await searchEmojis(q, {category: cat, limit: size, signal: ctl.signal}); if (current === generation) {results = page.items; next = page.next_offset;} }
			catch { if (!ctl.signal.aborted && current === generation) failed = true; }
			finally { if (current === generation) loading = false; }
		}, 200);
		return () => { clearTimeout(timer); ctl.abort(); };
	});
	async function more() {
		if (next === undefined || loading) return;
		const current = generation, offset = next, ctl = active;
		loading = true; failed = false;
		try { const page = await searchEmojis('', { category, limit, offset, signal: ctl?.signal }); if (current === generation) { results = [...results, ...page.items]; next = page.next_offset; } }
		catch { if (current === generation && !ctl?.signal.aborted) failed = true; }
		finally { if (current === generation) loading = false; }
	}
</script>

<div class="emoji-picker">
	{#if value}
		<div class="selected-value"><Emoji {value} size={24} decorative /><span>{selectedEmoji ? (zh ? selectedEmoji.name_zh : selectedEmoji.name_en) : value}</span></div>
	{/if}
	<input type="search" bind:value={query} placeholder={labels.placeholder} aria-label={labels.placeholder} />
	<select bind:value={category} aria-label={labels.category}>
		<option value="">{labels.all}</option>
		{#each categories as c (c.id)}<option value={c.id}>{zh ? c.name_zh : c.name_en}</option>{/each}
	</select>
	{#if failed}<p role="alert">{labels.error}</p>{/if}
	{#if loading}<p role="status">{labels.loading}</p>{:else if !failed && results.length === 0}<p>{labels.empty}</p>{/if}
	<div class="emoji-results">
		{#each results as emoji (emoji.unicode_id)}
			{@const name = zh ? emoji.name_zh : emoji.name_en}
			{@const detail = zh ? emoji.description_zh : emoji.description_en}
			<button type="button" class:selected={value === emoji.unicode_value} title={detail} aria-label={name} aria-pressed={value === emoji.unicode_value} onclick={() => onSelect(emoji.unicode_value, emoji)}><Emoji value={emoji.unicode_value} size={24} decorative /><span>{name}</span></button>
		{/each}
	</div>
	{#if next !== undefined && !query}<button type="button" disabled={loading} onclick={more}>{labels.more}</button>{/if}
	<a href="/shared_api/v1/emojis/notices/NOTICE.txt" target="_blank" rel="noreferrer">{labels.attribution}</a>
</div>

<style>
	.emoji-picker { user-select: text; display: flex; flex-direction: column; gap: .5rem; }
	.selected-value { display: flex; align-items: center; gap: .4rem; }
	input, select, button { color: inherit; background: transparent; border: 1px solid var(--cv-border, #888); border-radius: .3rem; padding: .4rem; }
	.emoji-results { display: grid; grid-template-columns: repeat(4, minmax(0,1fr)); gap: .25rem; }
	.emoji-results button { min-width: 0; display: flex; flex-direction: column; align-items: center; gap: .25rem; cursor: pointer; }
	.emoji-results span { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: .7rem; }
	.selected { outline: 2px solid var(--cv-accent, #3273d9); }
	button:focus-visible, input:focus-visible, select:focus-visible { outline: 2px solid var(--cv-accent, #3273d9); outline-offset: 2px; }
</style>
