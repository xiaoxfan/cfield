package cfield

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ToHeaders returns the display names of all fields, in order, for use as an
// Excel header row (or any ordered rendering).
func (c ExtendConfig) ToHeaders() []string {
	res := make([]string, 0, len(c))
	for _, conf := range c {
		res = append(res, conf.Name)
	}
	return res
}

// AppendHeader appends the field headers to an existing Excel sheet, after
// the last used column of row 1. Existing cell styles are copied to the new
// columns so templates stay visually consistent.
func (c ExtendConfig) AppendHeader(f *excelize.File, sheetName string) error {
	if len(c) == 0 {
		return nil
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return err
	}

	startColIdx := 0
	if len(rows) > 0 {
		startColIdx = len(rows[0])
	}

	return c.writeHeaders(f, sheetName, startColIdx)
}

// InsertHeader inserts the field headers right after the column named
// targetHeaderName. When the target is not found the headers are appended to
// the end of the row instead.
func (c ExtendConfig) InsertHeader(f *excelize.File, sheetName string, targetHeaderName string) error {
	if len(c) == 0 {
		return nil
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return c.AppendHeader(f, sheetName)
	}

	headerRow := rows[0]
	targetIdx := -1
	for i, h := range headerRow {
		if h == targetHeaderName {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return c.AppendHeader(f, sheetName)
	}

	// Insert after the target column (1-based column numbers).
	insertColNum := targetIdx + 2
	insertColName, err := excelize.ColumnNumberToName(insertColNum)
	if err != nil {
		return err
	}
	if err := f.InsertCols(sheetName, insertColName, len(c)); err != nil {
		return err
	}

	return c.writeHeaders(f, sheetName, insertColNum-1)
}

// writeHeaders writes header row 1 (field names) and row 2 (validation hint
// text) starting after startColIdx (0-based: the first new column is
// startColIdx+1), copying the styles of the neighbouring columns.
func (c ExtendConfig) writeHeaders(f *excelize.File, sheetName string, startColIdx int) error {
	if len(c) == 0 {
		return nil
	}

	// Copy the styles of the existing columns next to the inserted range.
	// On an empty sheet (startColIdx == 0) there is nothing to copy from.
	var styleID1, styleID2, styleA3 int
	if startColIdx >= 1 {
		baseColName, err := excelize.ColumnNumberToName(startColIdx)
		if err != nil {
			return err
		}
		styleID1, _ = f.GetCellStyle(sheetName, fmt.Sprintf("%s1", baseColName))
		styleID2, _ = f.GetCellStyle(sheetName, fmt.Sprintf("%s2", baseColName))
	}
	styleA3, _ = f.GetCellStyle(sheetName, "A3")
	rows, _ := f.GetRows(sheetName)
	styleEndRow := len(rows) - 1

	for i, conf := range c {
		colName, err := excelize.ColumnNumberToName(startColIdx + i + 1)
		if err != nil {
			return err
		}

		cell1 := fmt.Sprintf("%s1", colName)
		if err := f.SetCellValue(sheetName, cell1, conf.Name); err != nil {
			return err
		}
		if styleID1 != 0 {
			_ = f.SetCellStyle(sheetName, cell1, cell1, styleID1)
		}

		cell2 := fmt.Sprintf("%s2", colName)
		if err := f.SetCellValue(sheetName, cell2, conf.GetValidationText()); err != nil {
			return err
		}
		if styleID2 != 0 {
			_ = f.SetCellStyle(sheetName, cell2, cell2, styleID2)
		}
	}

	startStyleColName, err := excelize.ColumnNumberToName(startColIdx + 1)
	if err != nil {
		return err
	}
	endStyleColName, err := excelize.ColumnNumberToName(startColIdx + len(c))
	if err != nil {
		return err
	}
	if styleA3 != 0 && styleEndRow >= 3 {
		start := fmt.Sprintf("%s3", startStyleColName)
		end := fmt.Sprintf("%s%d", endStyleColName, styleEndRow)
		if err := f.SetCellStyle(sheetName, start, end, styleA3); err != nil {
			return err
		}
	}

	return nil
}

// GetValidationText renders the import hint shown in the second header row of
// an Excel template, e.g. "必填，单选，请填写以下内容的一项: [男/女]". The text is
// intentionally written in the same language business users fill into the
// sheet, matching the 是/否 boolean and comma-separated checkbox conventions
// of the import path.
func (c FieldConfig) GetValidationText() string {
	var sb strings.Builder
	if c.Required {
		sb.WriteString("必填，")
	} else {
		sb.WriteString("选填，")
	}

	switch c.Type {
	case TypeString:
		sb.WriteString("文本")
	case TypeInt:
		sb.WriteString("整数")
	case TypeFloat, TypeNumber:
		sb.WriteString("数字")
	case TypeBool:
		sb.WriteString("布尔，是/否")
	case TypeDate:
		sb.WriteString("时间，格式:")
		if c.Formatter != "" {
			sb.WriteString(fmt.Sprintf("(%s)", c.Formatter))
		} else {
			sb.WriteString("(YYYY-MM-DD)")
		}
	case TypeRadio:
		sb.WriteString("单选，请填写以下内容的一项:")
		writeOptionList(&sb, c.Options)
	case TypeCheckbox:
		sb.WriteString("多选，请填写以下内容的一项或多项，用英文逗号分隔")
		writeOptionList(&sb, c.Options)
	default:
		sb.WriteString("文本")
	}
	return sb.String()
}

func writeOptionList(sb *strings.Builder, options []Option) {
	if len(options) == 0 {
		return
	}
	labels := make([]string, 0, len(options))
	for _, o := range options {
		labels = append(labels, o.Label)
	}
	sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(labels, "/")))
}
