package cfield

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cast"
)

// BindToDB validates the submitted ExtendBO against the schema and converts
// it into the ExtendData map to persist. Required fields must be non-empty,
// every value is coerced to its canonical Go type, option values must exist,
// and attachment constraints are enforced. Disabled fields are ignored;
// fields missing from the input are stored as nil (i.e. cleared).
func BindToDB(items ExtendBO, configs ExtendConfig) (ExtendData, error) {
	inputMap := make(map[string]any, len(items))
	for _, item := range items {
		inputMap[item.Code] = item.Value
	}

	result := make(ExtendData, len(configs))
	for _, conf := range configs {
		if conf.Disabled {
			continue
		}

		val := inputMap[conf.Code]
		if isEmptyValue(val) {
			if conf.Required {
				return nil, fmt.Errorf("cfield: required field [%s] is empty", conf.Name)
			}
			result[conf.Code] = nil
			continue
		}

		converted, err := convertValue(val, conf)
		if err != nil {
			return nil, fmt.Errorf("cfield: field [%s] has an invalid %s value: %w", conf.Name, conf.Type, err)
		}
		result[conf.Code] = converted
	}

	return result, nil
}

// ValidateData checks that stored ExtendData satisfies the schema: required
// fields non-empty and values well formed. Use it after loading legacy data
// or before business transitions that demand completeness.
func ValidateData(data ExtendData, configs ExtendConfig) error {
	if data == nil {
		data = make(ExtendData)
	}

	for _, conf := range configs {
		if conf.Disabled {
			continue
		}

		val := data[conf.Code]
		if isEmptyValue(val) {
			if conf.Required {
				return fmt.Errorf("cfield: required field [%s] is empty", conf.Name)
			}
			continue
		}

		if _, err := convertValue(val, conf); err != nil {
			return fmt.Errorf("cfield: field [%s] has an invalid %s value: %w", conf.Name, conf.Type, err)
		}
	}

	return nil
}

// VOToDB converts an ExtendVO into the ExtendData map without validation,
// aligning values with the schema order. Prefer BindToDB for untrusted input.
func VOToDB(items ExtendVO, configs ExtendConfig) ExtendData {
	inputMap := make(map[string]any, len(items))
	for _, item := range items {
		inputMap[item.Code] = item.Value
	}

	result := make(ExtendData, len(configs))
	for _, conf := range configs {
		if conf.Disabled {
			continue
		}
		result[conf.Code] = inputMap[conf.Code]
	}

	return result
}

// isEmptyValue reports whether a submitted value counts as "empty": nil,
// blank string, or an empty map/slice.
func isEmptyValue(val any) bool {
	if val == nil {
		return true
	}

	switch v := val.(type) {
	case string:
		return v == ""
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case []string:
		return len(v) == 0
	case []FileInfo:
		return len(v) == 0
	case []map[string]any:
		return len(v) == 0
	}

	if s, err := cast.ToStringE(val); err == nil {
		return s == ""
	}
	// Complex non-nil values: let convertValue decide.
	return false
}

// convertValue coerces a submitted value to the canonical Go type of the
// field and enforces per-type constraints.
func convertValue(val any, conf FieldConfig) (any, error) {
	switch conf.Type {
	case TypeString:
		strVal := cast.ToString(val)
		if conf.UseMaxLength && conf.MaxLength > 0 && utf8.RuneCountInString(strVal) > conf.MaxLength {
			return nil, fmt.Errorf("length %d exceeds the limit of %d", utf8.RuneCountInString(strVal), conf.MaxLength)
		}
		return strVal, nil

	case TypeInt:
		return cast.ToIntE(val)

	case TypeFloat, TypeNumber:
		floatVal, err := cast.ToFloat64E(val)
		if err != nil {
			return nil, err
		}
		if conf.UsePrecision {
			floatVal = roundFloatPrecision(floatVal, conf.Precision)
		}
		return floatVal, nil

	case TypeBool:
		return parseBoolValue(val)

	case TypeDate:
		layout, ok := dateLayouts[conf.Formatter]
		if !ok {
			return nil, fmt.Errorf("unsupported date formatter %q", conf.Formatter)
		}
		return parseDateValue(val, layout)

	case TypeRadio:
		return convertRadioValue(val, conf)

	case TypeCheckbox:
		return convertCheckboxValue(val, conf)

	case TypeFile:
		return convertFileValue(val, conf)
	}
	return val, nil
}

// parseBoolValue accepts native booleans, 0/1 and the Chinese aliases
// 是/对/有/真 (true) and 否/错/无/假 (false), which show up in Excel imports.
func parseBoolValue(val any) (bool, error) {
	if s, ok := val.(string); ok {
		switch s {
		case "是", "对", "有", "真", "1":
			return true, nil
		case "否", "错", "无", "假", "0", "":
			return false, nil
		}
	}
	return cast.ToBoolE(val)
}

