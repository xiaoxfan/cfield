package cfield

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompareAny(t *testing.T) {
	assert.True(t, CompareAny(nil, nil))
	assert.True(t, CompareAny(nil, ""))
	assert.True(t, CompareAny(" ", nil))
	assert.True(t, CompareAny(" a ", "a"))
	assert.True(t, CompareAny(1, 1))
	assert.True(t, CompareAny(1.5, 1.5))
	assert.True(t, CompareAny(true, true))

	// Numbers compare across representations (int vs float64).
	assert.True(t, CompareAny(1, float64(1)))
	assert.True(t, CompareAny(int64(1), float64(1)))

	assert.False(t, CompareAny(nil, 0))
	assert.False(t, CompareAny("a", "b"))
	assert.False(t, CompareAny(true, false))

	now := time.Now()
	assert.True(t, CompareAny(now, now.Add(time.Hour-time.Hour)))
	assert.False(t, CompareAny(now, now.Add(time.Hour)))

	assert.True(t, CompareAny([]any{"a", 1}, []any{"a", float64(1)}))
	assert.False(t, CompareAny([]any{"a"}, []any{"a", "b"}))

	assert.True(t, CompareAny(map[string]any{"k": "v"}, map[string]any{"k": "v"}))
	assert.False(t, CompareAny(map[string]any{"k": "v"}, map[string]any{"k": "w"}))
}

func TestCompareWithConfig(t *testing.T) {
	conf := FieldConfig{Code: "d", Type: TypeDate, Formatter: "YYYY-MM-DD"}

	t.Run("date representations compare equal", func(t *testing.T) {
		assert.True(t, CompareWithConfig("2024-01-02", "2024-01-02T00:00:00Z", conf))
		assert.True(t, CompareWithConfig(
			time.Date(2024, 1, 2, 15, 0, 0, 0, time.UTC),
			"2024-01-02",
			conf,
		))
	})

	t.Run("different dates", func(t *testing.T) {
		assert.False(t, CompareWithConfig("2024-01-02", "2024-01-03", conf))
	})

	t.Run("unknown formatter falls back to day layout", func(t *testing.T) {
		conf := FieldConfig{Code: "d", Type: TypeDate, Formatter: "nope"}
		assert.True(t, CompareWithConfig("2024-01-02", "2024-01-02", conf))
	})

	t.Run("non-date fields delegate to CompareAny", func(t *testing.T) {
		conf := FieldConfig{Code: "s", Type: TypeString}
		assert.True(t, CompareWithConfig("a ", "a", conf))
	})
}

func TestFormatDateString(t *testing.T) {
	layout := "2006-01"

	s, ok := formatDateString("2024-03-05", layout)
	assert.True(t, ok)
	assert.Equal(t, "2024-03", s)

	s, ok = formatDateString("2024-03-05T10:00:00Z", layout)
	assert.True(t, ok)
	assert.Equal(t, "2024-03", s)

	_, ok = formatDateString(42, layout)
	assert.False(t, ok)

	_, ok = formatDateString("  ", layout)
	assert.False(t, ok)

	_, ok = formatDateString("not a date", layout)
	assert.False(t, ok)

	s, ok = formatDateString(time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC), layout)
	assert.True(t, ok)
	assert.Equal(t, "2024-03", s)
}
