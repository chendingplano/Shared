package RequestHandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

func CreateValueGroupsPG(
			user_name string,
			fieldDefs []ApiTypes.FieldDef,
			chunk []map[string]interface{}) ([]string, []interface{}, error) {
	paramCounter := 1
	valueGroups := []string{}
	args := []interface{}{}
	for _, rec := range chunk {
		placeholders := []string{}
		for _, f := range fieldDefs {
			log.Printf("Check field:%s, type:%s (SHD_DUP_022)", f.FieldName, f.DataType)
			val, ok := rec[f.FieldName]
			
			switch f.DataType {
			case "_creator":
				// Add the user_name value for creator fields
				log.Printf("Set creator:%s, field name:%s (SHD_DUP_064)", user_name, f.FieldName)
				args = append(args, user_name)
				placeholders = append(placeholders, fmt.Sprintf("$%d", paramCounter))
				paramCounter++
				
			case "_updater":
				// Add the user_name value for updater fields
				val = user_name
				log.Printf("Set updater:%s, field name:%s (SHD_DUP_072)", user_name, f.FieldName)
				args = append(args, val)
				placeholders = append(placeholders, fmt.Sprintf("$%d", paramCounter))
				paramCounter++
				
			case "_ignore":
				// Skip this field entirely - don't add to args or placeholders
				log.Printf("Field ignored:%s, field name:%s (SHD_DUP_079)", user_name, f.FieldName)
				continue // Skip to next field
				
			case "_auto_inc":
				// Skip auto increment field - don't add to args or placeholders
				log.Printf("auto-increment field:%s, ignored (SHD_DUP_084)", f.FieldName)
				continue // Skip to next field

			case "array":
				 log.Printf("processing array-string field:%s (SHD_DUP_095)", f.FieldName)
    			 if f.Required && !ok {
        			// Handle missing array field - could be nil or empty array depending on your needs
        			args = append(args, pq.Array([]string{})) // or nil if allowed
        			placeholders = append(placeholders, fmt.Sprintf("$%d", paramCounter))
        			paramCounter++
    			 } else {
					handleArrayValue(f, val, &args, &placeholders, &paramCounter)
    			}
				
			default:
				if f.Required && !ok {
					log.Printf("missing required field:%s, field_type:%s (SHD_DUP_088)", f.FieldName, f.DataType)
					return valueGroups, args, fmt.Errorf("missing required field: %s", f.FieldName)
				}
				log.Printf("FieldDef:%v (SHD_DUP_073)", f)
				handleValue(f.DataType, val, &args, &placeholders, &paramCounter)
			}
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ",")+")")
	}

	return valueGroups, args, nil
}

