package RequestHandlers

import (
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func CreateValueGroupsMySQL(
			user_name string,
			fieldDefs []ApiTypes.FieldDef,
			chunk []map[string]interface{}) ([]string, []interface{}, error) {
	valueGroups := []string{}
	args := []interface{}{}
	for _, rec := range chunk {
		placeholders := []string{}
		for _, f := range fieldDefs {
			val, ok := rec[f.FieldName]
			if f.Required && !ok {
				switch f.ElementType {
				case "creator":
				case "updater":
				     val = user_name

				default:
					 return valueGroups, args, fmt.Errorf("missing required field (SHD_DUM_020): %s", f.FieldName)
				}
			}
			args = append(args, val)
			placeholders = append(placeholders, "?")
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ",")+")")
	}

	return valueGroups, args, nil
}

func CreateOnConflictMySQL(resource_request ApiTypes.InsertRequest) (string, error) {

	updateCols := resource_request.OnConflictUpdateCols

	if len(updateCols) == 0 {
		return "", nil
	}

	updateAssignments := []string{}
	for _, col := range updateCols {
		updateAssignments = append(updateAssignments, fmt.Sprintf("%s = VALUES(%s)", col, col))
	}

	conflict_suffix := "ON DUPLICATE KEY UPDATE "+strings.Join(updateAssignments, ",")
	return conflict_suffix, nil
}