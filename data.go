package cfield

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// FileInfo describes one attachment of an attachment field.
type FileInfo struct {
	FileID      int       `json:"fileId"`
	CreatedAt   time.Time `json:"createdAt"`
	FileName    string    `json:"fileName"`
	FileSize    int64     `json:"fileSize"`
	FileType    string    `json:"fileType"`
	Description string    `json:"description,omitempty"`
	UploadedBy  string    `json:"uploadedBy,omitempty"`
	IsPublic    bool      `json:"isPublic,omitempty"`
	BucketName  string    `json:"bucketName,omitempty"`
	ObjKey      string    `json:"objKey,omitempty"`
}

// ExtendData is the per-row custom field storage: a map keyed by field code.
// Embed or reference it in a GORM entity to persist it as a JSON(B) column.
type ExtendData map[string]any

// Value implements [driver.Valuer]; an empty map serializes to "{}".
func (d ExtendData) Value() (driver.Value, error) {
	if len(d) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(d)
}

// Scan implements [sql.Scanner]. Besides the current JSON-object format it
// transparently reads the legacy array format [{"code":..,"value":..}] that
// older versions of this schema wrote, so migrations are not required.
func (d *ExtendData) Scan(value any) error {
	if value == nil {
		*d = make(ExtendData)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cfield: cannot scan ExtendData from %T", value)
	}

	trimmed := trimLeadingSpace(bytes)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var list []struct {
			Code  string `json:"code"`
			Value any    `json:"value"`
		}
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return fmt.Errorf("cfield: cannot scan legacy ExtendData array: %w", err)
		}
		*d = make(ExtendData, len(list))
		for _, item := range list {
			(*d)[item.Code] = item.Value
		}
		return nil
	}

	if len(trimmed) > 0 && trimmed[0] == '{' {
		if err := json.Unmarshal(trimmed, d); err != nil {
			return fmt.Errorf("cfield: cannot scan ExtendData: %w", err)
		}
		return nil
	}

	// Empty value or anything else: treat as no data.
	*d = make(ExtendData)
	return nil
}

// UnmarshalJSON implements [json.Unmarshaler]. It accepts both the JSON-object
// format and the legacy JSON-array format, and merges into the existing map.
func (d *ExtendData) UnmarshalJSON(data []byte) error {
	if *d == nil {
		*d = make(map[string]any)
	}
	data = trimLeadingAndTrailingSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	switch data[0] {
	case '{':
		var temp map[string]any
		if err := json.Unmarshal(data, &temp); err != nil {
			return fmt.Errorf("cfield: cannot unmarshal ExtendData: %w", err)
		}
		for k, v := range temp {
			(*d)[k] = v
		}
		return nil
	case '[':
		var temp []FieldBO
		if err := json.Unmarshal(data, &temp); err != nil {
			return fmt.Errorf("cfield: cannot unmarshal legacy ExtendData array: %w", err)
		}
		for _, item := range temp {
			(*d)[item.Code] = item.Value
		}
		return nil
	}
	return fmt.Errorf("cfield: unsupported ExtendData JSON payload: %s", string(data))
}

func trimLeadingSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}

func trimLeadingAndTrailingSpace(b []byte) []byte {
	b = trimLeadingSpace(b)
	for len(b) > 0 {
		last := b[len(b)-1]
		if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
			b = b[:len(b)-1]
		} else {
			break
		}
	}
	return b
}

// Equals compares two ExtendData maps with CompareAny semantics.
func (d ExtendData) Equals(other ExtendData) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	return mapsEqual(d, other)
}

// ToMap projects the stored values onto a plain map keyed by field code.
// Disabled and attachment fields are skipped, dates are formatted according
// to their field formatter, and fields without a value are omitted. It is
// well suited for handing structured data to algorithms or exporters.
func (d ExtendData) ToMap(configs ExtendConfig) map[string]any {
	result := make(map[string]any)
	for _, conf := range configs.FilterDisabledAndFile() {
		val, ok := d[conf.Code]
		if !ok || val == nil {
			continue
		}
		if conf.Type == TypeDate {
			if s, formatted := formatDateForOutput(val, conf); formatted {
				result[conf.Code] = s
				continue
			}
		}
		result[conf.Code] = val
	}
	return result
}
