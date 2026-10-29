<script lang="ts">
	import { getIconSvg } from '$lib/stores/iconLibrary';

	// Renders a library icon by name. The server sanitises SVG bodies, so inlining is safe.
	// Colour follows the surrounding text colour (currentColor).
	let { name, size = 20, class: className = '' }: { name: string; size?: number; class?: string } =
		$props();

	let svg = $state<string | null>(null);
	let missing = $state(false);

	$effect(() => {
		const n = name;
		svg = null;
		missing = false;
		getIconSvg(n).then((s) => {
			if (n !== name) return;
			if (s === null) missing = true;
			else svg = s;
		});
	});
</script>

<span
	class="inline-flex shrink-0 items-center justify-center align-middle [&>svg]:h-full [&>svg]:w-full {className}"
	style="width:{size}px;height:{size}px"
	aria-hidden="true"
>
	{#if svg}
		{@html svg}
	{:else if missing}
		<span
			class="block h-full w-full rounded border border-dashed border-current opacity-40"
		></span>
	{/if}
</span>
