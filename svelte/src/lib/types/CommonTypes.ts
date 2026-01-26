import type { IsoAutoDateString, IsoDateString } from "./DatabaseTypes";

export interface EmailSignupResponse {
    message: string;
    loc:     string;
}

export type FileNameString = string & { readonly filename: unique symbol }

// Make sure it syncs with go/api/ApiTypes/enums.go:
export enum RequestType {
    Insert      = "insert",
    Update      = "update",
    Delete      = "delete",
    Query       = "query"
}

type CondOperator = '=' | '<>' | '>' | '>=' | '<' | '<=' | 'contain' | 'prefix';

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::FieldDef
export type FieldDef = {
	field_name:     string,
	data_type:      string,
	required:       boolean,
	read_only?:     boolean,
	element_type?:  string,
	desc?:          string,
}

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::UpdateDef
export type UpdateDef = {
    field_name:     string,
    data_type:      string,
    value:          unknown
}

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::OnClauseDef
export interface OnClauseDef {
    source_field_name:  string;
    joined_field_name:  string;
    join_opr:           string;
    data_type:          string;
}

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::JoinDef
export interface JoinDef {
    from_table_name:    string;
    joined_table_name:  string;
    from_field_defs:    FieldDef[];
    joined_field_defs:  FieldDef[];
    on_clause:          OnClauseDef[];
    join_type:          string;
    selected_fields:    string[];
    embed_name?:        string;
}

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::OrderbyDef
export interface OrderbyDef {
    field_name:         string;
    data_type:          string;
    is_asc:             boolean;
}

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go
export interface AtomicCondition {
    type:           'atomic';
    field_name:     string;
    opr:            CondOperator;
    value:          any;
    data_type:      string;
}

export interface NullCondition {
    type:           'null';
}

export interface GroupCondition {
    type:           'and' | 'or';
    conditions:     CondDef[];
}

export type CondDef = AtomicCondition | GroupCondition | NullCondition;

export type UpdateWithCondDef = {
    condition:      CondDef[],
    updates:        UpdateDef[]
}

export type JimoRequest = {
    request_type:   string
}

export type QueryResults = {
	status:         boolean;
	error_msg:      string;
    results:        Record<string, unknown> | null;
	loc:            string;
} & Record<string, unknown>

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::DeleteRequest
export type DeleteRequest = {
    request_type:   string;
    db_name:        string;
    table_name:     string;
    condition:      CondDef;
    field_defs?:    Record<string, unknown>[];
    loc:            string;
};

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::QueryRequest
export type QueryRequest = {
    request_type:   string;
    db_name:        string;
    table_name:     string;
    condition:      CondDef;
    join_def:      JoinDef[];
    field_defs:     Record<string, unknown>[];
    field_names:    string[];
    orderby_def:    OrderbyDef[];
    start:          number;
    page_size:      number;
    loc:            string;
};

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::InsertRequest
export type InsertRequest = {
    request_type:               string;
    db_name:                    string;
    table_name:                 string;
    records:                    Record<string, unknown>[];
    field_defs:                 Record<string, unknown>[];
    on_conflict_cols:           string[];
    on_conflict_update_cols:    string[];
    loc:                        string;
};

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::UpdateRequest
export type UpdateRequest = {
    request_type:               string;
    db_name:                    string;
    table_name:                 string;
    condition:                  CondDef;
    record:                     Record<string, unknown>,
    update_entries:             UpdateDef[];
    field_defs?:                Record<string, unknown>[];
    on_conflict_cols:           string[];
    on_conflict_update_cols:    string[];
    need_record:                boolean;
    loc:                        string;
};

export type JsonObjectOrArray = { [key: string]: unknown } | unknown[];

// Make sure it syncs with go/api/ApiTypes/ApiTypes.go::JimoResponse
export type JimoResponse = {
	status:         boolean;
	error_msg:      string;
    req_id?:        string;
    result_type:    string;
    error_code:     number;
    table_name:     string;
    base_url:       string;
    num_records:    number;
    results:        JsonObjectOrArray | string;
	loc:            string;
}


// Make sure sync the changes to Shared/go/api/ApiTypes/ApiTypes.go
export const CustomHttpStatus = {
    Success:                550,
    ResourceNotFound:       551,
    BadRequest:             552,
    NotImplementedYet:      553,
    InternalError:          554,
    ServerException:        555,
    KeyNotUnique:           556,
    NotLoggedIn:            557,
} as const; 

export type ResourceDef = {
    resource_name:      string;
    resource_type:      string;
    action:             string;
    db_name:            string;
    table_name:         string;
    description?:       string;
    field_defs:         Record<string, unknown>[]
} & Record<string, unknown>;

// Make sure this struct syncs with shared/go/api/ApiTypes/ApiTypes.go::UserInfo
// SECURITY: Sensitive fields (password, tokens) are excluded from API responses via json:"-" in Go
// These fields are intentionally NOT included in the TypeScript type as they should never be sent to the client
export type UserInfo = {
	id:                 string
	name?:              string
	// password - EXCLUDED: Never sent to client (json:"-" in Go)
	user_id_type?:      string
	first_name?:        string
	last_name?:         string
	email:              string
	user_mobile?:       string
	user_address?:      string
	verified?:          boolean
	admin?:             boolean
	is_owner?:          boolean
	email_visibility?:  boolean
	auth_type?:         string
	user_status?:       string
	avatar?:            FileNameString
	locale?:            string
	// outlook_refresh_token - EXCLUDED: Never sent to client (json:"-" in Go)
	// outlook_access_token - EXCLUDED: Never sent to client (json:"-" in Go)
	outlook_token_expires_at?: IsoDateString
	outlook_sub_id?:    string
	outlook_sub_expires_at?: IsoDateString
	// v_token - EXCLUDED: Never sent to client (json:"-" in Go)
	v_token_expires_at?: IsoDateString
	created?:           IsoAutoDateString
	updated?:           IsoAutoDateString
}