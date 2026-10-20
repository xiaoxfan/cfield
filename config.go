package cfield

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// FieldType is the data type of a custom field.
type FieldType string

const (
	TypeString   FieldType = "string"     // single-line or multi-line text
	TypeInt      FieldType = "int"        // integer
	TypeFloat    FieldType = "float"      // float with optional precision
	TypeNumber   FieldType = "number"     // alias of TypeFloat, kept for wire compatibility
	TypeBool     FieldType = "boolean"    // true/false
	TypeDate     FieldType = "date"       // date with a configurable layout
	TypeFile     FieldType = "attachment" // list of attached files
	TypeRadio    FieldType = "radio"      // single choice from fixed options
	TypeCheckbox FieldType = "checkbox"   // multiple choices from fixed options
)

// AllFieldTypes lists every supported field type.
var AllFieldTypes = []FieldType{
	TypeString, TypeInt, TypeFloat, TypeNumber, TypeBool,
	TypeDate, TypeFile, TypeRadio, TypeCheckbox,
}

// IsValid reports whether t is a supported field type.
func (t FieldType) IsValid() bool {
	for _, ft := range AllFieldTypes {
		if ft == t {
			return true
		}
	}
	return false
}

// EditCondition controls whether a field can be modified after the entity is
// created.
type EditCondition string

const (
	// EditableAfterCreate is the default: the field can be edited at any time.
	EditableAfterCreate EditCondition = "editable_after_create"
	// ReadonlyAfterCreate locks a field once it has a value: after creation the
	// field may be filled in while still empty, but a non-empty value can no
	// longer be changed or cleared. See BindToDBWithEditCheck.
	ReadonlyAfterCreate EditCondition = "readonly_after_create"
)

// dateLayouts maps the user-facing formatter token to a Go time layout.
var dateLayouts = map[string]string{
	"YYYY-MM-DD HH:mm:ss": "2006-01-02 15:04:05",
	"YYYY-MM-DD":          "2006-01-02",
	"YYYY-MM":             "2006-01",
	"YYYY":                "2006",
}

// SupportedDateFormatters returns the formatter tokens accepted by date fields.
func SupportedDateFormatters() []string {
	return []string{
		"YYYY-MM-DD HH:mm:ss",
		"YYYY-MM-DD",
		"YYYY-MM",
		"YYYY",
	}
}

// Option is a selectable choice for radio and checkbox fields.
type Option struct {
	Label     string `json:"label"`     // display text
	Value     string `json:"value"`     // stored value
	IsDefault bool   `json:"isDefault"` // selected by default in empty forms
}

// FileConfig constrains attachment fields.
type FileConfig struct {
	// MaxSize is the per-file size limit in bytes; 0 means unlimited.
	MaxSize int64 `json:"maxSize"`
	// Accept lists allowed file types, e.g. ".jpg,.png" or "image/*".
	Accept string `json:"accept"`
	// ShowSize hints the front end to display file sizes.
	ShowSize bool `json:"showSize"`
}

// FieldConfig describes a single custom field.
type FieldConfig struct {
	Code string    `json:"code"`
	Name string    `json:"name"`
	Type FieldType `json:"type"`

	// Required rejects empty values on write.
	Required bool `json:"required"`
	// Display hints the front end to show the field in detail views.
	Display bool `json:"display,omitempty"`
	// HasDefaultValue hints the front end that DefaultValue is meaningful.
	HasDefaultValue bool `json:"hasDefaultValue,omitempty"`
	// Placeholder is a front-end hint text.
	Placeholder string `json:"placeholder,omitempty"`
	// Disabled removes the field from all validation, conversion, query and
	// export paths while keeping its historical data untouched.
	Disabled bool `json:"disabled,omitempty"`

	// Formatter selects the layout of date fields; see SupportedDateFormatters.
	Formatter string `json:"formatter,omitempty"`

	// ===== string =====
	TextArea     bool `json:"textArea,omitempty"`
	UseMaxLength bool `json:"useMaxLength,omitempty"`
	MaxLength    int  `json:"maxLength,omitempty"` // in runes
	// BatchQuery is only valid for string fields: when the search value
	// contains commas (ASCII or full-width)，it is split into an exact-match
	// IN list; otherwise the value falls back to fuzzy (LIKE) search.
	BatchQuery bool `json:"batchQuery,omitempty"`

	// ===== float / number =====
	UsePrecision         bool `json:"usePrecision,omitempty"`
	Precision            int  `json:"precision,omitempty"`
	UseThousandSeparator bool `json:"useThousandSeparator,omitempty"`

	// ===== radio / checkbox =====
	Options     []Option `json:"options,omitempty"`
	DisplayMode string   `json:"displayMode,omitempty"` // tile / dropdown

	// ===== attachment =====
	FileConfig *FileConfig `json:"fileConfig,omitempty"`

	// DefaultValue is the initial value suggested to the front end.
	DefaultValue any `json:"defaultValue,omitempty"`

	// Sort orders fields in UIs and exports.
	Sort int `json:"sort,omitempty"`
	// EditCondition restricts editing after creation.
	EditCondition EditCondition `json:"editCondition,omitempty"`
}

