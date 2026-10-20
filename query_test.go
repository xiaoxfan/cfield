package cfield

import (
	"database/sql/driver"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/clause"
)

func TestBuildQuery(t *testing.T) {
	configs := ExtendConfig{
		{Code: "name", Name: "Name", Type: TypeString},
		{Code: "gender", Name: "Gender", Type: TypeRadio, Options: []Option{{Label: "Male", Value: "male"}}},
		{Code: "hobbies", Name: "Hobbies", Type: TypeCheckbox, Options: []Option{{Label: "Reading", Value: "read"}, {Label: "Sports", Value: "sport"}}},
		{Code: "birthday", Name: "Birthday", Type: TypeDate, Formatter: "YYYY-MM-DD"},
		{Code: "disabled", Name: "Disabled", Type: TypeString, Disabled: true},
	}

	exprs, err := BuildQuery(ExtendData{
		"name":     "zhang",
		"gender":   "Male",
		"hobbies":  []string{"read", "sport"},
		"birthday": []string{"2024-01-01", "2024-01-31"},
		"disabled": "skip",
		"empty":    "",
	}, configs, "extend_data")
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, exprs, 4) {
		return
	}

	stringExpr := exprs[0].(clause.Expr)
	assert.Equal(t, "extend_data->>'name' LIKE CONCAT('%', ?::text, '%')", stringExpr.SQL)
	assert.Equal(t, "zhang", stringExpr.Vars[0])

	radioExpr := exprs[1].(clause.Expr)
	assert.Equal(t, "extend_data->>'gender' = ANY(?)", radioExpr.SQL)
	radioValuer, ok := radioExpr.Vars[0].(driver.Valuer)
	assert.True(t, ok)
	radioValue, err := radioValuer.Value()
	assert.NoError(t, err)
	assert.Equal(t, `{"male"}`, radioValue, "labels are resolved to option values")

	checkboxExpr := exprs[2].(clause.Expr)
	assert.Contains(t, checkboxExpr.SQL, "jsonb_array_elements_text")
	arrayValuer, ok := checkboxExpr.Vars[0].(driver.Valuer)
	assert.True(t, ok)
	checkboxValue, err := arrayValuer.Value()
	assert.NoError(t, err)
	assert.Equal(t, `{"read","sport"}`, checkboxValue)

	dateExpr := exprs[3].(clause.Expr)
	assert.Equal(t, "(extend_data->>'birthday') != '' and (extend_data->>'birthday')::timestamp BETWEEN ? AND ?", dateExpr.SQL)
	assert.Len(t, dateExpr.Vars, 2)
}

func TestBuildQuery_InputTypes(t *testing.T) {
	configs := ExtendConfig{{Code: "name", Name: "Name", Type: TypeString}}

	for _, input := range []any{
		ExtendBO{{Code: "name", Value: "x"}},
		ExtendVO{{Code: "name", Value: "x"}},
		ExtendData{"name": "x"},
	} {
		exprs, err := BuildQuery(input, configs, "extend_data")
		assert.NoError(t, err)
		assert.Len(t, exprs, 1)
	}

	_, err := BuildQuery("not-a-field-struct", configs, "extend_data")
	assert.ErrorContains(t, err, "unsupported extend type")
}

func TestBuildQuery_Security(t *testing.T) {
	configs := ExtendConfig{{Code: "name", Name: "Name", Type: TypeString}}

	t.Run("invalid column name rejected", func(t *testing.T) {
		for _, col := range []string{"extend_data; DROP TABLE x", "a b", "1abc", "", "a.b.c"} {
			_, err := BuildQuery(ExtendData{"name": "x"}, configs, col)
			assert.ErrorContains(t, err, "invalid column name", "column %q", col)
		}
	})

	t.Run("qualified column name accepted", func(t *testing.T) {
		exprs, err := BuildQuery(ExtendData{"name": "x"}, configs, "orders.extend_data")
		assert.NoError(t, err)
		assert.Len(t, exprs, 1)
	})

	t.Run("unsafe field code rejected", func(t *testing.T) {
		evil := ExtendConfig{{Code: "x' OR '1'='1", Name: "Evil", Type: TypeString}}
		_, err := BuildQuery(ExtendData{"x' OR '1'='1": "x"}, evil, "extend_data")
		assert.ErrorContains(t, err, "invalid field code")
	})
}

