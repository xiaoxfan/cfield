package cfield

import (
	"reflect"
	"strings"
	"time"
)

// CompareAny deeply compares two custom field values. It supports the basic
// types produced by JSON decoding plus time.Time, slices and maps. Numbers
// compare across representations (int, int64, float64), so a value stored by
// BindToDB still equals its JSON round-trip. For strings, nil, "" and
// whitespace-only values are considered equal, so "cleared" fields compare
// equal across representations.
func CompareAny(v1, v2 any) bool {
	if v1 == nil && v2 == nil {
		return true
	}

	if v1 == nil {
		if str, ok := v2.(string); ok {
			return strings.TrimSpace(str) == ""
		}
		return false
	}

	if v2 == nil {
		if str, ok := v1.(string); ok {
			return strings.TrimSpace(str) == ""
		}
		return false
	}

	// Numeric values compare across int/int64/float64 representations.
	if f1, ok := toFloat64(v1); ok {
		if f2, ok := toFloat64(v2); ok {
			return f1 == f2
		}
	}

	switch val1 := v1.(type) {
	case string:
		if val2, ok := v2.(string); ok {
			return strings.TrimSpace(val1) == strings.TrimSpace(val2)
		}
	case int:
		if val2, ok := v2.(int); ok {
			return val1 == val2
		}
	case int64:
		if val2, ok := v2.(int64); ok {
			return val1 == val2
		}
	case float64:
		if val2, ok := v2.(float64); ok {
			return val1 == val2
		}
	case bool:
		if val2, ok := v2.(bool); ok {
			return val1 == val2
		}
	case time.Time:
		if val2, ok := v2.(time.Time); ok {
			return val1.Equal(val2)
		}
	case []any:
		if val2, ok := v2.([]any); ok {
			if len(val1) != len(val2) {
				return false
			}
			for i := range val1 {
				if !CompareAny(val1[i], val2[i]) {
					return false
				}
			}
			return true
		}
	case map[string]any:
		if val2, ok := v2.(map[string]any); ok {
			return mapsEqual(val1, val2)
		}
	}

	return reflect.DeepEqual(v1, v2)
}

// toFloat64 reports whether v is one of the numeric kinds produced by Go or
// JSON decoding, along with its float64 value. Booleans and time.Time are
// deliberately excluded.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// CompareWithConfig compares two values of a field, honouring its type.
// Date fields are first formatted with the field's layout on both sides, so
// that RFC 3339 strings, time.Time values and layout-formatted strings all
// compare without timezone surprises. All other types delegate to CompareAny.
func CompareWithConfig(v1, v2 any, conf FieldConfig) bool {
	if conf.Type == TypeDate {
		layout, ok := dateLayouts[conf.Formatter]
		if !ok {
			layout = "2006-01-02"
		}
		s1, ok1 := formatDateString(v1, layout)
		s2, ok2 := formatDateString(v2, layout)
		if ok1 && ok2 {
			return s1 == s2
		}
	}
	return CompareAny(v1, v2)
}

// formatDateString renders a value as a date string with the given layout.
// time.Time values keep their own timezone; strings are parsed without
// timezone conversion.
func formatDateString(val any, layout string) (string, bool) {
	if t, ok := val.(time.Time); ok {
		return t.Format(layout), true
	}
	s, ok := val.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	for _, l := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02", "2006-01"} {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format(layout), true
		}
	}
	return "", false
}

// mapsEqual compares two code/value maps with CompareAny.
func mapsEqual(m1, m2 map[string]any) bool {
	if len(m1) != len(m2) {
		return false
	}
	for key, val1 := range m1 {
		val2, ok := m2[key]
		if !ok || !CompareAny(val1, val2) {
			return false
		}
	}
	return true
}
