export type RecordIdString = string
export type IsoAutoDateString = string & { readonly autodate: unique symbol }

export enum DBNames {
	DB_Tax 			= "tax",
	DB_DeepDoc 		= "miner"
}

export enum Collections {
    Users = "users"
}

type ExpandType<T> = unknown extends T
    ? { expand?: unknown }
    : { expand: T }

export type BaseSystemFields<T = unknown> = {
	id: RecordIdString
	collectionId: string
	collectionName: Collections
} & ExpandType<T>

export type AuthSystemFields<T = unknown> = {
	email: string
	emailVisibility: boolean
	username: string
	verified: boolean
} & BaseSystemFields<T>
