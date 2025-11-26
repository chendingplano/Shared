import type { JsonObjectOrArray } from "../types/CommonTypes";
export declare function SafeJsonParseAsObject(input: string): {
    [key: string]: unknown;
} | null;
export declare function ParseObjectOrArray(input: string): JsonObjectOrArray | null;
export declare function GetAllKeys(generic_map: {
    [key: string]: unknown;
}): string[];
