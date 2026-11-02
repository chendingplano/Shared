<script lang="ts">
	import { emojiSvgUrl } from '../../stores/emojiLibrary';
	let { value, size = 24, label = '', decorative = false }: { value: string; size?: number; label?: string; decorative?: boolean } = $props();
	let failed = $state(false);
	$effect(() => { value; failed = false; });
</script>

{#if value}
	{#if failed}
		<span aria-hidden={decorative ? 'true' : undefined} role={decorative ? undefined : 'img'} aria-label={decorative ? undefined : (label || value)} style:font-size="{size}px">{value}</span>
	{:else}
		<img src={emojiSvgUrl(value)} alt={decorative ? '' : (label || value)} width={size} height={size} draggable="false" onerror={() => failed = true} />
	{/if}
{/if}

<style>img { display: inline-block; vertical-align: -0.15em; object-fit: contain; }</style>
