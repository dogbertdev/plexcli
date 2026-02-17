package cmd

import (
	"fmt"
	"reflect"
)

// anyToString safely converts an any type to string
func anyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if sp, ok := v.(*string); ok && sp != nil {
		return *sp
	}
	// Handle other pointer types via reflection
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		return fmt.Sprintf("%v", rv.Elem().Interface())
	}
	return fmt.Sprintf("%v", v)
}

// int64PtrToInt safely converts *int64 to int
func int64PtrToInt(v *int64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}
