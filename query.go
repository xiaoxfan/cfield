package cfield

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm/clause"

	"github.com/spf13/cast"
)

// Field codes and column names are interpolated into SQL fragments, so they
// must be plain identifiers. ExtendConfig.Validate enforces this for codes;
// BuildQuery re-checks both defensively.
const (
	safeFieldCodePattern = `^[A-Za-z_][A-Za-z0-9_]*$`
	safeColumnPattern    = `^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`
)

var (
	safeFieldCodeRegex = regexp.MustCompile(safeFieldCodePattern)
	safeColumnRegex    = regexp.MustCompile(safeColumnPattern)
)

// numberTextRegex guards ::numeric casts on jsonb ->> text: casting
// non-numeric text raises a database error, so the regex filters it first.
const numberTextRegex = `^-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?$`

// BuildQuery converts filter values into GORM clause expressions backed by
// native PostgreSQL JSONB operators, ready to be combined with clause.And /
// passed to gorm's Where:
//
//	exprs, err := cfield.BuildQuery(filters, configs, "extend_data")
//	db.Clauses(clause.And(exprs...)).Find(&orders)
//
// extend accepts ExtendBO, ExtendVO or ExtendData; empty values (nil or blank
// strings) are ignored. extDataColumn is the JSONB column holding the values,
// optionally qualified with a table alias or name ("o.extend_data").
//
// Only fields present in cnf are queryable. Disabled fields and attachment
// fields are skipped (attachment fields cannot be filtered).
func BuildQuery(extend any, cnf ExtendConfig, extDataColumn string) ([]clause.Expression, error) {
	if !safeColumnRegex.MatchString(extDataColumn) {
		return nil, fmt.Errorf("cfield: invalid column name %q: must match %s", extDataColumn, safeColumnPattern)
	}

	extendData, err := toExtendData(extend)
	if err != nil {
		return nil, err
	}

	exprs := make([]clause.Expression, 0, len(cnf))
	for _, conf := range cnf {
		if conf.Disabled {
			continue
		}

		queryParam, ok := extendData[conf.Code]
		if !ok || isEmptyQueryValue(queryParam) {
			continue
		}

		expr, err := fieldQuery(queryParam, conf, extDataColumn)
		if err != nil {
			return nil, fmt.Errorf("cfield: invalid query condition for field [%s]: %w", conf.Name, err)
		}
		exprs = append(exprs, expr)
	}

	return exprs, nil
}

func toExtendData(extend any) (ExtendData, error) {
	switch v := extend.(type) {
	case ExtendVO:
		return v.DirectToDB(), nil
	case ExtendBO:
		return v.DirectToDB(), nil
	case ExtendData:
		return v, nil
	default:
		return nil, fmt.Errorf("cfield: unsupported extend type %T (want ExtendBO, ExtendVO or ExtendData)", extend)
	}
}

func fieldQuery(queryValue any, cnf FieldConfig, extDataColumn string) (clause.Expression, error) {
	if !safeFieldCodeRegex.MatchString(cnf.Code) {
		return nil, fmt.Errorf("invalid field code %q: must match %s", cnf.Code, safeFieldCodePattern)
	}

	textColumn := fmt.Sprintf("%s->>'%s'", extDataColumn, cnf.Code)

	switch cnf.Type {
	case TypeFile:
		return nil, errors.New("attachment fields cannot be queried")

	case TypeInt, TypeFloat, TypeNumber:
		start, end, err := normalizeNumberRangeQueryValue(queryValue, cnf)
		if err != nil {
			return nil, err
		}
		// jsonb ->> returns text; guard the ::numeric cast with a regex.
		guardSQL := fmt.Sprintf("%s ~ ?", textColumn)
		switch {
		case start != nil && end != nil:
			return clause.Expr{
				SQL:  fmt.Sprintf("(%s AND (%s)::numeric BETWEEN ? AND ?)", guardSQL, textColumn),
				Vars: []any{numberTextRegex, *start, *end},
			}, nil
		case start != nil:
			return clause.Expr{
				SQL:  fmt.Sprintf("(%s AND (%s)::numeric >= ?)", guardSQL, textColumn),
				Vars: []any{numberTextRegex, *start},
			}, nil
		default:
			return clause.Expr{
				SQL:  fmt.Sprintf("(%s AND (%s)::numeric <= ?)", guardSQL, textColumn),
				Vars: []any{numberTextRegex, *end},
			}, nil
		}

	case TypeString:
		value := strings.TrimSpace(cast.ToString(queryValue))
		if value == "" {
			return nil, errors.New("query value is empty")
		}
		// Batch mode: values containing commas (ASCII or full-width) are
		// split into an exact-match list; otherwise fall back to fuzzy search.
		if cnf.BatchQuery {
			if values, ok := splitBatchQueryValues(value); ok {
				return clause.Expr{
					SQL:  fmt.Sprintf("%s = ANY(?)", textColumn),
					Vars: []any{pq.Array(values)},
				}, nil
			}
		}
		return clause.Expr{
			SQL:  fmt.Sprintf("%s LIKE CONCAT('%%', ?::text, '%%')", textColumn),
			Vars: []any{value},
		}, nil

	case TypeRadio, TypeBool:
		values, err := normalizeExactMatchQueryValues(queryValue, cnf)
		if err != nil {
			return nil, err
		}
		return clause.Expr{
			SQL:  fmt.Sprintf("%s = ANY(?)", textColumn),
			Vars: []any{pq.Array(values)},
		}, nil

	case TypeCheckbox:
		values, err := normalizeCheckboxQueryValue(queryValue, cnf)
		if err != nil {
			return nil, err
		}
		return clause.Expr{
			SQL: fmt.Sprintf(
				"EXISTS (SELECT 1 FROM jsonb_array_elements_text(COALESCE(%s, '[]')::jsonb) AS elem(value) WHERE elem.value = ANY(?))",
				textColumn,
			),
			Vars: []any{pq.Array(values)},
		}, nil

	case TypeDate:
		start, end, err := normalizeDateRangeQueryValue(queryValue, cnf)
		if err != nil {
			return nil, err
		}
		return clause.Expr{
			SQL:  fmt.Sprintf("(%s) != '' and (%s)::timestamp BETWEEN ? AND ?", textColumn, textColumn),
			Vars: []any{start, end},
		}, nil

	default:
		return clause.Expr{
			SQL:  fmt.Sprintf("%s = ?", textColumn),
			Vars: []any{cast.ToString(queryValue)},
		}, nil
	}
}

