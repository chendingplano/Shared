import { z } from 'zod';
import type {
	QueryRequest,
	UpdateRequest,
	InsertRequest,
	DeleteRequest,
	JimoResponse,
	JsonObjectOrArray,
	CondDef,
	JoinDef,
	OrderbyDef,
	UpdateDef
} from '$lib/types/CommonTypes';
import { CustomHttpStatus, RequestType } from '$lib/types/CommonTypes';

export type QueryResult = {
	valid: boolean;
	error_msg: string;
	data: JsonObjectOrArray;
};

export const EmptyJimoResponse: JimoResponse = {
	status: false,
	error_msg: '',
	error_code: 0,
	req_id: '',
	result_type: 'none',
	table_name: '',
	base_url: '',
	num_records: 0,
	results: '',
	loc: 'SHD_DST_029'
};

// makeErrorResponse builds a failure JimoResponse. Every error path in this
// file goes through it so the response shape stays consistent.
function makeErrorResponse(
	error_msg: string,
	error_code: number,
	loc: string,
	options: { result_type?: string; table_name?: string } = {}
): JimoResponse {
	return {
		status: false,
		error_msg,
		error_code,
		result_type: options.result_type ?? 'exception',
		table_name: options.table_name ?? '',
		base_url: '',
		num_records: 0,
		results: '',
		loc
	};
}

// checkSystemResp checks the system response codes.
// If it is a system response code, it means the response
// is not a JimoResponse. It constructs a JimoResponse and returns it.
// Otherwise, it returns [false, anything] to let the caller
// parse the JimoResponse.
function checkSystemResp(resp: Response): [boolean, JimoResponse] {
	const httpError = { result_type: 'http_error' };

	if (resp.status === 401) {
		const error_msg = 'Operation rejected (401):' + resp.statusText;
		return [true, makeErrorResponse(error_msg, resp.status, 'SHD_DST_022', httpError)];
	}

	if (resp.status === 404) {
		const error_msg = '404:' + resp.statusText;
		return [true, makeErrorResponse(error_msg, resp.status, 'SHD_DST_034', httpError)];
	}

	// Special handling for 550 status code, which is a custom code
	// returned by our server.
	if (resp.status === CustomHttpStatus.NotLoggedIn) {
		const error_msg = 'User not logged in:' + resp.statusText;
		return [true, makeErrorResponse(error_msg, resp.status, 'SHD_DST_048', httpError)];
	}

	if (resp.status < CustomHttpStatus.Success) {
		// The returned is not a JSON doc. It is returned by the system.
		const error_msg = `Server returned ${resp.statusText}`;
		return [true, makeErrorResponse(error_msg, resp.status, 'SHD_DST_060', httpError)];
	}

	// The response should be a JimoResponse. Let the caller parses it.
	return [false, EmptyJimoResponse];
}

async function handleResp(resp: Response): Promise<JimoResponse> {
	if (!resp.ok) {
		const [resp_generated, jimo_response] = checkSystemResp(resp);
		if (resp_generated) {
			return jimo_response;
		}
	}

	try {
		const resp_json = (await resp.json()) as JimoResponse;
		return resp_json;
	} catch (e) {
		const error_msg =
			e instanceof Error
				? `Server returned ${resp.status} ${resp.statusText} + Error message: ${e.message}`
				: 'Server exception:' + String(e);
		return makeErrorResponse(error_msg, CustomHttpStatus.ServerException, 'SHD_DST_117');
	}
}

// DBStore interfaces with the backend database through GraphQL/HTTP requests.
// The function retrieveRecords implements the SELECT-like operation.
// The function saveRecord implements the INSERT-like operation.
// The function updateRecord implements the UPDATE-like operation.
// The function deleteRecord implements the DELETE-like operation.
class DBStore {
	private home_url: string;
	private data_home: string;
	private file_dir: string;

	constructor() {
		this.home_url = 'http://localhost:8080';
		this.data_home = 'Data';
		this.file_dir = 'CustomerFiles';
	}

	getFileURLByUserId(
		user_id: string,
		filename: string,
		file_prefix: string,
		_token: string
	): string {
		// The URL for a file is composed:
		// '<home_url>/<data_home>/<file_dir>/<user_id>/<filename>
		// If filename starts with 'https://', return it directly
		if (filename.startsWith('https://') || filename.startsWith('http://')) {
			return filename;
		}

		if (typeof filename === 'string' && filename.length > 0) {
			if (file_prefix != '') {
				return `/api/files/${this.data_home}/${this.file_dir}/${user_id}/${file_prefix}_${filename}`;
			} else {
				return `/api/files/${this.data_home}/${this.file_dir}/${user_id}/${filename}`;
			}
		}
		console.log('invalid filename:' + filename);
		return '';
	}

