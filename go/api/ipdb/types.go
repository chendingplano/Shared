package ipdb

import "time"

// IPRecord holds the geolocation result for a single IP address.
type IPRecord struct {
	IP            string    `json:"ip"`
	ASNNumber     uint      `json:"asn_number"`
	ASNOrg        string    `json:"asn_org"`
	CountryName   string    `json:"country_name"`
	CountryISO    string    `json:"country_iso"`
	ContinentName string    `json:"continent_name"`
	ContinentCode string    `json:"continent_code"`
	LookedUpAt    time.Time `json:"looked_up_at"`
}

// mmdbRecord mirrors the structure of the ip66.mmdb file.
type mmdbRecord struct {
	ASN struct {
		Number       uint   `maxminddb:"number"`
		Organization string `maxminddb:"organization"`
	} `maxminddb:"asn"`
	Country struct {
		Name    string `maxminddb:"name"`
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	Continent struct {
		Name string `maxminddb:"name"`
		Code string `maxminddb:"code"`
	} `maxminddb:"continent"`
}

// SyncStatus describes the last database synchronisation event.
type SyncStatus struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`   // "success" | "failure"
	FileSize  int64     `json:"file_size"` // bytes downloaded
	ErrorMsg  string    `json:"error_msg,omitempty"`
	SyncedAt  time.Time `json:"synced_at"`
}
