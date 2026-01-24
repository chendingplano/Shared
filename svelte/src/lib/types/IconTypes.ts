import type { IsoAutoDateString } from "./DatabaseTypes";

// Make sure it syncs with shared/go/api/icons/icons_types.go::IconDef
export interface IconDef {
  id: string;
  name: string;
  category: string;
  file_name: string;
  file_path: string;
  mime_type: string;
  file_size: number;
  width?: number;
  height?: number;
  tags: string[];
  description?: string;
  creator: string;
  updater: string;
  created_at: IsoAutoDateString;
  updated_at: IsoAutoDateString;
}

// Make sure it syncs with shared/go/api/icons/icons_types.go::IconUploadRequest
export interface IconUploadRequest {
  name: string;
  category: string;
  tags: string[];
  description?: string;
}

// Make sure it syncs with shared/go/api/icons/icons_types.go::IconUpdateRequest
export interface IconUpdateRequest {
  name?: string;
  category?: string;
  tags?: string[];
  description?: string;
}

// Make sure it syncs with shared/go/api/icons/icons_types.go::IconListRequest
export interface IconListRequest {
  category?: string;
  search?: string;
  page?: number;
  page_size?: number;
}

// Response wrapper for icon list operations
export interface IconListResponse {
  icons: IconDef[];
  total: number;
}

// Allowed MIME types for icon uploads (synced with Go)
export const AllowedMimeTypes = [
  'image/svg+xml',
  'image/png',
  'image/jpeg',
  'image/webp',
  'image/gif'
] as const;

export type AllowedMimeType = typeof AllowedMimeTypes[number];

export function isAllowedMimeType(mimeType: string): mimeType is AllowedMimeType {
  return AllowedMimeTypes.includes(mimeType as AllowedMimeType);
}
