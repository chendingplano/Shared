import { 
    type JimoResponse, 
    type CondDef,
    type UpdateDef,
    cond_builder,
    orderby_builder} from '@chendingplano/shared';
import { db_store } from '@chendingplano/shared';
import { CustomHttpStatus, type ResourceDef, type UserInfo } from "../types/CommonTypes";
import { type IsoAutoDateString,
         type AuthSystemFields,
         DBNames,
         Collections,
} from "../types/DatabaseTypes"
import { StatusCodes } from 'http-status-codes'

export const TableUsersFieldDefs: Record<string, unknown>[] = 
    [
        {
            "field_name": "user_id",
            "required": true,
            "data_type": "string",
            "element_type": "",
            "read_only": true,
        },
        {
            "field_name": "user_name",
            "required": true,
            "data_type": "string",
            "read_only": true,
        },
        {
            "field_name": "user_password",
            "required": false,
            "data_type": "string",
            "read_only": true,
        },
        {
            "field_name": "user_id_type",
            "required": false,
            "data_type": "string",
            "read_only": true,
        },
        {
            "field_name": "firstName",
            "required": false,
            "data_type": "string",
            "read_only": false,
        },
        {
            "field_name": "lastName",
            "required": false,
            "data_type": "string",
            "element_type": "",
            "read_only": true,
        },
        {
            "field_name": "email",
            "required": true,
            "data_type": "string",
            "read_only": false,
        },
        {
            "field_name": "user_mobile",
            "required": false,
            "data_type": "string",
            "read_only": false,
        },
        {
            "field_name": "user_address",
            "required": false,
            "data_type": "string",
            "element_type": "",
            "read_only": false,
        },
        {
            "field_name": "verified",
            "required": false,
            "data_type": "bool",
            "read_only": true,
            "element_type": ""
        },
        {
            "field_name": "is_admin",
            "required": false,
            "read_only": false,
            "data_type": "bool",
        },
        {
            "field_name": "emailVisibility",
            "required": false,
            "read_only": false,
            "data_type": "bool",
        },
        {
            "field_name": "user_type",
            "required": true,
            "read_only": true,
            "data_type": "string",
        },
        {
            "field_name": "user_status",
            "required": true,
            "read_only": true,
            "data_type": "string",
        },
        {
            "field_name": "avatar",
            "required": false,
            "read_only": false,
            "data_type": "string",
        },
        {
            "field_name": "locale",
            "required": false,
            "read_only": false,
            "data_type": "string",
        },
        {
            "field_name": "userToken",
            "required": false,
            "read_only": true,
            "data_type": "string",
        },
        {
            "field_name": "created",
            "required": false,
            "read_only": true,
            "data_type": "timestamp",
        },
        {
            "field_name": "updated",
            "required": false,
            "read_only": false,
            "data_type": "timestamp",
        },
    ]

export const nowIso = (): IsoAutoDateString =>
  	new Date().toISOString() as IsoAutoDateString;

export type UsersResponse<Texpand = unknown> = Required<UserInfo> & AuthSystemFields<Texpand>

export const UsersAllFieldNames = [
    "user_id", "user_name", "user_password", "user_id_type", "firstName", 
    "lastName", "email", "user_mobile", "user_address", "verified",
    "is_admin", "emailVisibility", "user_type", "user_status", "avatar",
    "locale", "userToken", "created", "updated"
]

export function IsUserInfoRecord(record: Record<string, unknown>): boolean {
    return (typeof record.user_id           === 'string' &&
            typeof record.user_name         === 'string' &&
            typeof record.user_password     === 'string' &&
            typeof record.email             === 'string' &&
            typeof record.user_type         === 'string' &&
            typeof record.user_status       === 'string')
}

