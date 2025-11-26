export type CondDef = {
    field_name:     string,
    opr:            string,
    value:          unknown
}

export type QueryResults = {
	status:         boolean;
	error_msg:      string;
    results:        Record<string, unknown> | null;
	loc:            string;
} & Record<string, unknown>
