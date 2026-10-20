package cfield

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtendConfig_Validate(t *testing.T) {
	validRadio := FieldConfig{
		Code: "gender", Name: "Gender", Type: TypeRadio,
		Options: []Option{{Label: "Male", Value: "male"}, {Label: "Female", Value: "female"}},
	}

	tests := []struct {
		name    string
		configs ExtendConfig
		wantErr string // empty means no error
	}{
		{
			name:    "valid schema",
			configs: ExtendConfig{{Code: "name", Name: "Name", Type: TypeString, Required: true}, validRadio},
		},
		{
			name:    "empty schema is valid",
			configs: ExtendConfig{},
		},
		{
			name:    "field code with space",
			configs: ExtendConfig{{Code: "bad code", Name: "Name", Type: TypeString}},
			wantErr: "invalid field code",
		},
		{
			name:    "field code enables SQL injection",
			configs: ExtendConfig{{Code: "a' OR '1'='1", Name: "Name", Type: TypeString}},
			wantErr: "invalid field code",
		},
		{
			name:    "field code starting with digit",
			configs: ExtendConfig{{Code: "1abc", Name: "Name", Type: TypeString}},
			wantErr: "invalid field code",
		},
		{
			name: "duplicate field code",
			configs: ExtendConfig{
				{Code: "name", Name: "Name", Type: TypeString},
				{Code: "name", Name: "Other", Type: TypeInt},
			},
			wantErr: "duplicate field code",
		},
		{
			name:    "unknown type",
			configs: ExtendConfig{{Code: "x", Name: "X", Type: FieldType("polygon")}},
			wantErr: "unsupported type",
		},
		{
			name:    "date without formatter",
			configs: ExtendConfig{{Code: "d", Name: "D", Type: TypeDate}},
			wantErr: "unsupported date formatter",
		},
		{
			name:    "date with unknown formatter",
			configs: ExtendConfig{{Code: "d", Name: "D", Type: TypeDate, Formatter: "DD/MM/YYYY"}},
			wantErr: "unsupported date formatter",
		},
		{
			name:    "unknown edit condition",
			configs: ExtendConfig{{Code: "x", Name: "X", Type: TypeString, EditCondition: EditCondition("sometimes")}},
			wantErr: "unsupported edit condition",
		},
		{
			name: "duplicate option value",
			configs: ExtendConfig{{
				Code: "tags", Name: "Tags", Type: TypeCheckbox,
				Options: []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "a"}},
			}},
			wantErr: "duplicate option value",
		},
		{
			name: "empty option value",
			configs: ExtendConfig{{
				Code: "tags", Name: "Tags", Type: TypeCheckbox,
				Options: []Option{{Label: "A", Value: ""}},
			}},
			wantErr: "empty value",
		},
		{
			name:    "maxLength not positive",
			configs: ExtendConfig{{Code: "s", Name: "S", Type: TypeString, UseMaxLength: true, MaxLength: 0}},
			wantErr: "maxLength must be positive",
		},
		{
			name:    "precision out of range",
			configs: ExtendConfig{{Code: "n", Name: "N", Type: TypeFloat, UsePrecision: true, Precision: 20}},
			wantErr: "precision must be within",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.configs.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestFieldConfig_DateLayout(t *testing.T) {
	conf := FieldConfig{Code: "d", Type: TypeDate, Formatter: "YYYY-MM"}
	layout, ok := conf.DateLayout()
	assert.True(t, ok)
	assert.Equal(t, "2006-01", layout)

	conf = FieldConfig{Code: "d", Type: TypeString, Formatter: "YYYY-MM"}
	_, ok = conf.DateLayout()
	assert.False(t, ok)
}

func TestExtendConfig_Filters(t *testing.T) {
	configs := ExtendConfig{
		{Code: "a", Name: "A", Type: TypeString},
		{Code: "b", Name: "B", Type: TypeFile},
		{Code: "c", Name: "C", Type: TypeString, Disabled: true},
		{Code: "d", Name: "D", Type: TypeInt, Required: true},
	}

	assert.Equal(t, []string{"a", "b", "d"}, codes(configs.FilterDisabled()))
	assert.Equal(t, []string{"a", "d"}, codes(configs.FilterDisabledAndFile()))

	ignoreRequired := configs.IgnoreRequired()
	for _, conf := range ignoreRequired {
		assert.False(t, conf.Required)
	}
	// Original is untouched.
	assert.True(t, configs[3].Required)
}

func TestExtendConfig_ValueScan(t *testing.T) {
	configs := ExtendConfig{
		{Code: "name", Name: "Name", Type: TypeString, Required: true},
	}

	v, err := configs.Value()
	assert.NoError(t, err)
	assert.Equal(t, []byte(`[{"code":"name","name":"Name","type":"string","required":true}]`), v)

	var scanned ExtendConfig
	assert.NoError(t, scanned.Scan(v))
	assert.Equal(t, configs, scanned)

	// Empty config marshals as [] and nil scans as empty.
	v, err = ExtendConfig{}.Value()
	assert.NoError(t, err)
	assert.Equal(t, []byte("[]"), v)
	assert.NoError(t, scanned.Scan(nil))
	assert.Empty(t, scanned)

	assert.Error(t, scanned.Scan(42))
}

func codes(c ExtendConfig) []string {
	res := make([]string, 0, len(c))
	for _, conf := range c {
		res = append(res, conf.Code)
	}
	return res
}