func TestFieldQuery(t *testing.T) {
	t.Run("string fuzzy", func(t *testing.T) {
		expr, err := fieldQuery("abc", FieldConfig{Code: "name", Type: TypeString}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "t.extend_data->>'name' LIKE CONCAT('%', ?::text, '%')", sqlExpr.SQL)
		assert.Equal(t, "abc", sqlExpr.Vars[0])
	})

	t.Run("string batch exact match", func(t *testing.T) {
		expr, err := fieldQuery("1,2,3", FieldConfig{Code: "name", Type: TypeString, BatchQuery: true}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "t.extend_data->>'name' = ANY(?)", sqlExpr.SQL)
		value, err := sqlExpr.Vars[0].(driver.Valuer).Value()
		assert.NoError(t, err)
		assert.Equal(t, `{"1","2","3"}`, value)
	})

	t.Run("string batch with full-width comma and spaces", func(t *testing.T) {
		expr, err := fieldQuery(" 1，2 , 3 ", FieldConfig{Code: "name", Type: TypeString, BatchQuery: true}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		value, err := sqlExpr.Vars[0].(driver.Valuer).Value()
		assert.NoError(t, err)
		assert.Equal(t, `{"1","2","3"}`, value)
	})

	t.Run("string batch without comma falls back to fuzzy", func(t *testing.T) {
		expr, err := fieldQuery("abc", FieldConfig{Code: "name", Type: TypeString, BatchQuery: true}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "t.extend_data->>'name' LIKE CONCAT('%', ?::text, '%')", sqlExpr.SQL)
	})

	t.Run("string batch with only empty parts falls back to fuzzy", func(t *testing.T) {
		expr, err := fieldQuery(" , ", FieldConfig{Code: "name", Type: TypeString, BatchQuery: true}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "t.extend_data->>'name' LIKE CONCAT('%', ?::text, '%')", sqlExpr.SQL)
		assert.Equal(t, ",", sqlExpr.Vars[0])
	})

	t.Run("bool exact", func(t *testing.T) {
		expr, err := fieldQuery("是", FieldConfig{Code: "enabled", Type: TypeBool}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		value, err := sqlExpr.Vars[0].(driver.Valuer).Value()
		assert.NoError(t, err)
		assert.Equal(t, `{"true"}`, value)
	})

	t.Run("checkbox any match", func(t *testing.T) {
		expr, err := fieldQuery([]string{"read", "sport"}, FieldConfig{
			Code: "tags", Type: TypeCheckbox,
			Options: []Option{{Label: "Reading", Value: "read"}, {Label: "Sports", Value: "sport"}},
		}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Contains(t, sqlExpr.SQL, "elem.value = ANY(?)")
		value, err := sqlExpr.Vars[0].(driver.Valuer).Value()
		assert.NoError(t, err)
		assert.Equal(t, `{"read","sport"}`, value)
	})

	t.Run("checkbox label resolved to value", func(t *testing.T) {
		expr, err := fieldQuery("Reading,Sports", FieldConfig{
			Code: "tags", Type: TypeCheckbox,
			Options: []Option{{Label: "Reading", Value: "read"}, {Label: "Sports", Value: "sport"}},
		}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		value, err := sqlExpr.Vars[0].(driver.Valuer).Value()
		assert.NoError(t, err)
		assert.Equal(t, `{"read","sport"}`, value)
	})

	t.Run("date range", func(t *testing.T) {
		expr, err := fieldQuery([]string{"2024-01-01", "2024-01-31"}, FieldConfig{
			Code: "birthday", Type: TypeDate, Formatter: "YYYY-MM-DD",
		}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "(t.extend_data->>'birthday') != '' and (t.extend_data->>'birthday')::timestamp BETWEEN ? AND ?", sqlExpr.SQL)
		_, ok := sqlExpr.Vars[0].(time.Time)
		assert.True(t, ok)
		_, ok = sqlExpr.Vars[1].(time.Time)
		assert.True(t, ok)
	})

	t.Run("date range invalid", func(t *testing.T) {
		_, err := fieldQuery([]string{"2024-01-01"}, FieldConfig{
			Code: "birthday", Type: TypeDate, Formatter: "YYYY-MM-DD",
		}, "t.extend_data")
		assert.ErrorContains(t, err, "start and end time")
	})

	t.Run("date range inverted", func(t *testing.T) {
		_, err := fieldQuery([]string{"2024-02-01", "2024-01-01"}, FieldConfig{
			Code: "birthday", Type: TypeDate, Formatter: "YYYY-MM-DD",
		}, "t.extend_data")
		assert.ErrorContains(t, err, "greater than or equal to start")
	})

	t.Run("attachment fields cannot be queried", func(t *testing.T) {
		_, err := fieldQuery("x", FieldConfig{Code: "f", Type: TypeFile}, "t.extend_data")
		assert.ErrorContains(t, err, "attachment fields cannot be queried")
	})

	t.Run("empty string value", func(t *testing.T) {
		_, err := fieldQuery("  ", FieldConfig{Code: "name", Type: TypeString}, "t.extend_data")
		assert.ErrorContains(t, err, "query value is empty")
	})
}

func TestFieldQueryNumber(t *testing.T) {
	conf := FieldConfig{Code: "min_stock", Name: "Min Stock", Type: TypeInt}

	t.Run("single value exact", func(t *testing.T) {
		expr, err := fieldQuery(200, conf, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "(t.extend_data->>'min_stock' ~ ? AND (t.extend_data->>'min_stock')::numeric BETWEEN ? AND ?)", sqlExpr.SQL)
		assert.Equal(t, numberTextRegex, sqlExpr.Vars[0])
		assert.Equal(t, float64(200), sqlExpr.Vars[1])
		assert.Equal(t, float64(200), sqlExpr.Vars[2])
	})

	t.Run("string number exact", func(t *testing.T) {
		expr, err := fieldQuery("200", conf, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, float64(200), sqlExpr.Vars[1])
	})

	t.Run("range both ends", func(t *testing.T) {
		expr, err := fieldQuery([]any{100, 200.5}, FieldConfig{Code: "weight", Type: TypeFloat}, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "(t.extend_data->>'weight' ~ ? AND (t.extend_data->>'weight')::numeric BETWEEN ? AND ?)", sqlExpr.SQL)
		assert.Equal(t, float64(100), sqlExpr.Vars[1])
		assert.Equal(t, 200.5, sqlExpr.Vars[2])
	})

	t.Run("range open start", func(t *testing.T) {
		expr, err := fieldQuery([]any{nil, 200}, conf, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "(t.extend_data->>'min_stock' ~ ? AND (t.extend_data->>'min_stock')::numeric <= ?)", sqlExpr.SQL)
	})

	t.Run("range open end", func(t *testing.T) {
		expr, err := fieldQuery([]any{200, ""}, conf, "t.extend_data")
		assert.NoError(t, err)
		sqlExpr := expr.(clause.Expr)
		assert.Equal(t, "(t.extend_data->>'min_stock' ~ ? AND (t.extend_data->>'min_stock')::numeric >= ?)", sqlExpr.SQL)
	})

	t.Run("invalid value", func(t *testing.T) {
		_, err := fieldQuery("abc", conf, "t.extend_data")
		assert.Error(t, err)
	})

	t.Run("invalid range order", func(t *testing.T) {
		_, err := fieldQuery([]any{300, 200}, conf, "t.extend_data")
		assert.Error(t, err)
	})

	t.Run("empty range", func(t *testing.T) {
		_, err := fieldQuery([]any{nil, ""}, conf, "t.extend_data")
		assert.Error(t, err)
	})

	t.Run("too many range elements", func(t *testing.T) {
		_, err := fieldQuery([]any{1, 2, 3}, conf, "t.extend_data")
		assert.Error(t, err)
	})
}