// DateLayout returns the Go time layout configured for a date field. It
// reports false for non-date fields or unsupported formatters.
func (c FieldConfig) DateLayout() (string, bool) {
	if c.Type != TypeDate {
		return "", false
	}
	layout, ok := dateLayouts[c.Formatter]
	return layout, ok
}

// ExtendConfig is the ordered schema of all custom fields of an entity. It
// implements [driver.Valuer] and [sql.Scanner] so it can be stored in a
// JSON(B) column via GORM.
type ExtendConfig []FieldConfig

// Value implements [driver.Valuer].
func (c ExtendConfig) Value() (driver.Value, error) {
	if len(c) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(c)
}

// Scan implements [sql.Scanner].
func (c *ExtendConfig) Scan(value any) error {
	if value == nil {
		*c = ExtendConfig{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cfield: cannot scan ExtendConfig from %T", value)
	}
	return json.Unmarshal(bytes, c)
}

// FilterDisabled returns a copy without disabled fields.
func (c ExtendConfig) FilterDisabled() ExtendConfig {
	return c.filter(func(conf FieldConfig) bool {
		return !conf.Disabled
	})
}

// FilterDisabledAndFile returns a copy without disabled and attachment
// fields; useful when building Excel headers.
func (c ExtendConfig) FilterDisabledAndFile() ExtendConfig {
	return c.filter(func(conf FieldConfig) bool {
		return !conf.Disabled && conf.Type != TypeFile
	})
}

// IgnoreRequired returns a copy with required flags cleared, e.g. for partial
// updates or draft saves.
func (c ExtendConfig) IgnoreRequired() ExtendConfig {
	res := make(ExtendConfig, len(c))
	for i, item := range c {
		cp := item
		cp.Required = false
		res[i] = cp
	}
	return res
}

func (c ExtendConfig) filter(keep func(FieldConfig) bool) ExtendConfig {
	res := make(ExtendConfig, 0, len(c))
	for _, item := range c {
		if keep(item) {
			res = append(res, item)
		}
	}
	return res
}

// Validate checks that the schema is well formed. Call it before persisting a
// config authored by an admin UI: every field code must be a safe identifier
// (it is interpolated into SQL by BuildQuery), types and formatters must be
// known, and option values must be unique.
func (c ExtendConfig) Validate() error {
	codes := make(map[string]struct{}, len(c))
	for _, conf := range c {
		if !safeFieldCodeRegex.MatchString(conf.Code) {
			return fmt.Errorf("cfield: invalid field code %q: must match %s", conf.Code, safeFieldCodePattern)
		}
		if _, dup := codes[conf.Code]; dup {
			return fmt.Errorf("cfield: duplicate field code %q", conf.Code)
		}
		codes[conf.Code] = struct{}{}

		if !conf.Type.IsValid() {
			return fmt.Errorf("cfield: field [%s]: unsupported type %q", conf.Name, conf.Type)
		}
		if conf.Type == TypeDate {
			if _, ok := dateLayouts[conf.Formatter]; !ok {
				return fmt.Errorf("cfield: field [%s]: unsupported date formatter %q (supported: %v)",
					conf.Name, conf.Formatter, SupportedDateFormatters())
			}
		}
		switch conf.EditCondition {
		case "", EditableAfterCreate, ReadonlyAfterCreate:
		default:
			return fmt.Errorf("cfield: field [%s]: unsupported edit condition %q", conf.Name, conf.EditCondition)
		}
		if conf.Type == TypeRadio || conf.Type == TypeCheckbox {
			values := make(map[string]struct{}, len(conf.Options))
			for _, opt := range conf.Options {
				if opt.Value == "" {
					return fmt.Errorf("cfield: field [%s]: option %q has an empty value", conf.Name, opt.Label)
				}
				if _, dup := values[opt.Value]; dup {
					return fmt.Errorf("cfield: field [%s]: duplicate option value %q", conf.Name, opt.Value)
				}
				values[opt.Value] = struct{}{}
			}
		}
		if conf.UseMaxLength && conf.MaxLength <= 0 {
			return fmt.Errorf("cfield: field [%s]: maxLength must be positive when enabled", conf.Name)
		}
		if conf.UsePrecision && (conf.Precision < 0 || conf.Precision > 12) {
			return fmt.Errorf("cfield: field [%s]: precision must be within [0, 12]", conf.Name)
		}
	}
	return nil
}
