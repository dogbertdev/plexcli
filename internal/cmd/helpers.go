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

// anyToInt safely converts an any type to int
func anyToInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// int64PtrToInt safely converts *int64 to int
func int64PtrToInt(v *int64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}
