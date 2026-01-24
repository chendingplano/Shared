import type { JimoResponse } from '$lib/types/CommonTypes';
import type { IconDef, IconUploadRequest, IconListRequest, IconListResponse } from '$lib/types/IconTypes';

const BASE_URL = '/shared_api/v1/icons';

/**
 * IconStore provides API client functions for icon operations
 */
class IconStore {
  /**
   * List icons with optional filters and pagination
   */
  async listIcons(options?: IconListRequest): Promise<IconListResponse> {
    const params = new URLSearchParams();
    if (options?.category) params.set('category', options.category);
    if (options?.search) params.set('search', options.search);
    params.set('page', String(options?.page ?? 0));
    params.set('page_size', String(options?.page_size ?? 50));

    const resp = await fetch(`${BASE_URL}?${params}`, {
      credentials: 'include',
    });

    const json = await resp.json() as JimoResponse;
    if (!json.status) {
      throw new Error(json.error_msg || 'Failed to list icons');
    }

    return {
      icons: json.results as IconDef[],
      total: json.num_records,
    };
  }

  /**
   * Get a single icon by ID
   */
  async getIcon(id: string): Promise<IconDef> {
    const resp = await fetch(`${BASE_URL}/${id}`, {
      credentials: 'include',
    });

    const json = await resp.json() as JimoResponse;
    if (!json.status) {
      throw new Error(json.error_msg || 'Failed to get icon');
    }

    return json.results as unknown as IconDef;
  }

  /**
   * Upload a new icon
   */
  async uploadIcon(file: File, metadata: IconUploadRequest): Promise<IconDef> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('name', metadata.name);
    formData.append('category', metadata.category);
    formData.append('tags', JSON.stringify(metadata.tags ?? []));
    if (metadata.description) {
      formData.append('description', metadata.description);
    }

    const resp = await fetch(BASE_URL, {
      method: 'POST',
      body: formData,
      credentials: 'include',
    });

    const json = await resp.json() as JimoResponse;
    if (!json.status) {
      throw new Error(json.error_msg || 'Failed to upload icon');
    }

    return json.results as unknown as IconDef;
  }

  /**
   * Delete an icon by ID
   */
  async deleteIcon(id: string): Promise<void> {
    const resp = await fetch(`${BASE_URL}/${id}`, {
      method: 'DELETE',
      credentials: 'include',
    });

    const json = await resp.json() as JimoResponse;
    if (!json.status) {
      throw new Error(json.error_msg || 'Failed to delete icon');
    }
  }

  /**
   * Get all distinct categories
   */
  async getCategories(): Promise<string[]> {
    const resp = await fetch(`${BASE_URL}/categories`, {
      credentials: 'include',
    });

    const json = await resp.json() as JimoResponse;
    if (!json.status) {
      throw new Error(json.error_msg || 'Failed to get categories');
    }

    return json.results as string[];
  }

  /**
   * Get the URL for an icon file
   */
  getIconUrl(icon: IconDef): string {
    return `${BASE_URL}/file/${encodeURIComponent(icon.category)}/${encodeURIComponent(icon.file_name)}`;
  }

  /**
   * Get the URL for an icon file by category and filename
   */
  getIconUrlByPath(category: string, filename: string): string {
    return `${BASE_URL}/file/${encodeURIComponent(category)}/${encodeURIComponent(filename)}`;
  }
}

// Export singleton instance
export const iconStore = new IconStore();
