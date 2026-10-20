package cfield

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xuri/excelize/v2"
)

func TestToHeaders(t *testing.T) {
	configs := ExtendConfig{
		{Code: "a", Name: "Field A", Type: TypeString},
		{Code: "b", Name: "Field B", Type: TypeInt},
	}
	assert.Equal(t, []string{"Field A", "Field B"}, configs.ToHeaders())
	assert.Empty(t, ExtendConfig{}.ToHeaders())
}

func TestGetValidationText(t *testing.T) {
	tests := []struct {
		name     string
		conf     FieldConfig
		contains string
	}{
		{
			name:     "required string",
			conf:     FieldConfig{Type: TypeString, Required: true},
			contains: "必填，文本",
		},
		{
			name:     "optional int",
			conf:     FieldConfig{Type: TypeInt},
			contains: "选填，整数",
		},
		{
			name:     "number",
			conf:     FieldConfig{Type: TypeNumber},
			contains: "数字",
		},
		{
			name:     "bool",
			conf:     FieldConfig{Type: TypeBool},
			contains: "布尔，是/否",
		},
		{
			name:     "date with formatter",
			conf:     FieldConfig{Type: TypeDate, Formatter: "YYYY-MM"},
			contains: "时间，格式:(YYYY-MM)",
		},
		{
			name:     "date without formatter",
			conf:     FieldConfig{Type: TypeDate},
			contains: "(YYYY-MM-DD)",
		},
		{
			name:     "radio with options",
			conf:     FieldConfig{Type: TypeRadio, Options: []Option{{Label: "男", Value: "male"}, {Label: "女", Value: "female"}}},
			contains: "单选",
		},
		{
			name:     "checkbox with options",
			conf:     FieldConfig{Type: TypeCheckbox, Options: []Option{{Label: "阅读", Value: "read"}}},
			contains: "多选",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.conf.GetValidationText(), tt.contains)
		})
	}

	assert.Contains(t,
		FieldConfig{Type: TypeRadio, Options: []Option{{Label: "男", Value: "male"}}}.GetValidationText(),
		"[男]")
}

func newSheet(t *testing.T, headers []string) (*excelize.File, string) {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		assert.NoError(t, err)
		assert.NoError(t, f.SetCellValue(sheet, cell, h))
	}
	return f, sheet
}

func TestAppendHeader(t *testing.T) {
	configs := ExtendConfig{
		{Code: "a", Name: "Field A", Type: TypeString},
		{Code: "b", Name: "Field B", Type: TypeInt, Required: true},
	}

	f, sheet := newSheet(t, []string{"Order No", "Customer"})
	assert.NoError(t, configs.AppendHeader(f, sheet))

	rows, err := f.GetRows(sheet)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Order No", "Customer", "Field A", "Field B"}, rows[0])
	assert.Equal(t, []string{"", "", "选填，文本", "必填，整数"}, rows[1])

	t.Run("empty config is a no-op", func(t *testing.T) {
		f, sheet := newSheet(t, []string{"Order No"})
		assert.NoError(t, ExtendConfig{}.AppendHeader(f, sheet))
		rows, _ := f.GetRows(sheet)
		assert.Equal(t, []string{"Order No"}, rows[0])
	})
}

func TestInsertHeader(t *testing.T) {
	configs := ExtendConfig{
		{Code: "a", Name: "Field A", Type: TypeString},
		{Code: "b", Name: "Field B", Type: TypeString},
	}

	t.Run("insert after target", func(t *testing.T) {
		f, sheet := newSheet(t, []string{"Order No", "Customer", "Status"})
		assert.NoError(t, configs.InsertHeader(f, sheet, "Order No"))

		rows, err := f.GetRows(sheet)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Order No", "Field A", "Field B", "Customer", "Status"}, rows[0])
	})

	t.Run("unknown target appends to the end", func(t *testing.T) {
		f, sheet := newSheet(t, []string{"Order No"})
		assert.NoError(t, configs.InsertHeader(f, sheet, "Missing"))

		rows, err := f.GetRows(sheet)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Order No", "Field A", "Field B"}, rows[0])
	})

	t.Run("empty sheet appends", func(t *testing.T) {
		f := excelize.NewFile()
		sheet := f.GetSheetName(0)
		assert.NoError(t, configs.InsertHeader(f, sheet, "Anything"))

		rows, err := f.GetRows(sheet)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Field A", "Field B"}, rows[0])
	})
}

func TestExportMap(t *testing.T) {
	configs := ExtendConfig{
		{Code: "is_vip", Name: "VIP", Type: TypeBool},
		{Code: "gender", Name: "Gender", Type: TypeRadio, Options: []Option{{Label: "Male", Value: "male"}}},
		{Code: "tags", Name: "Tags", Type: TypeCheckbox, Options: []Option{{Label: "Reading", Value: "read"}, {Label: "Sports", Value: "sport"}}},
		{Code: "file", Name: "File", Type: TypeFile},
	}

	vo := ExtendVO{
		{Code: "is_vip", Value: true},
		{Code: "gender", Value: "male"},
		{Code: "tags", Value: []string{"read", "sport"}},
		{Code: "file", Value: []FileInfo{{FileName: "report.pdf"}}},
	}

	m := vo.ExportMap(configs)
	assert.Equal(t, map[string]string{
		"is_vip": "是",
		"gender": "Male",
		"tags":   "Reading,Sports",
	}, m)
}

func TestToExportSlice(t *testing.T) {
	configs := ExtendConfig{
		{Code: "c1", Name: "C1", Type: TypeString},
		{Code: "c2", Name: "C2", Type: TypeInt},
		{Code: "c3", Name: "C3", Type: TypeString},
	}
	vo := ExtendVO{
		{Code: "c1", Value: "val1"},
		{Code: "c2", Value: 123},
	}

	slice := vo.ToExportSlice(configs)
	assert.Equal(t, []string{"val1", "123", ""}, slice, "missing values align with headers as empty strings")
}

func TestFormatAnyToString(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Equal(t, "", formatAnyToString(nil, FieldConfig{Type: TypeString}))
	})

	t.Run("date without config layout falls back", func(t *testing.T) {
		conf := FieldConfig{Type: TypeDate, Formatter: "YYYY-MM-DD"}
		assert.Equal(t, "2024-01-02", formatAnyToString("2024-01-02", conf))
	})

	t.Run("attachment names", func(t *testing.T) {
		conf := FieldConfig{Type: TypeFile}
		val := []FileInfo{{FileName: "a.pdf"}, {FileName: "b.pdf"}}
		assert.Equal(t, "a.pdf,b.pdf", formatAnyToString(val, conf))
	})

	t.Run("checkbox fallback on non-slice", func(t *testing.T) {
		conf := FieldConfig{Type: TypeCheckbox, Options: []Option{{Label: "A", Value: "a"}}}
		assert.Equal(t, "42", formatAnyToString(42, conf))
	})
}
