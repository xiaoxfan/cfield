package cfield

import (
	"strings"
	"time"

	"github.com/spf13/cast"
)

// formatAnyToString renders a stored field value as display text for exports:
// dates use the field layout, booleans become 是/否, option values are mapped
// back to their labels, checkbox lists are comma-joined, attachments are
// reduced to their file names.
func formatAnyToString(val any, conf FieldConfig) string {
	if val == nil {
		return ""
	}

	switch conf.Type {
	case TypeDate:
		if t, ok := val.(time.Time); ok {
			if layout, ok := dateLayouts[conf.Formatter]; ok {
				return t.Format(layout)
			}
			return t.Format("2006-01-02 15:04:05")
		}
		return cast.ToString(val)

	case TypeBool:
		if b, err := cast.ToBoolE(val); err == nil {
			if b {
				return "是"
			}
			return "否"
		}
		return ""

	case TypeFile:
		var names []string
		for _, f := range toFileInfos(val) {
			if f.FileName != "" {
				names = append(names, f.FileName)
			}
		}
		return strings.Join(names, ",")

	case TypeRadio, TypeCheckbox:
		optionMap := make(map[string]string, len(conf.Options))
		for _, opt := range conf.Options {
			optionMap[opt.Value] = opt.Label
		}
		if conf.Type == TypeRadio {
			valueStr := cast.ToString(val)
			if label, ok := optionMap[valueStr]; ok {
				return label
			}
			return valueStr
		}
		values, err := cast.ToStringSliceE(val)
		if err != nil {
			return cast.ToString(val)
		}
		labels := make([]string, 0, len(values))
		for _, v := range values {
			if label, ok := optionMap[v]; ok {
				labels = append(labels, label)
			} else {
				labels = append(labels, v)
			}
		}
		return strings.Join(labels, ",")

	default:
		// Numbers and plain strings.
		return cast.ToString(val)
	}
}
