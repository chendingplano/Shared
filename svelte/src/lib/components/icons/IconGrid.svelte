<script lang="ts">
  import type { IconDef } from '$lib/types/IconTypes';
  import { iconStore } from '$lib/stores/iconStore';

  // Props
  export let icons: IconDef[] = [];
  export let selectedId: string | null = null;
  export let onSelect: (icon: IconDef) => void = () => {};
  export let showDelete: boolean = false;
  export let onDelete: (icon: IconDef) => void = () => {};
  export let columns: number = 6;
</script>

<div
  class="grid gap-3"
  style="grid-template-columns: repeat({columns}, minmax(0, 1fr));"
>
  {#each icons as icon (icon.id)}
    <div
      class="relative flex flex-col items-center p-2 rounded-lg border cursor-pointer transition-all hover:bg-gray-50 dark:hover:bg-gray-800"
      class:ring-2={selectedId === icon.id}
      class:ring-blue-500={selectedId === icon.id}
      class:border-blue-500={selectedId === icon.id}
      onclick={() => onSelect(icon)}
      role="button"
      tabindex="0"
      onkeydown={(e) => e.key === 'Enter' && onSelect(icon)}
    >
      <img
        src={iconStore.getIconUrl(icon)}
        alt={icon.name}
        class="h-10 w-10 object-contain"
        loading="lazy"
      />
      <span class="mt-1 text-xs text-center truncate w-full text-gray-700 dark:text-gray-300" title={icon.name}>
        {icon.name}
      </span>

      {#if showDelete}
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); onDelete(icon); }}
          class="absolute -top-1 -right-1 h-5 w-5 rounded-full bg-red-500 text-white flex items-center justify-center text-xs hover:bg-red-600 transition-colors"
          title="Delete icon"
        >
          &times;
        </button>
      {/if}
    </div>
  {/each}
</div>

{#if icons.length === 0}
  <div class="text-center py-8 text-gray-500 dark:text-gray-400">
    No icons found
  </div>
{/if}
