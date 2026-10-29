<script lang="ts">
	import { searchIcons, type LibraryIcon } from '$lib/stores/iconLibrary';
	import Icon from './Icon.svelte';

	// Text is passed in by the host app (this library has no i18n of its own).
	interface IconSearchPickerLabels {
		placeholder: string;
		empty: string;
		error: string;
	}

	let {
		value = $bindable(''),
		onSelect = () => {},
		labels = {
			placeholder: 'Search icons by keyword or description',
			empty: 'No icons found',
			error: 'Icon search failed'
		},
		locale = 'en',
		limit = 48,
		columns = 6
	}: {
		value?: string;
		onSelect?: (name: string, icon: LibraryIcon) => void;
		labels?: IconSearchPickerLabels;
		locale?: string;
		limit?: number;
		columns?: number;
	} = $props();

	let query = $state('');
	let results = $state<LibraryIcon[]>([]);
	let failed = $state(false);
	let loading = $state(false);

	$effect(() => {
		const q = query;
		const ctl = new AbortController();
		const timer = setTimeout(async () => {
			loading = true;
			failed = false;
			try {
				results = await searchIcons(q, { limit, signal: ctl.signal });
			} catch (e) {
				if ((e as Error).name !== 'AbortError') failed = true;
			} finally {
				loading = false;
			}
		}, 250);
		return () => {
			clearTimeout(timer);
			ctl.abort();
		};
	});

	function title(ic: LibraryIcon) {
		const d = locale.startsWith('zh') ? ic.description_zh : ic.description_en;
		return d ? `${ic.name} — ${d}` : ic.name;
	}

	function pick(ic: LibraryIcon) {
		value = ic.name;
		onSelect(ic.name, ic);
	}
</script>

<div class="flex select-text flex-col gap-3">
	<input
		type="search"
		class="w-full rounded border border-gray-300 bg-transparent px-3 py-2 text-sm dark:border-gray-600"
		placeholder={labels.placeholder}
		aria-label={labels.placeholder}
		bind:value={query}
	/>
	{#if failed}
		<p class="text-sm text-red-600">{labels.error}</p>
	{:else if !loading && results.length === 0}
		<p class="text-sm opacity-60">{labels.empty}</p>
	{:else}
		<div class="grid gap-2" style="grid-template-columns: repeat({columns}, minmax(0, 1fr));">
			{#each results as ic (ic.name)}
				<button
					type="button"
					class="flex flex-col items-center gap-1 rounded border p-2 text-xs hover:bg-gray-50 dark:hover:bg-gray-800 {value ===
					ic.name
						? 'border-blue-500 ring-2 ring-blue-500'
						: 'border-gray-200 dark:border-gray-700'}"
					title={title(ic)}
					onclick={() => pick(ic)}
				>
					<Icon name={ic.name} size={24} />
					<span class="w-full truncate text-center">{ic.name}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