// splitBatchQueryValues splits a batch search value on ASCII and full-width
// commas. It reports ok=true only when the original value contains a comma
// and at least one non-empty part remains; otherwise the caller falls back to
// fuzzy search.
func splitBatchQueryValues(value string) ([]string, bool) {
	if !strings.ContainsAny(value, ",\uff0c") {
		return nil, false
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\uff0c'
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	return values, true
}

func normalizeExactMatchQueryValues(queryValue any, cnf FieldConfig) ([]string, error) {
	values, err := toAnySlice(queryValue)
	if err != nil {
		values = []any{queryValue}
	}

	result := make([]string, 0, len(values))
	for _, item := range values {
		converted, err := convertValue(item, cnf)
		if err != nil {
			return nil, err
		}
		result = append(result, cast.ToString(converted))
	}
	if len(result) == 0 {
		return nil, errors.New("query value is empty")
	}
	return result, nil
}

func normalizeCheckboxQueryValue(queryValue any, cnf FieldConfig) ([]string, error) {
	converted, err := convertValue(queryValue, cnf)
	if err != nil {
		return nil, err
	}

	values, ok := converted.([]string)
	if !ok {
		return nil, errors.New("checkbox query value should be a string array")
	}
	if len(values) == 0 {
		return nil, errors.New("checkbox query value is empty")
	}
	return values, nil
}

func normalizeDateRangeQueryValue(queryValue any, cnf FieldConfig) (time.Time, time.Time, error) {
	rawValues, err := toAnySlice(queryValue)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("date query value should be an array of [start, end]")
	}
	if len(rawValues) != 2 {
		return time.Time{}, time.Time{}, errors.New("date query value should contain both start and end time")
	}

	start, err := convertValue(rawValues[0], cnf)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start time: %w", err)
	}
	end, err := convertValue(rawValues[1], cnf)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end time: %w", err)
	}

	startTime, ok := start.(time.Time)
	if !ok {
		return time.Time{}, time.Time{}, errors.New("invalid start time type")
	}
	endTime, ok := end.(time.Time)
	if !ok {
		return time.Time{}, time.Time{}, errors.New("invalid end time type")
	}
	if endTime.Before(startTime) {
		return time.Time{}, time.Time{}, errors.New("end time should be greater than or equal to start time")
	}

	return startTime, endTime, nil
}

// normalizeNumberRangeQueryValue normalizes a numeric filter into the range
// [start, end]. A single value means exact match (start == end); a
// two-element array means a range where a null/empty end means unbounded.
func normalizeNumberRangeQueryValue(queryValue any, cnf FieldConfig) (*float64, *float64, error) {
	rawValues, err := toAnySlice(queryValue)
	if err != nil {
		// Not an array: single-value exact match.
		value, err := toQueryNumber(queryValue, cnf)
		if err != nil {
			return nil, nil, err
		}
		return &value, &value, nil
	}

	switch len(rawValues) {
	case 1:
		value, err := toQueryNumber(rawValues[0], cnf)
		if err != nil {
			return nil, nil, err
		}
		return &value, &value, nil
	case 2:
		var start, end *float64
		if !isEmptyQueryValue(rawValues[0]) {
			value, err := toQueryNumber(rawValues[0], cnf)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid range start: %w", err)
			}
			start = &value
		}
		if !isEmptyQueryValue(rawValues[1]) {
			value, err := toQueryNumber(rawValues[1], cnf)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid range end: %w", err)
			}
			end = &value
		}
		if start == nil && end == nil {
			return nil, nil, errors.New("number query value is empty")
		}
		if start != nil && end != nil && *end < *start {
			return nil, nil, errors.New("range end should be greater than or equal to start")
		}
		return start, end, nil
	default:
		return nil, nil, errors.New("number query value should be a number or a [min, max] array")
	}
}

// toQueryNumber converts a filter value to float64, validating it against
// the field type first.
func toQueryNumber(val any, cnf FieldConfig) (float64, error) {
	converted, err := convertValue(val, cnf)
	if err != nil {
		return 0, err
	}
	switch v := converted.(type) {
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return cast.ToFloat64E(converted)
	}
}

// isEmptyQueryValue reports whether a filter value should be skipped: nil,
// blank/whitespace string, or an empty slice/map.
func isEmptyQueryValue(value any) bool {
	if value == nil {
		return true
	}

	if v, ok := value.(string); ok {
		return strings.TrimSpace(v) == ""
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	}

	return false
}

func toAnySlice(value any) ([]any, error) {
	if value == nil {
		return nil, errors.New("value is nil")
	}

	switch v := value.(type) {
	case []any:
		return v, nil
	case []string:
		result := make([]any, 0, len(v))
		for _, item := range v {
			result = append(result, item)
		}
		return result, nil
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, errors.New("value is not a slice")
	}

	result := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result = append(result, rv.Index(i).Interface())
	}
	return result, nil
}
