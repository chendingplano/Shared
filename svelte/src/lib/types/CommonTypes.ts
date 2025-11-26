export interface EmailSignupResponse {
    message: string;
    loc:     string;
}

export type JimoRequest = {
    request_type:   string;
    action:         string;
    resource_name:  string;
    resource_opr:   string;
    conditions:     string;
    resource_info:  string;
} & Record<string, unknown>;

export type JimoResponse = {
	status:         boolean;
	error_msg:      string;
    result_type:    string;
    results:        string;
	loc:            string;
} & Record<string, unknown>

export type JsonObjectOrArray = { [key: string]: unknown } | unknown[];