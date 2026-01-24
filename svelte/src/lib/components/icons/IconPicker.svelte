<script lang="ts">
  import { onMount } from 'svelte';
  import type { IconDef } from '$lib/types/IconTypes';
  import { iconStore } from '$lib/stores/iconStore';
  import IconGrid from './IconGrid.svelte';

  // Props
  export let selected: IconDef | null = null;
  export let onSelect: (icon: IconDef) => void = () => {};
  export let allowedCategories: string[] | null = null;

  // State
  let icons: IconDef[] = [];
  let categories: string[] = [];
  let loading = true;
  let searchQuery = '';
  let selectedCategory: string | null = null;
  let error: string | null = null;

  // Filter icons based on search and category
  $: filteredIcons = icons.filter((icon) => {
    const matchesSearch =
      !searchQuery ||
      icon.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      icon.tags.some((t) => t.toLowerCase().includes(searchQuery.toLowerCase()));
    const matchesCategory = !selectedCategory || icon.category === selectedCategory;
    return matchesSearch && matchesCategory;
  });

  // Filter categories if allowedCategories is set
  $: displayCategories = allowedCategories
    ? categories.filter((c) => allowedCategories.includes(c))
    : categories;

  onMount(() => {
    loadData();
  });

  async function loadData() {
    loading = true;
    error = null;
    try {
      const [iconsResp, cats] = await Promise.all([
        iconStore.listIcons({ page_size: 200 }),
        iconStore.getCategories(),
      ]);
      icons = iconsResp.icons;
      categories = cats;
    } catch (err) {
      console.error('Failed to load icons:', err);
      error = err instanceof Error ? err.message : 'Failed to load icons';
    } finally {
      loading = false;
    }
  }

  function handleSelect(icon: IconDef) {
    selected = icon;
    onSelect(icon);
  }

  function handleCategoryChange(category: string | null) {
    selectedCategory = category;
  }
</script>

<div class="flex flex-col gap-4">
  <!-- Search and category filter -->
  <div class="flex gap-2 flex-wrap">
    <input
      type="text"
      placeholder="Search icons..."
      bind:value={searchQuery}
      class="flex-1 min-w-48 rounded-md border border-gray-300 dark:border-gray-600 px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
    />
    <select
      bind:value={selectedCategory}
      onchange={() => handleCategoryChange(selectedCategory)}
      class="rounded-md border border-gray-300 dark:border-gray-600 px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
    >
      <option value={null}>All Categories</option>
      {#each displayCategories as cat}
        <option value={cat}>{cat}</option>
      {/each}
    </select>
  </div>

  <!-- Error message -->
  {#if error}
    <div class="text-red-500 text-sm p-2 bg-red-50 dark:bg-red-900/20 rounded">
      {error}
    </div>
  {/if}

  <!-- Icon grid -->
  {#if loading}
    <div class="flex justify-center py-8">
      <div class="animate-pulse text-gray-500 dark:text-gray-400">Loading icons...</div>
    </div>
  {:else}
    <div class="max-h-96 overflow-y-auto">
      <IconGrid
        icons={filteredIcons}
        selectedId={selected?.id ?? null}
        onSelect={handleSelect}
      />
    </div>
  {/if}
</div>