func handleValue(
		db_field_data_type string,
		value interface{},
		args *[]interface{},
		placeholders *[]string,
		paramCount *int) error {
	// This function appends 'value' to 'args', add a placeholder to 'placeholder' and
	// increment 'paramCount'. It must match the data type of 'value' with the database field 
	// data type 'db_field_data_type'.

	log.Printf("------------handle value (SHD_DUP_092), data_field_type:%s, value:%v", db_field_data_type, value)
    switch val := value.(type) {
    case string:
		log.Printf("------------handle value (SHD_DUP_096), value type:string")
        switch db_field_data_type {
        case "text", "varchar", "char", "string":
             *args = append(*args, val)
             *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
             *paramCount++
			 log.Printf("------------string value (SHD_DUP_099), placeholder:%s, args:%s, paramcount:%d", 
			 	*placeholders, *args, *paramCount)
             return nil

        case "integer", "int", "int4":
             if num, err := strconv.Atoi(val); err == nil {
                *args = append(*args, num)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to integer", val)

        case "bigint", "int8":
             if num, err := strconv.ParseInt(val, 10, 64); err == nil {
                *args = append(*args, num)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to bigint", val)

        case "smallint", "int2":
             if num, err := strconv.ParseInt(val, 10, 16); err == nil {
                *args = append(*args, int16(num))
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to smallint", val)

        case "real", "float4":
             if num, err := strconv.ParseFloat(val, 32); err == nil {
                *args = append(*args, float32(num))
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to real", val)

        case "double precision", "float8":
             if num, err := strconv.ParseFloat(val, 64); err == nil {
                *args = append(*args, num)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to double precision", val)

        case "boolean", "bool":
             if lower := strings.ToLower(val); lower == "true" || lower == "1" || lower == "t" {
                *args = append(*args, true)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             } else if lower == "false" || lower == "0" || lower == "f" {
                *args = append(*args, false)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to boolean", val)

        case "date", "timestamp", "timestamptz":
             if parsed, err := time.Parse("2006-01-02", val); err == nil {
                *args = append(*args, parsed)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             } else if parsed, err := time.Parse("2006-01-02 15:04:05", val); err == nil {
                *args = append(*args, parsed)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             } else if parsed, err := time.Parse(time.RFC3339, val); err == nil {
                *args = append(*args, parsed)
                *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
                *paramCount++
                return nil
             }
             return fmt.Errorf("cannot convert string '%s' to timestamp", val)

        case "text[]", "varchar[]", "string[]":
			 // If the string represents a JSON array like '["item1", "item2"]'
        	 var stringArray []string
         	 if err := json.Unmarshal([]byte(val), &stringArray); err == nil {
            	*args = append(*args, pq.Array(stringArray))
        	 } else {
            	// If it's not a JSON array, treat as single-element array
            	*args = append(*args, pq.Array([]string{val}))
        	 }
        	 *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
         	 *paramCount++
             return nil

        default:
             return fmt.Errorf("unsupported database field type '%s' for string value", db_field_data_type)
        }

    case int:
		log.Printf("------------handle value (SHD_DUP_202), value type:int")
        switch db_field_data_type {
        case "integer", "int", "int4":
            *args = append(*args, val)
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "bigint", "int8":
            *args = append(*args, int64(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "smallint", "int2":
            if val < math.MinInt16 || val > math.MaxInt16 {
                return fmt.Errorf("integer value %d out of range for smallint", val)
            }
            *args = append(*args, int16(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "real", "float4":
            *args = append(*args, float32(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "double precision", "float8":
            *args = append(*args, float64(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "text", "varchar", "char", "string":
            *args = append(*args, strconv.Itoa(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        default:
            return fmt.Errorf("unsupported database field type '%s' for int value", db_field_data_type)
        }

    case int64:
		log.Printf("------------handle value (SHD_DUP_248), value type:int64")
        switch db_field_data_type {
        case "bigint", "int8":
            *args = append(*args, val)
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "integer", "int", "int4":
            if val < math.MinInt32 || val > math.MaxInt32 {
                return fmt.Errorf("bigint value %d out of range for integer", val)
            }
            *args = append(*args, int32(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "smallint", "int2":
            if val < math.MinInt16 || val > math.MaxInt16 {
                return fmt.Errorf("bigint value %d out of range for smallint", val)
            }
            *args = append(*args, int16(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "real", "float4":
            *args = append(*args, float32(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "double precision", "float8":
            *args = append(*args, float64(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "text", "varchar", "char", "string":
            *args = append(*args, strconv.FormatInt(val, 10))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        default:
            return fmt.Errorf("unsupported database field type '%s' for int64 value", db_field_data_type)
        }

    case float64: // JSON numbers are typically float64
		log.Printf("------------handle value (SHD_DUP_297), value type:float64")
        switch db_field_data_type {
        case "double precision", "float8":
            *args = append(*args, val)
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "real", "float4":
            *args = append(*args, float32(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "integer", "int", "int4":
            if val < math.MinInt32 || val > math.MaxInt32 {
                return fmt.Errorf("float value %f out of range for integer", val)
            }
            *args = append(*args, int32(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "bigint", "int8":
            if val < math.MinInt64 || val > math.MaxInt64 {
                return fmt.Errorf("float value %f out of range for bigint", val)
            }
            *args = append(*args, int64(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "text", "varchar", "char", "string":
            *args = append(*args, strconv.FormatFloat(val, 'g', -1, 64))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        default:
            return fmt.Errorf("unsupported database field type '%s' for float64 value", db_field_data_type)
        }

    case bool:
		log.Printf("------------handle value (SHD_DUP_340), value type:bool")
        switch db_field_data_type {
        case "boolean", "bool":
            *args = append(*args, val)
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "text", "varchar", "char", "string":
            *args = append(*args, strconv.FormatBool(val))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        default:
            return fmt.Errorf("unsupported database field type '%s' for bool value", db_field_data_type)
        }

    case nil:
		log.Printf("------------handle value (SHD_DUP_359), value type:nil")
        *args = append(*args, nil)
        *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
        *paramCount++
        return nil

    case []interface{}:
		log.Printf("------------handle value (SHD_DUP_366), value type:%s", db_field_data_type)
        switch db_field_data_type {
		case "string":
        	 // The value is an array. The database field data type is string. 
         	 // Need to convert the array to a single string
        	 stringParts := make([]string, len(val))
        	 for i, item := range val {
            	if item != nil {
                	stringParts[i] = fmt.Sprintf("%v", item)
            	} else {
                	stringParts[i] = "null"  // or "" depending on your preference
            	}
         	 }
        	 // Join array elements with a delimiter (comma, pipe, etc.)
        	 // You can customize the delimiter based on your needs
        	 resultString := strings.Join(stringParts, ",")
        	 *args = append(*args, resultString)
        	 *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
        	 *paramCount++
        	 return nil

        case "text[]", "varchar[]", "string[]":
            stringArray := make([]string, len(val))
            for i, item := range val {
                if item != nil {
                    stringArray[i] = fmt.Sprintf("%v", item)
                } else {
                    stringArray[i] = ""
                }
            }
            *args = append(*args, pq.Array(stringArray))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        case "integer[]", "int[]", "int4[]":
            intArray := make([]int, len(val))
            for i, item := range val {
                switch v := item.(type) {
                case int:
                    intArray[i] = v
                case float64: // JSON numbers
                    intArray[i] = int(v)
                case string:
                    if num, err := strconv.Atoi(v); err == nil {
                        intArray[i] = num
                    } else {
                        return fmt.Errorf("cannot convert array element '%s' to integer", v)
                    }
                case nil:
                    intArray[i] = 0
                default:
                    return fmt.Errorf("unsupported type %T for integer array element", v)
                }
            }
            *args = append(*args, pq.Array(intArray))
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        default:
            return fmt.Errorf("unsupported database field type '%s' for array value", db_field_data_type)
        }

    default:
        // Handle other types by converting to string
		log.Printf("------------handle value (SHD_DUP_413), value type:defaul")
        strVal := fmt.Sprintf("%v", val)
        switch db_field_data_type {
        case "text", "varchar", "char", "string":
            *args = append(*args, strVal)
            *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
            *paramCount++
            return nil

        default:
            return fmt.Errorf("unsupported database field type '%s' for value type %T", db_field_data_type, val)
        }
    }
}

func handleArrayValue(
		fieldDef ApiTypes.FieldDef,
		value interface{},
		args *[]interface{},
		placeholders *[]string,
		paramCount *int) error {
	switch fieldDef.ElementType {
	case "string":
		 return handleStrArray(value, args, placeholders, paramCount)

	case "int32":
		 return handleInt32Array(value, args, placeholders, paramCount)

	case "int64":
		 return handleInt64Array(value, args, placeholders, paramCount)

	default:
		 error_msg := fmt.Sprintf("array element data type not supported:%s", fieldDef.DataType)
		 log.Printf("***** Alarm:%s (SHD_DUP_096)", error_msg)
		 return fmt.Errorf("%s", error_msg)
	}
}

func handleStrArray(
		value interface{},
		args *[]interface{},
		placeholders *[]string,
		paramCount *int) error {
    // Convert the value to string array
    var stringArray []string
        
	log.Printf("==========handleStrArray (SHD_DUP_114)")
    switch v := value.(type) {
    case []string:
		 log.Printf("==========handleStrArray (SHD_DUP_117)")
         stringArray = v

    case []interface{}:
         // Convert []interface{} to []string
		 log.Printf("==========handleStrArray (SHD_DUP_122)")
         stringArray = make([]string, len(v))
         for i, item := range v {
            if item != nil {
                stringArray[i] = fmt.Sprintf("%v", item)
            } else {
                stringArray[i] = ""
            }
         }

    case string:
         // If it's a single string, you might want to treat it as single-element array
		 log.Printf("==========handleStrArray (SHD_DUP_134)")
         stringArray = []string{v}

    case nil:
		 log.Printf("==========handleStrArray (SHD_DUP_138)")
         stringArray = []string{}

    default:
         // Try to handle other types that might represent arrays
		 log.Printf("==========handleStrArray (SHD_DUP_143)")
         stringArray = []string{fmt.Sprintf("%v", v)}
    }
        
    *args = append(*args, pq.Array(stringArray))
    *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
    *paramCount++
	return nil
}

func handleInt32Array(
		value interface{},
		args *[]interface{},
		placeholders *[]string,
		paramCount *int) error {
    // Convert the value to integer array
    var intArray []int32
        
	error_msg := ""
    switch v := value.(type) {
    case []int32:
         intArray = v

    case []int:
         intArray = make([]int32, len(v))
		 for i, item := range v {
		 	if item < math.MinInt || item > math.MaxInt {
				error_msg += fmt.Sprintf("Value out of bound, idx:%d, value:%d (01). ", i, item)
		 	}
         	intArray[i] = int32(item)
		 }

    case []interface{}:
         // Convert []interface{} to []int
         intArray = make([]int32, len(v))
         for i, item := range v {
            if item != nil {
                // Convert to int - you might want to handle conversion errors
                switch val := item.(type) {
                case int:
					 if val < math.MinInt || val > math.MaxInt {
						 error_msg += fmt.Sprintf("Value out of bound, idx:%d, value:%d (02). ", i, val)
					 }
                     intArray[i] = int32(val)

                case int64:
					 if val < math.MinInt || val > math.MaxInt {
						 error_msg += fmt.Sprintf("Value out of bound, idx:%d, value:%d (03). ", i, val)
					 }
                     intArray[i] = int32(val)

                case int32:
                     intArray[i] = val

                case float64: // JSON numbers are often float64
					 if val < math.MinInt || val > math.MaxInt {
						 error_msg += fmt.Sprintf("Value out of bound, idx:%d, value:%v (04). ", i, val)
					 }
                     intArray[i] = int32(val)

                case string:
                     // Convert string to int if possible. 'num' is int64.
                     if num, err := strconv.Atoi(val); err == nil {
					 	if num < math.MinInt || num > math.MaxInt {
						 	error_msg += fmt.Sprintf("Value out of bound, idx:%d, value:%s (05). ", i, val)
					 	}
                        intArray[i] = int32(num)
                     } else {
						error_msg += fmt.Sprintf("value is not a number:%d, value:%s (06). ", i, val)
                        intArray[i] = 0 // or handle error as appropriate
                     }
                default:
					 error_msg += fmt.Sprintf("unrecognized type:%d, value:%v (07). ", i, val)
                     intArray[i] = 0 // default value for unhandled types
                }
            } else {
                intArray[i] = 0
            }
         }

    case int:
         // If it's a single int, treat it as single-element array
		 if v < math.MinInt || v > math.MaxInt {
			 error_msg += fmt.Sprintf("Value out of bound:%d (08). ", v)
		 }
         intArray = []int32{int32(v)}

    case int64:
         // If it's a single int64, treat it as single-element array
		 if v < math.MinInt || v > math.MaxInt {
			 error_msg += fmt.Sprintf("Value out of bound:%d (08). ", v)
		 }
         intArray = []int32{int32(v)}

    case string:
         // If it's a string, try to convert to int
         if num, err := strconv.Atoi(v); err == nil {
		 	if num < math.MinInt || num > math.MaxInt {
			 	error_msg += fmt.Sprintf("Value out of bound:%s (09). ", v)
		 	}
            intArray = []int32{int32(num)}
         } else {
			error_msg += fmt.Sprintf("value is not an integer:%v (10). ", v)
             intArray = []int32{0} // or handle error as appropriate
         }

    case nil:
         intArray = []int32{}

    default:
         // Try to handle other types that might represent integers
         var num int32
         switch val := v.(type) {
         case int:
		 	  if val < math.MinInt || val > math.MaxInt {
			 	  error_msg += fmt.Sprintf("Value out of bound:%d (11). ", val)
		 	  }
              num = int32(val)

         case int64:
		 	  if val < math.MinInt || val > math.MaxInt {
			 	  error_msg += fmt.Sprintf("Value out of bound:%d (12). ", val)
		 	  }
              num = int32(val)

         case int32:
              num = val

         case float64:
		 	  if val < math.MinInt || val > math.MaxInt {
			 	  error_msg += fmt.Sprintf("Value out of bound:%f (12). ", val)
		 	  }
              num = int32(val)

         default:
              num = 0
         }
         intArray = []int32{num}
    }
        
    *args = append(*args, pq.Array(intArray))
    *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
    *paramCount++
    return nil
}

func handleInt64Array(
		value interface{},
		args *[]interface{},
		placeholders *[]string,
		paramCount *int) error {
    // Convert the value to integer array
    var intArray []int64
        
    error_msg := ""
    switch v := value.(type) {
    case []int64:
         intArray = v

    case []int:
         intArray = make([]int64, len(v))
         for i, item := range v {
             intArray[i] = int64(item)
         }

    case []interface{}:
         // Convert []interface{} to []int64
         intArray = make([]int64, len(v))
         for i, item := range v {
            if item != nil {
                // Convert to int64 - you might want to handle conversion errors
                switch val := item.(type) {
                case int:
                     intArray[i] = int64(val)

                case int64:
                     intArray[i] = val

                case int32:
                     intArray[i] = int64(val)

                case float64: // JSON numbers are often float64
                     intArray[i] = int64(val)

                case string:
                     // Convert string to int if possible. 'num' is int64.
                     if num, err := strconv.ParseInt(val, 10, 64); err == nil {
                        intArray[i] = num
                     } else {
                        error_msg += fmt.Sprintf("value is not a number:%d, value:%s (06). ", i, val)
                        intArray[i] = 0 // or handle error as appropriate
                     }
                default:
                     error_msg += fmt.Sprintf("unrecognized type:%d, value:%v (07). ", i, val)
                     intArray[i] = 0 // default value for unhandled types
                }
            } else {
                intArray[i] = 0
            }
         }

    case int:
         // If it's a single int, treat it as single-element array
         intArray = []int64{int64(v)}

    case int64:
         // If it's a single int64, treat it as single-element array
         intArray = []int64{v}

    case int32:
         // If it's a single int32, treat it as single-element array
         intArray = []int64{int64(v)}

    case string:
         // If it's a string, try to convert to int64
         if num, err := strconv.ParseInt(v, 10, 64); err == nil {
            intArray = []int64{num}
         } else {
            error_msg += fmt.Sprintf("value is not an integer:%v (10). ", v)
             intArray = []int64{0} // or handle error as appropriate
         }

    case nil:
         intArray = []int64{}

    default:
         // Try to handle other types that might represent integers
         var num int64
         switch val := v.(type) {
         case int:
              num = int64(val)

         case int64:
              num = val

         case int32:
              num = int64(val)

         case float64:
              num = int64(val)

         default:
              num = 0
         }
         intArray = []int64{num}
    }
        
    *args = append(*args, pq.Array(intArray))
    *placeholders = append(*placeholders, fmt.Sprintf("$%d", *paramCount))
    *paramCount++
    return nil
}

func CreateOnConflictPG(
			resource_store ApiTypes.ResourceStoreDef,
			resource_name string) (string, error) {
	conflictCols, err := GetFieldStrArrayValue(resource_store.ResourceDef.ResourceJSON, resource_name, "on_conflict_cols")
	if err != nil {
		return "", err
	}

	if len(conflictCols) == 0 {
		return "", nil
	}

	updateCols, err := GetFieldStrArrayValue(resource_store.ResourceDef.ResourceJSON, resource_name, "on_conflict_update_cols")
	if err != nil {
		return "", err
	}

	if len(updateCols) == 0 {
		return "", fmt.Errorf("updateCols cannot be empty (SHD_DUP_049)")
	}

	// UPDATE SET col = EXCLUDED.col
	updateAssignments := []string{}
	for _, col := range updateCols {
		updateAssignments = append(updateAssignments,
			fmt.Sprintf("%s = EXCLUDED.%s", col, col))
	}

	suffix := fmt.Sprintf(
			"ON CONFLICT (%s) DO UPDATE SET %s",
			strings.Join(conflictCols, ","),
			strings.Join(updateAssignments, ","),)
	return suffix, nil
}

func PgCopy(
	conn *pgx.Conn,
	tableName string,
	fieldInfo []ApiTypes.FieldDef,
	records []map[string]interface{},
) error {

	columns := make([]string, len(fieldInfo))
	for i, f := range fieldInfo {
		columns[i] = f.FieldName
	}

	// Build CopyFromSource
	rows := make([][]interface{}, len(records))
	for i, rec := range records {
		row := make([]interface{}, len(fieldInfo))
		for j, f := range fieldInfo {
			val, ok := rec[f.FieldName]
			if f.Required && !ok {
				return fmt.Errorf("missing required field: %s", f.FieldName)
			}
			row[j] = val
		}
		rows[i] = row
	}

	_, err := conn.CopyFrom(
		context.Background(),
		pgx.Identifier{tableName},
		columns,
		pgx.CopyFromRows(rows),
	)
	return err
}
