package autotester

import (
	"fmt"
	"reflect"
	"strings"
)

// AssertEqual compares expected and actual values using reflect.DeepEqual.
// Returns true if they are equal, false otherwise with a description of the difference.
func AssertEqual(expected, actual interface{}) (bool, string) {
	if reflect.DeepEqual(expected, actual) {
		return true, ""
	}
	return false, fmt.Sprintf("expected %v (%T), got %v (%T)", expected, expected, actual, actual)
}

// AssertNoError returns true if err is nil.
func AssertNoError(err error) (bool, string) {
	if err == nil {
		return true, ""
	}
	return false, err.Error()
}

// AssertError returns true if err is not nil.
func AssertError(err error) (bool, string) {
	if err != nil {
		return true, ""
	}
	return false, "expected an error but got nil"
}

// AssertErrorContains returns true if err is not nil and contains the substring.
func AssertErrorContains(err error, substring string) (bool, string) {
	if err == nil {
		return false, fmt.Sprintf("expected an error containing %q but got nil", substring)
	}
	if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(substring)) {
		return true, ""
	}
	return false, fmt.Sprintf("expected error to contain %q, got: %s", substring, err.Error())
}

// AssertTrue returns true if condition is true.
func AssertTrue(condition bool, message string) (bool, string) {
	if condition {
		return true, ""
	}
	return false, message
}

// AssertFalse returns true if condition is false.
func AssertFalse(condition bool, message string) (bool, string) {
	if !condition {
		return true, ""
	}
	return false, message
}

// AssertNotNil returns true if value is not nil.
func AssertNotNil(value interface{}) (bool, string) {
	if value != nil {
		return true, ""
	}
	return false, "expected non-nil value but got nil"
}

// AssertNil returns true if value is nil.
func AssertNil(value interface{}) (bool, string) {
	if value == nil {
		return true, ""
	}
	return false, fmt.Sprintf("expected nil but got %v (%T)", value, value)
}

// AssertContains returns true if slice contains the element.
// Uses reflect.DeepEqual for element comparison.
func AssertContains(slice, element interface{}) (bool, string) {
	sliceValue := reflect.ValueOf(slice)
	if sliceValue.Kind() != reflect.Slice && sliceValue.Kind() != reflect.Array {
		return false, fmt.Sprintf("expected slice or array, got %T", slice)
	}

	for i := 0; i < sliceValue.Len(); i++ {
		if reflect.DeepEqual(sliceValue.Index(i).Interface(), element) {
			return true, ""
		}
	}
	return false, fmt.Sprintf("slice does not contain %v", element)
}

// AssertLen returns true if the slice/array/map has the expected length.
func AssertLen(collection interface{}, expectedLen int) (bool, string) {
	value := reflect.ValueOf(collection)
	switch value.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		actualLen := value.Len()
		if actualLen == expectedLen {
			return true, ""
		}
		return false, fmt.Sprintf("expected length %d, got %d", expectedLen, actualLen)
	default:
		return false, fmt.Sprintf("cannot get length of %T", collection)
	}
}

// AssertInRange returns true if value is within [min, max].
// Works with int, int8, int16, int32, int64, uint, float32, float64.
func AssertInRange(value, min, max interface{}) (bool, string) {
	v := reflect.ValueOf(value)
	minVal := reflect.ValueOf(min)
	maxVal := reflect.ValueOf(max)

	if v.Kind() != minVal.Kind() || v.Kind() != maxVal.Kind() {
		return false, "value, min, and max must have the same type"
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		vv, minv, maxv := v.Int(), minVal.Int(), maxVal.Int()
		if vv >= minv && vv <= maxv {
			return true, ""
		}
		return false, fmt.Sprintf("value %d is not in range [%d, %d]", vv, minv, maxv)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		vv, minv, maxv := v.Uint(), minVal.Uint(), maxVal.Uint()
		if vv >= minv && vv <= maxv {
			return true, ""
		}
		return false, fmt.Sprintf("value %d is not in range [%d, %d]", vv, minv, maxv)

	case reflect.Float32, reflect.Float64:
		vv, minv, maxv := v.Float(), minVal.Float(), maxVal.Float()
		if vv >= minv && vv <= maxv {
			return true, ""
		}
		return false, fmt.Sprintf("value %f is not in range [%f, %f]", vv, minv, maxv)

	default:
		return false, fmt.Sprintf("unsupported type %T for range check", value)
	}
}

// AssertSubstring returns true if needle is in haystack (case-insensitive).
func AssertSubstring(haystack, needle string) (bool, string) {
	if strings.Contains(strings.ToLower(haystack), strings.ToLower(needle)) {
		return true, ""
	}
	return false, fmt.Sprintf("expected %q to contain %q", haystack, needle)
}

// AssertDurationWithin returns true if duration is within the expected range.
func AssertDurationWithin(duration, min, max interface{}) (bool, string) {
	return AssertInRange(duration, min, max)
}
