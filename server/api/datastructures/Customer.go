package datastructures

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type CustomTime struct {
    time.Time
}

// Implement driver.Valuer interface for database operations
func (ct CustomTime) Value() (driver.Value, error) {
    return ct.Time, nil  // Return the underlying time.Time
}

// Implement sql.Scanner interface for database operations  
func (ct *CustomTime) Scan(value interface{}) error {
    if value == nil {
        ct.Time = time.Time{}
        return nil
    }
    
    switch v := value.(type) {
    case time.Time:
        ct.Time = v
        return nil
    case string:
        t, err := time.Parse("2006-01-02", v)
        if err != nil {
            t, err = time.Parse(time.RFC3339, v)
            if err != nil {
                return err
            }
        }
        ct.Time = t
        return nil
    case []byte:
        t, err := time.Parse("2006-01-02", string(v))
        if err != nil {
            t, err = time.Parse(time.RFC3339, string(v))
            if err != nil {
                return err
            }
        }
        ct.Time = t
        return nil
    default:
        return fmt.Errorf("cannot scan %T into CustomTime", value)
    }
}


func (ct *CustomTime) UnmarshalJSON(b []byte) error {
    s := string(b[1 : len(b)-1]) // Remove quotes
    
   	// Try RFC3339 first
    if t, err := time.Parse(time.RFC3339, s); err == nil {
        ct.Time = t
        return nil
    }
    
    // Then try YYYY-MM-DD
    if t, err := time.Parse("2006-01-02", s); err == nil {
        ct.Time = t
        return nil
    }
    
    // Add other formats as needed
    return fmt.Errorf("unable to parse time: %s", s)
}

func (ct CustomTime) MarshalJSON() ([]byte, error) {
    return json.Marshal(ct.Format("2006-01-02"))
}

type AosCustomer struct {
    ID           int        `json:"id,omitempty"`
    UserName     string     `json:"userName"`
    DateOfBirth  CustomTime `json:"dateOfBirth"`
    Email        string     `json:"email"`
    PhoneNumber  string     `json:"phoneNumber"`
    Education    string     `json:"education"`
    IsMarried    bool       `json:"isMarried"`
    NumberOfKids int        `json:"numberOfKids"`
    CreatedAt    CustomTime `json:"created_at"`
}

