export function SafeJsonParseAsObject(input) {
    // This function parses input string into a generic JSON object ({[key: string]: unknown})
    // Note that 
    try {
        const value = JSON.parse(input);
        return (typeof value === 'object' &&
            value !== null &&
            !Array.isArray(value)) ? value : null;
    }
    catch {
        return null;
    }
}
export function ParseObjectOrArray(input) {
    // The difference between this function and SafeJsonParseAsObject(...)
    // Is that this function accepts JSON arrays (unknown[]).
    try {
        const value = JSON.parse(input);
        if ((typeof value === 'object' && value !== null)) { // includes arrays & objects
            return value;
        }
        return null; // reject primitives like "hello", 42, true
    }
    catch {
        return null;
    }
}
export function GetAllKeys(generic_map) {
    return Object.keys(generic_map);
}
