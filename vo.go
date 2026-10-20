package cfield

import (
	"encoding/json"
	"time"

	"github.com/spf13/cast"
)

// FieldVO is one code/value pair returned to a front end, enriched with the
// field metadata needed to render it.
type FieldVO struct {
	Code       string      `json:"code"`
	Name       string      `json:"name"`
	Value      any         `json:"value"`
	Required   bool        `json:"required"`
	Options    []Option    `json:"options,omitempty"`    // radio / checkbox
	FileConfig *FileConfig `json:"fileConfig,omitempty"` // attachment
}

// ExtendVO is the ordered list of custom field values returned to a front end.
type ExtendVO []FieldVO

// MarshalJSON implements [json.Marshaler]; a nil VO marshals as an empty
// array so API responses stay stable.
func (v ExtendVO) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("[]"), nil
	}
	type plain ExtendVO
	return json.Marshal(plain(v))
}

// DirectToDB converts the VO into an ExtendData map without schema
// validation; nil values are dropped.
func (v ExtendVO) DirectToDB() ExtendData {
	data := make(ExtendData, len(v))
	for _, item := range v {
		if item.Value != nil {
			data[item.Code] = item.Value
		}
	}
	return data
}

// ToExportBO converts the VO into the BO form, dropping metadata.
func (v ExtendVO) ToExportBO() ExtendBO {
	bo := make(ExtendBO, 0, len(v))
	for _, item := range v {
		bo = append(bo, FieldBO{Code: item.Code, Value: item.Value})
	}
	return bo
}

// Equals compares two ExtendVOs by code, order-independently, using
// CompareAny for value equality.
func (v ExtendVO) Equals(other ExtendVO) bool {
	m1 := make(map[string]any, len(v))
	for _, item := range v {
		m1[item.Code] = item.Value
	}
	m2 := make(map[string]any, len(other))
	for _, item := range other {
		m2[item.Code] = item.Value
	}
	return mapsEqual(m1, m2)
}

// FormatToVO converts stored ExtendData into the ordered ExtendVO for API
// responses. Values are formatted per schema: dates are rendered with their
// configured layout, option fields are enriched with their options, and
// attachment values are normalized to []FileInfo. Disabled fields are
// skipped.
func FormatToVO(dbData ExtendData, configs ExtendConfig) ExtendVO {
	vo := make(ExtendVO, 0, len(configs))

	for _, conf := range configs {
		if conf.Disabled {
			continue
		}

		item := FieldVO{
			Code:       conf.Code,
			Name:       conf.Name,
			Required:   conf.Required,
			FileConfig: conf.FileConfig,
		}

		val, ok := dbData[conf.Code]
		switch {
		case !ok || val == nil:
			item.Value = nil
		case conf.Type == TypeDate:
			// JSON round-trips leave dates as RFC3339 strings; normalize them
			// back to the configured layout.
			if s, formatted := formatDateForOutput(val, conf); formatted {
				item.Value = s
			} else {
				// Keep corrupted values visible instead of failing the read.
				item.Value = val
			}
		case conf.Type == TypeRadio || conf.Type == TypeCheckbox:
			item.Value = val
			item.Options = conf.Options
		case conf.Type == TypeFile:
			item.Value = toFileInfos(val)
		default:
			item.Value = val
		}
		vo = append(vo, item)
	}

	return vo
}

// formatDateForOutput renders a date value with the field's configured
// layout. It reports false when the value cannot be interpreted as a date.
func formatDateForOutput(val any, conf FieldConfig) (string, bool) {
	layout, ok := dateLayouts[conf.Formatter]
	if !ok {
		layout = "2006-01-02 15:04:05"
	}
	if t, ok := val.(time.Time); ok {
		return t.Format(layout), true
	}
	t, err := cast.ToTimeInDefaultLocationE(val, time.Local)
	if err != nil {
		return "", false
	}
	return t.Format(layout), true
}

// toFileInfos normalizes arbitrary JSON-decoded attachment data into
// []FileInfo; malformed data yields nil.
func toFileInfos(val any) []FileInfo {
	if infos, ok := val.([]FileInfo); ok {
		return infos
	}
	b, err := json.Marshal(val)
	if err != nil {
		return nil
	}
	var infos []FileInfo
	_ = json.Unmarshal(b, &infos)
	return infos
}

// ExportMap converts the VO into a code->string map for Excel export.
// Attachment fields and fields without schema are skipped.
func (v ExtendVO) ExportMap(configs ExtendConfig) map[string]string {
	confMap := make(map[string]FieldConfig, len(configs))
	for _, conf := range configs {
		confMap[conf.Code] = conf
	}

	res := make(map[string]string, len(v))
	for _, item := range v {
		conf, ok := confMap[item.Code]
		if !ok || conf.Disabled || conf.Type == TypeFile {
			continue
		}
		res[item.Code] = formatAnyToString(item.Value, conf)
	}
	return res
}

// ToExportSlice converts the VO into an ordered string slice aligned with the
// given configs, so it pairs with ExtendConfig.ToHeaders. Missing values are
// rendered as empty strings.
//
// By default attachment fields are skipped. Pass includeFile=true when the
// header row was generated from a config that still contains attachment
// fields, so data columns stay aligned with headers.
func (v ExtendVO) ToExportSlice(configs ExtendConfig, includeFile ...bool) []string {
	skipFile := true
	if len(includeFile) > 0 && includeFile[0] {
		skipFile = false
	}

	dataMap := make(map[string]any, len(v))
	for _, item := range v {
		dataMap[item.Code] = item.Value
	}

	res := make([]string, 0, len(configs))
	for _, conf := range configs {
		if conf.Disabled || (skipFile && conf.Type == TypeFile) {
			continue
		}
		val, ok := dataMap[conf.Code]
		if !ok || val == nil {
			res = append(res, "")
			continue
		}
		res = append(res, formatAnyToString(val, conf))
	}
	return res
}
