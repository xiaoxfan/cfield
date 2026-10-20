package cfield

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToMap(t *testing.T) {
	configs := ExtendConfig{
		{Code: "a", Name: "A", Type: TypeString},
		{Code: "b", Name: "B", Type: TypeInt},
		{Code: "c", Name: "C", Type: TypeString},
		{Code: "d", Name: "D", Type: TypeString, Disabled: true},
		{Code: "file", Name: "File", Type: TypeFile},
		{Code: "date", Name: "Date", Type: TypeDate, Formatter: "YYYY-MM-DD"},
	}

	t.Run("only configured non-empty values", func(t *testing.T) {
		got := ExtendData{
			"a":       "v-a",
			"b":       2,
			"c":       nil,      // configured but nil -> omitted
			"d":       "v-d",    // configured but disabled -> omitted
			"file":    "v-file", // attachment -> omitted
			"unknown": "v-unk",  // not configured -> omitted
		}.ToMap(configs)

		assert.Equal(t, map[string]any{"a": "v-a", "b": 2}, got)
	})

	t.Run("nil config yields empty map", func(t *testing.T) {
		got := ExtendData{"a": "v-a"}.ToMap(nil)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("nil data yields empty map", func(t *testing.T) {
		got := ExtendData(nil).ToMap(configs)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("dates formatted per formatter", func(t *testing.T) {
		got := ExtendData{"date": "2026-09-10T13:05:00+08:00"}.ToMap(configs)
		assert.Equal(t, map[string]any{"date": "2026-09-10"}, got)
	})

	t.Run("date as time.Time formatted per formatter", func(t *testing.T) {
		got := ExtendData{"date": time.Date(2026, 9, 10, 13, 5, 0, 0, time.Local)}.ToMap(configs)
		assert.Equal(t, map[string]any{"date": "2026-09-10"}, got)
	})
}