	async retrieveRecords(
		db_name: string,
		table_name: string,
		field_names: string[],
		field_defs: Record<string, unknown>[],
		loc: string,
		conds: CondDef,
		join_def: JoinDef[],
		orderby_def: OrderbyDef[],
		record_schema: unknown | null,
		embed_name: string,
		embed_schema: unknown | null,
		start: number,
		num_records: number
	): Promise<JimoResponse> {
		const req: QueryRequest = {
			request_type: RequestType.Query,
			db_name: db_name,
			table_name: table_name,
			field_defs: field_defs,
			condition: conds,
			join_def: join_def,
			field_names: field_names,
			orderby_def: orderby_def,
			start: start,
			page_size: num_records,
			loc: loc
		};

		try {
			const resp = await fetch('/shared_api/v1/jimo_req', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(req),
				credentials: 'include'
			});

			if (!resp.ok) {
				const [resp_generated, jimo_response] = checkSystemResp(resp);
				if (resp_generated) {
					return jimo_response;
				}
			}

			try {
				const text = await resp.text();
				console.log('Raw response (SHD_DBS_251):', text); // 👈 Look at this!
				const resp_json = JSON.parse(text) as JimoResponse;
				if (resp_json.status) {
					if (resp_json.num_records <= 0) {
						return resp_json;
					}
					if (record_schema) {
						const r_schema = record_schema as z.ZodType;
						if (resp_json.result_type === 'json_array') {
							const results = resp_json.results;
							if (Array.isArray(results)) {
								const valid_records: Record<string, unknown>[] = [];
								for (const record of results) {
									const result = r_schema.safeParse(record as unknown);
									if (result.success) {
										if (embed_schema) {
											const rr = record as Record<string, unknown>;
											if (typeof rr[embed_name] !== 'object') {
												console.warn(
													`Missing/incorrect embedded object (SHD_DBS_280):${embed_name}, type:${typeof rr[embed_name]}`
												);
											} else {
												const e_schema = embed_schema as z.ZodType;
												console.log(`Embed object (SHD_DBS_283):${rr[embed_name]}`);
												const result1 = e_schema.safeParse(rr[embed_name] as unknown);
												if (result1.success) {
													valid_records.push(record as Record<string, unknown>);
												} else {
													console.warn(
														`Embed object validation failed, name:${embed_name} (${loc}:SHD_DBS_275)`,
														{
															errors: result1.error.issues,
															record: result.data
														}
													);
												}
											}
										} else {
											valid_records.push(record as Record<string, unknown>);
										}
									} else {
										console.log('Error 2 (SHD_DBS_293)');
										console.warn(`Object validation failed (${loc}:SHD_DBS_299)`, {
											errors: result.error.issues,
											record: result.data
										});
									}
								}
								resp_json.results = valid_records;
							} else {
								console.warn(
									`Expecting an array but got:${typeof results}, loc:${loc}:ARX_DBS_291)`
								);
							}
						}
					}
				}
				return resp_json;
			} catch (e) {
				const error_msg =
					e instanceof Error
						? `Server returned ${resp.status} ${resp.statusText}, get exception: ${e.message}`
						: 'Error fetching data:' + String(e);
				return makeErrorResponse(error_msg, CustomHttpStatus.ServerException, 'SHD_DST_153', {
					table_name
				});
			}
		} catch (e) {
			const error_msg = 'Error fetching data:' + (e instanceof Error ? e.message : String(e));
			return makeErrorResponse(error_msg, CustomHttpStatus.ServerException, 'SHD_DST_176', {
				table_name
			});
		}
		return makeErrorResponse(
			'Unknown error occurred.',
			CustomHttpStatus.ServerException,
			'SHD_DST_196',
			{
				table_name
			}
		);
	}

	async deleteRecord(
		db_name: string,
		table_name: string,
		cond_def: CondDef,
		field_defs: Record<string, unknown>[],
		delete_single: boolean,
		loc: string
	): Promise<[boolean, number, string]> {
		const req: DeleteRequest = {
			request_type: RequestType.Delete,
			db_name: db_name,
			table_name: table_name,
			condition: cond_def,
			field_defs: field_defs,
			loc: loc
		};

		try {
			const resp = await fetch('/shared_api/v1/jimo_req', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(req),
				credentials: 'include'
			});

			if (!resp.ok) {
				const [resp_generated, jimo_response] = checkSystemResp(resp);
				if (resp_generated) {
					const error_msg = jimo_response.error_msg + ', loc:' + jimo_response.loc;
					return [false, jimo_response.error_code, error_msg];
				}
			}

			try {
				const resp_json = (await resp.json()) as JimoResponse;
				if (resp_json.status) {
					return [true, 200, ''];
				}
				const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${resp_json.error_msg}, loc:${resp_json.loc}`;
				return [false, resp_json.error_code, error_msg];
			} catch (e) {
				if (e instanceof Error) {
					const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${e.message}`;
					return [false, CustomHttpStatus.ServerException, error_msg];
				}

				const error_msg = `Server returned ${resp.status} ${resp.statusText} + Error message: ${String(e)}`;
				return [false, CustomHttpStatus.ServerException, error_msg];
			}
		} catch (e) {
			if (e instanceof Error) {
				const error_msg = 'Error fetching data (SHD_DST_286):' + e.message;
				return [false, CustomHttpStatus.ServerException, error_msg];
			}

			const error_msg = 'Error fetching data (SHD_DST_290):' + e;
			return [false, CustomHttpStatus.ServerException, error_msg];
		}
	}

	async saveRecord(
		db_name: string,
		table_name: string,
		field_defs: Record<string, unknown>[],
		records: Record<string, unknown>[],
		on_conflict_cols: string[],
		on_conflict_update_cols: string[],
		loc: string
	): Promise<JimoResponse> {
		if (records.length <= 0) {
			return makeErrorResponse('record is null', CustomHttpStatus.BadRequest, 'SHD_DST_099', {
				result_type: '',
				table_name
			});
		}

		const req: InsertRequest = {
			request_type: RequestType.Insert,
			db_name: db_name,
			table_name: table_name,
			records: records,
			field_defs: field_defs,
			on_conflict_cols: on_conflict_cols,
			on_conflict_update_cols: on_conflict_update_cols,
			loc: loc
		};

		try {
			const resp = await fetch('/shared_api/v1/jimo_req', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(req),
				credentials: 'include'
			});

			if (!resp.ok) {
				const [resp_generated, jimo_response] = checkSystemResp(resp);
				if (resp_generated) {
					return jimo_response;
				}
			}

			try {
				const resp_json = (await resp.json()) as JimoResponse;
				return resp_json;
			} catch (e) {
				const error_msg =
					e instanceof Error
						? `Server returned ${resp.status} ${resp.statusText} + Error message: ${e.message}`
						: 'Error fetching data:' + String(e);
				return makeErrorResponse(error_msg, CustomHttpStatus.ServerException, 'SHD_DST_453', {
					table_name
				});
			}
		} catch (e) {
			const error_msg = 'Error fetching data:' + (e instanceof Error ? e.message : String(e));
			return makeErrorResponse(error_msg, CustomHttpStatus.ServerException, 'SHD_DST_477', {
				table_name
			});
		}
		return makeErrorResponse(
			'Unknown error occurred.',
			CustomHttpStatus.ServerException,
			'SHD_DST_491',
			{
				table_name
			}
		);
	}

	// updateRecord updates a record. The fields to be updated
	// is in 'update_entries'.
	async updateRecord(
		db_name: string,
		table_name: string,
		field_defs: Record<string, unknown>[],
		condition: CondDef,
		record: Record<string, unknown>,
		update_entries: UpdateDef[],
		on_conflict_cols: string[],
		on_conflict_update_cols: string[],
		need_record: boolean,
		loc: string
	): Promise<JimoResponse> {
		const req: UpdateRequest = {
			request_type: RequestType.Update,
			db_name: db_name,
			table_name: table_name,
			field_defs: field_defs,
			condition: condition,
			record: record,
			update_entries: update_entries,
			on_conflict_cols: on_conflict_cols,
			on_conflict_update_cols: on_conflict_update_cols,
			need_record: need_record,
			loc: loc
		};

		try {
			const resp = await fetch('/shared_api/v1/jimo_req', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(req),
				credentials: 'include'
			});

			const resp_json = await handleResp(resp);
			return resp_json;
		} catch (e) {
			const error_msg = `Error fetching data: ${e instanceof Error ? e.message : e}`;
			return makeErrorResponse(error_msg, CustomHttpStatus.ServerException, 'SHD_DST_464', {
				table_name
			});
		}
	}

	async getToken(): Promise<[string, number, string]> {
		// Fail closed: callers must not mistake the unimplemented stub for a real token.
		console.error('getToken not implemented yet (SHD_DST_641)');
		return ['', CustomHttpStatus.NotImplementedYet, 'NotImplementedYet'];
	}
}

// ✅ Create and export a SINGLE instance
export const db_store = new DBStore();
