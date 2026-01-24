<script lang="ts">
  import { onMount } from 'svelte';
  import type { IconDef } from '$lib/types/IconTypes';
  import { iconStore } from '$lib/stores/iconStore';
  import { isAllowedMimeType, AllowedMimeTypes } from '$lib/types/IconTypes';
  import IconGrid from './IconGrid.svelte';

  // State
  let icons: IconDef[] = [];
  let categories: string[] = [];
  let loading = true;
  let uploading = false;
  let selectedCategory: string | null = null;
  let showUploadDialog = false;
  let error: string | null = null;

  // Upload form state
  let uploadFile: File | null = null;
  let uploadName = '';
  let uploadCategory = '';
  let uploadTags = '';
  let uploadDescription = '';
  let uploadError: string | null = null;

  // Filter icons by selected category
  $: filteredIcons = selectedCategory
    ? icons.filter((icon) => icon.category === selectedCategory)
    : icons;

  onMount(() => {
    loadData();
  });

  async function loadData() {
    loading = true;
    error = null;
    try {
      const [iconsResp, cats] = await Promise.all([
        iconStore.listIcons({ category: selectedCategory ?? undefined, page_size: 200 }),
        iconStore.getCategories(),
      ]);
      icons = iconsResp.icons;
      categories = cats;
    } catch (err) {
      console.error('Failed to load data:', err);
      error = err instanceof Error ? err.message : 'Failed to load icons';
    } finally {
      loading = false;
    }
  }

  function handleFileSelect(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) {
      if (!isAllowedMimeType(file.type)) {
        uploadError = `Invalid file type: ${file.type}. Allowed: ${AllowedMimeTypes.join(', ')}`;
        uploadFile = null;
        return;
      }
      uploadError = null;
      uploadFile = file;
      // Auto-fill name from filename if empty
      if (!uploadName) {
        uploadName = file.name.replace(/\.[^/.]+$/, '');
      }
    }
  }

  async function handleUpload() {
    if (!uploadFile) {
      uploadError = 'Please select a file';
      return;
    }
    if (!uploadName.trim()) {
      uploadError = 'Please enter a name';
      return;
    }
    if (!uploadCategory.trim()) {
      uploadError = 'Please enter a category';
      return;
    }

    uploading = true;
    uploadError = null;
    try {
      const tags = uploadTags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean);

      const newIcon = await iconStore.uploadIcon(uploadFile, {
        name: uploadName.trim(),
        category: uploadCategory.trim(),
        tags,
        description: uploadDescription.trim() || undefined,
      });

      // Add to list and update categories if new
      icons = [newIcon, ...icons];
      if (!categories.includes(newIcon.category)) {
        categories = [...categories, newIcon.category].sort();
      }

      resetUploadForm();
      showUploadDialog = false;
    } catch (err) {
      console.error('Upload failed:', err);
      uploadError = err instanceof Error ? err.message : 'Upload failed';
    } finally {
      uploading = false;
    }
  }

  async function handleDelete(icon: IconDef) {
    if (!confirm(`Delete icon "${icon.name}"?`)) return;

    try {
      await iconStore.deleteIcon(icon.id);
      icons = icons.filter((i) => i.id !== icon.id);

      // Refresh categories in case one became empty
      const remainingCategories = [...new Set(icons.map((i) => i.category))].sort();
      categories = remainingCategories;
    } catch (err) {
      console.error('Delete failed:', err);
      alert(`Delete failed: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  }

  function resetUploadForm() {
    uploadFile = null;
    uploadName = '';
    uploadCategory = '';
    uploadTags = '';
    uploadDescription = '';
    uploadError = null;
  }

  function openUploadDialog() {
    resetUploadForm();
    showUploadDialog = true;
  }

  function closeUploadDialog() {
    showUploadDialog = false;
    resetUploadForm();
  }
</script>

<div class="flex flex-col gap-4">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <h2 class="text-xl font-semibold text-gray-900 dark:text-gray-100">Icon Manager</h2>
    <button
      type="button"
      onclick={openUploadDialog}
      class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
    >
      Add Icon
    </button>
  </div>

  <!-- Error message -->
  {#if error}
    <div class="text-red-500 text-sm p-2 bg-red-50 dark:bg-red-900/20 rounded">
      {error}
    </div>
  {/if}

  <!-- Category filter tabs -->
  <div class="flex gap-2 flex-wrap border-b border-gray-200 dark:border-gray-700 pb-2">
    <button
      type="button"
      onclick={() => { selectedCategory = null; loadData(); }}
      class="px-3 py-1 text-sm rounded-md transition-colors"
      class:bg-blue-100={!selectedCategory}
      class:text-blue-700={!selectedCategory}
      class:dark:bg-blue-900={!selectedCategory}
      class:dark:text-blue-200={!selectedCategory}
      class:text-gray-600={selectedCategory}
      class:hover:bg-gray-100={selectedCategory}
      class:dark:text-gray-400={selectedCategory}
      class:dark:hover:bg-gray-800={selectedCategory}
    >
      All ({icons.length})
    </button>
    {#each categories as cat}
      {@const count = icons.filter((i) => i.category === cat).length}
      <button
        type="button"
        onclick={() => { selectedCategory = cat; }}
        class="px-3 py-1 text-sm rounded-md transition-colors"
        class:bg-blue-100={selectedCategory === cat}
        class:text-blue-700={selectedCategory === cat}
        class:dark:bg-blue-900={selectedCategory === cat}
        class:dark:text-blue-200={selectedCategory === cat}
        class:text-gray-600={selectedCategory !== cat}
        class:hover:bg-gray-100={selectedCategory !== cat}
        class:dark:text-gray-400={selectedCategory !== cat}
        class:dark:hover:bg-gray-800={selectedCategory !== cat}
      >
        {cat} ({count})
      </button>
    {/each}
  </div>

  <!-- Icon grid with delete actions -->
  {#if loading}
    <div class="flex justify-center py-8">
      <div class="animate-pulse text-gray-500 dark:text-gray-400">Loading icons...</div>
    </div>
  {:else}
    <IconGrid icons={filteredIcons} showDelete={true} onDelete={handleDelete} />
  {/if}
</div>

<!-- Upload dialog -->
{#if showUploadDialog}
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
    onclick={closeUploadDialog}
    onkeydown={(e) => e.key === 'Escape' && closeUploadDialog()}
    role="button"
    tabindex="-1"
    aria-label="Close dialog"
  >
    <div
      class="bg-white dark:bg-gray-900 rounded-lg shadow-xl p-6 w-full max-w-md mx-4"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      tabindex="-1"
    >
      <h3 class="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Upload Icon</h3>

      {#if uploadError}
        <div class="text-red-500 text-sm p-2 bg-red-50 dark:bg-red-900/20 rounded mb-4">
          {uploadError}
        </div>
      {/if}

      <div class="flex flex-col gap-4">
        <!-- File input -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
                 for="icon-file-upload"
          >
            Icon File *
          </label>
          <input
            type="file"
            id="icon-file-upload"
            accept=".svg,.png,.jpg,.jpeg,.webp,.gif,image/svg+xml,image/png,image/jpeg,image/webp,image/gif"
            onchange={handleFileSelect}
            class="w-full text-sm text-gray-500 dark:text-gray-400 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 dark:file:bg-blue-900 dark:file:text-blue-200"
          />
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
            Allowed: SVG, PNG, JPEG, WebP, GIF
          </p>
        </div>

        <!-- Name input -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
              for="icon-name-input"
          >
            Name *
          </label>
          <input
            type="text"
            id="icon-name-input"
            bind:value={uploadName}
            placeholder="e.g., Arrow Right"
            class="w-full rounded-md border border-gray-300 dark:border-gray-600 px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <!-- Category input -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
              for="icon-category-input"
          >
            Category *
          </label>
          <input
            type="text"
            id="icon-category-input"
            bind:value={uploadCategory}
            placeholder="e.g., ui, social, business"
            list="category-suggestions"
            class="w-full rounded-md border border-gray-300 dark:border-gray-600 px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <datalist id="category-suggestions">
            {#each categories as cat}
              <option value={cat}>
              </option>
            {/each}
          </datalist>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
            Enter existing category or create a new one
          </p>
        </div>

        <!-- Tags input -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
              for="icon-tags-input"
          >
            Tags
          </label>
          <input
            type="text"
            id="icon-tags-input"
            bind:value={uploadTags}
            placeholder="e.g., arrow, navigation, direction"
            class="w-full rounded-md border border-gray-300 dark:border-gray-600 px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
            Comma-separated list of tags for search
          </p>
        </div>

        <!-- Description input -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
              for="icon-desc-input">
            Description
          </label>
          <textarea
            bind:value={uploadDescription}
            id="icon-desc-input"
            placeholder="Optional description..."
            rows="2"
            class="w-full rounded-md border border-gray-300 dark:border-gray-600 px-3 py-2 text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
          ></textarea>
        </div>
      </div>

      <!-- Dialog actions -->
      <div class="flex justify-end gap-2 mt-6">
        <button
          type="button"
          onclick={closeUploadDialog}
          class="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-md"
        >
          Cancel
        </button>
        <button
          type="button"
          onclick={handleUpload}
          disabled={uploading}
          class="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {uploading ? 'Uploading...' : 'Upload'}
        </button>
      </div>
    </div>
  </div>
{/if}
