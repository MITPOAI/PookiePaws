package conv

import (
	"fmt"
	"strings"
)

// AsString converts any value to its string representation.
// Returns empty string for nil values.
func AsString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

// AsStringSlice converts any value to a string slice.
// Supports []string and []any inputs. Returns nil for unsupported types.
func AsStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, AsString(item))
		}
		return items
	default:
		return nil
	}
}

// AsBool converts any value to a boolean.
// Supports bool and string ("true"/"false") inputs. Returns false for unsupported types.
func AsBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