func convertRadioValue(val any, conf FieldConfig) (any, error) {
	selected := strings.TrimSpace(cast.ToString(val))
	if selected == "" {
		return "", nil
	}

	// Accept both the stored value (API input) and the display label
	// (Excel import); always store the value.
	for _, option := range conf.Options {
		if option.Value == selected || option.Label == selected {
			return option.Value, nil
		}
	}
	return nil, fmt.Errorf("value %q is not one of the valid options", selected)
}

func convertCheckboxValue(val any, conf FieldConfig) (any, error) {
	var selectedValues []string
	switch v := val.(type) {
	case []string:
		selectedValues = v
	case []any:
		selectedValues = make([]string, 0, len(v))
		for _, item := range v {
			selectedValues = append(selectedValues, cast.ToString(item))
		}
	case string:
		if v != "" {
			selectedValues = strings.Split(v, ",")
		}
	}

	seen := make(map[string]struct{}, len(selectedValues))
	for _, v := range selectedValues {
		if _, dup := seen[v]; dup {
			return nil, fmt.Errorf("duplicate value %q", v)
		}
		seen[v] = struct{}{}
	}

	valueMap := make(map[string]string, len(conf.Options)*2)
	for _, option := range conf.Options {
		valueMap[option.Value] = option.Value
		valueMap[option.Label] = option.Value
	}

	finalValues := make([]string, 0, len(selectedValues))
	for _, selected := range selectedValues {
		selected = strings.TrimSpace(selected)
		if v, ok := valueMap[selected]; ok {
			finalValues = append(finalValues, v)
		} else {
			return nil, fmt.Errorf("value %q is not one of the valid options", selected)
		}
	}
	return finalValues, nil
}

func convertFileValue(val any, conf FieldConfig) (any, error) {
	var fileInfos []FileInfo
	switch v := val.(type) {
	case []FileInfo:
		fileInfos = v
	case string:
		if err := json.Unmarshal([]byte(v), &fileInfos); err != nil {
			return nil, fmt.Errorf("invalid attachment payload")
		}
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("invalid attachment payload")
		}
		if err := json.Unmarshal(b, &fileInfos); err != nil {
			return nil, fmt.Errorf("invalid attachment payload")
		}
	}

	for _, fileInfo := range fileInfos {
		if fileInfo.FileName == "" || fileInfo.FileID == 0 {
			return nil, fmt.Errorf("fileId and fileName are required")
		}

		if conf.FileConfig != nil {
			if conf.FileConfig.MaxSize > 0 && fileInfo.FileSize > conf.FileConfig.MaxSize {
				return nil, fmt.Errorf("file %q exceeds the size limit of %d bytes", fileInfo.FileName, conf.FileConfig.MaxSize)
			}
			if err := validateFileAccept(fileInfo, conf.FileConfig.Accept); err != nil {
				return nil, err
			}
		}
	}
	return fileInfos, nil
}

// validateFileAccept checks a file against an accept list. Extensions
// (".jpg") are matched case-insensitively; MIME patterns ("image/*") fall
// back to a prefix/extension check because the file type is client-supplied.
func validateFileAccept(fileInfo FileInfo, accept string) error {
	if accept == "" {
		return nil
	}

	fileExt := filepath.Ext(fileInfo.FileName)
	for _, acc := range strings.Split(accept, ",") {
		acc = strings.TrimSpace(acc)
		if acc == "" {
			continue
		}
		if strings.HasPrefix(acc, ".") {
			if strings.EqualFold(fileExt, acc) {
				return nil
			}
			continue
		}
		// MIME pattern: match exactly or by "type/*" prefix.
		if fileInfo.FileType == acc ||
			strings.HasSuffix(acc, "/*") && strings.HasPrefix(fileInfo.FileType, strings.TrimSuffix(acc, "*")) {
			return nil
		}
	}
	return fmt.Errorf("file %q has an unsupported type %q", fileInfo.FileName, fileInfo.FileType)
}

// parseDateValue parses a date value with the field's layout, falling back to
// RFC 3339 for values produced by JSON round-trips.
func parseDateValue(val any, layout string) (time.Time, error) {
	if t, ok := val.(time.Time); ok {
		return t, nil
	}

	strVal := cast.ToString(val)
	if t, err := time.ParseInLocation(layout, strVal, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, strVal); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, strVal); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation(layout, strVal, time.Local)
	return t, fmt.Errorf("cannot parse %q with layout %q: %w", strVal, layout, err)
}

func roundFloatPrecision(value float64, precision int) float64 {
	if precision <= 0 {
		return math.Round(value)
	}
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor
}
