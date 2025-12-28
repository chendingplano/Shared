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

export function IsValidNonEmptyString(str: unknown): str is string {
    return typeof str === 'string' && str.length > 0;
}

export async function ParseConfigFile(tomlContent: string): Promise<[Record<string, unknown>, string]> {
    try {
        const TOML = await import('@iarna/toml')
        const parsed = TOML.parse(tomlContent) as Record<string, unknown>;
        
        // Validate required fields
        if (!parsed.app_name || !parsed.home_url || !parsed.server) {
            const error_msg = 'Missing required configuration fields';
            return [{}, error_msg]
        }
        
        if (typeof parsed.debug !== 'boolean') {
            parsed.debug = Boolean(parsed.debug);
        }
        
        return [parsed, ""];
    } catch (error) {
            const error_msg = `Failed to parse config file: ${error instanceof Error ? error.message : 'Unknown error'}`;
            return [{}, error_msg]
    }
}

// Example usage:
// const configFileContent = await Deno.readTextFile('./config.toml'); // For Deno
// Or for Node.js:
// const fs = require('fs');
// const configFileContent = fs.readFileSync('./config.toml', 'utf-8');

// const config = parseConfigFile(configFileContent);
// console.log(config.app_name); // "Mirai"