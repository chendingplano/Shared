import type { JsonObjectOrArray } from "$lib/types/CommonTypes";

export function SafeJsonParseAsObject(input: string): { [key: string]: unknown } | null {
    // This function parses input string into a generic JSON object ({[key: string]: unknown})
    // Note that 
    try {
        const value = JSON.parse(input);
        return (
            typeof value === 'object' &&
            value !== null &&
            !Array.isArray(value)
        ) ? value as { [key: string]: unknown } : null;
    } catch {
        return null;
    }
}

export function ParseObjectOrArray(input: string): JsonObjectOrArray | null {
    // The difference between this function and SafeJsonParseAsObject(...)
    // Is that this function accepts JSON arrays (unknown[]).
    try {
        const value = JSON.parse(input);
        if ((typeof value === 'object' && value !== null)) { // includes arrays & objects
            return value as JsonObjectOrArray;
        }
        return null; // reject primitives like "hello", 42, true
    } catch {
        return null;
    }
}

export function GetAllKeys(generic_map: { [key: string]: unknown }): string[] {
  return Object.keys(generic_map);
}