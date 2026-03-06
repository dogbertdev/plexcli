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
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		return fmt.Sprintf("%v", rv.Elem().Interface())
	}
	return fmt.Sprintf("%v", v)
}

// intPtrToInt safely converts *int to int
func intPtrToInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