export async function RetrieveUsers<T=UserInfo>(
        loc: string,
        alert_on_error: boolean): Promise<[T[], string]> {
    const orderby_def = orderby_builder.start()
                                       .orderby("name", "string", true)
                                       .build()
    // TBD: order-by: name
    const cond_def = cond_builder.null()
    const response = await db_store.retrieveRecords(
        DBNames.DB_Tax,
        Collections.Users,
        UsersAllFieldNames, 
        TableUsersFieldDefs, 
        loc, 
        cond_def, 
        [], 
        orderby_def,
        0, 200) as JimoResponse

    if (!response.status) {
        const error_msg = "Failed retrieving users (MRX_USR_035), error_msg:" + 
            response.error_msg + 
            ", error code:" + response.error_code + 
            ", loc:" + response.loc +
            ":" + loc
        if (alert_on_error) {
            alert(error_msg);
        } else {
            console.warn(error_msg)
        }
        return [[], error_msg];
    }

    if (!Array.isArray(response.records)) {
        const error_msg = "Expected array in response (" + 
            ", loc:" + response.loc + ":" + loc + ")"
        if (alert_on_error) {
            alert(error_msg);
        } else {
            console.warn(error_msg);
        }
        return [[], error_msg];
    }

    // Filter and validate each record.
    // IMPORTANT: make sure the check is synced with QuestionnaireTemplatesRecord!!! 
    const validRecords: T[] = [];
    for (const item of response.records) {
        const record = item as Record<string, unknown>;
        if (IsUserInfoRecord(record)) {
            validRecords.push(item as T);
        } else {
            console.warn("Skipping invalid users record (MRX_USR_142):", item);
        }
    }
    return [validRecords, ""];
}

export async function CreateUsersRecord(
        record: Record<string, unknown>, 
        loc: string, 
        alert_on_error: boolean) : Promise<[UserInfo | null, number, string]> {

    if (!IsUserInfoRecord(record)) {
        const error_msg = "not a valid UserInfo record (MRX_USR_150)"
        return [null, CustomHttpStatus.BadRequest, error_msg]
    }        

    const response = await db_store.saveRecord(
            DBNames.DB_Tax,
            Collections.Users,
            TableUsersFieldDefs,
            [record],
            [],
            [],
            "ARX_USR_162")
    if (response.status) {
        const rr = record as unknown
        return [rr as UserInfo, StatusCodes.OK, ""]
    }

    const error_msg = `failed creating record, error:${response.error_msg}, loc:${response.loc}`
    return [null, response.error_code, error_msg]
}

export async function UpdateUsersRecord(
        user_id: string, 
        record: Record<string, unknown>,
        update_entries: UpdateDef[],
        loc: string, 
        alert_on_error: boolean) : Promise<[UserInfo | null, number, string]> {
    const cond_def = cond_builder.filter()
                                 .condEq("user_id", user_id, "string")
                                 .build()

    const resp = await db_store.updateRecord(
            DBNames.DB_Tax,
            Collections.Users,
            TableUsersFieldDefs,
            cond_def,
            record,
            update_entries,
            [],
            [],
            true,
            "ARX_CLT_191")

    if (resp.status) {
        if (resp.result_type === "json") {
            const user_record = resp.records
            if (typeof user_record === "object") {
                if (IsUserInfoRecord(user_record as Record<string, unknown>)) {
                    const rr = user_record as unknown
                    return [rr as UserInfo, StatusCodes.OK, ""]
                }
            }
        }
        const error_msg = "record updated but did not return the record correctly (SHD_USR_302)"
        return [null, CustomHttpStatus.InternalError, error_msg]
    }

    const error_msg = `failed updating checklistItems Record, error:${resp.error_msg}, loc:${resp.loc}`
    return [null, resp.error_code, error_msg]
}

export async function DeleteChecklistItemsRecord(
        db_name: string,
        id: string,
        loc: string,
        alert_on_error: boolean): Promise<[boolean, number, string]> {

    const cond_def = cond_builder.filter()
                                 .condEq("user_id", id, "string")
                                 .build()
    return await db_store.deleteRecord(
            db_name,
            Collections.Users,
            cond_def,
            TableUsersFieldDefs,
            true,
            "SHR_USR_318")
}