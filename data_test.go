package cfield

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtendData_UnmarshalJSON(t *testing.T) {
	type TestEntity struct {
		ID   int        `json:"id"`
		Name string     `json:"name"`
		Ext  ExtendData `json:"ext"`
	}

	tests := []struct {
		name     string
		jsonStr  string
		expected map[string]any
		wantErr  bool
	}{
		{
			name:    "basic types",
			jsonStr: `{"id": 1, "name": "test1", "ext": {"age": 18, "score": 99.5, "active": true}}`,
			expected: map[string]any{
				"age":    float64(18),
				"score":  99.5,
				"active": true,
			},
		},
		{
			name:    "checkbox array value",
			jsonStr: `{"id": 3, "ext": {"hobbies": ["reading", "coding"]}}`,
			expected: map[string]any{
				"hobbies": []any{"reading", "coding"},
			},
		},
		{
			name:    "attachment object value",
			jsonStr: `{"id": 4, "ext": {"resume": {"fileName": "cv.pdf", "fileSize": 102400}}}`,
			expected: map[string]any{
				"resume": map[string]any{"fileName": "cv.pdf", "fileSize": float64(102400)},
			},
		},
		{
			name:     "empty object",
			jsonStr:  `{"id": 6, "ext": {}}`,
			expected: map[string]any{},
		},
		{
			name:     "null",
			jsonStr:  `{"id": 7, "ext": null}`,
			expected: nil,
		},
		{
			name:    "legacy array format",
			jsonStr: `{"id": 8, "ext": [{"code": "a", "value": 1}, {"code": "b", "value": "x"}]}`,
			expected: map[string]any{
				"a": float64(1),
				"b": "x",
			},
		},
		{
			name:    "plain string is rejected",
			jsonStr: `{"ext": "invalid_format"}`,
			wantErr: true,
		},
		{
			name:    "plain number is rejected",
			jsonStr: `{"ext": 123}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entity TestEntity
			err := json.Unmarshal([]byte(tt.jsonStr), &entity)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			if tt.expected == nil {
				assert.Empty(t, entity.Ext)
				return
			}
			assert.True(t, reflect.DeepEqual(map[string]any(entity.Ext), tt.expected),
				"got %#v, want %#v", entity.Ext, tt.expected)
		})
	}
}

func TestExtendData_ValueScan(t *testing.T) {
	t.Run("value", func(t *testing.T) {
		v, err := ExtendData{"a": 1}.Value()
		assert.NoError(t, err)
		assert.Equal(t, []byte(`{"a":1}`), v)

		v, err = ExtendData{}.Value()
		assert.NoError(t, err)
		assert.Equal(t, []byte("{}"), v)
	})

	t.Run("scan object", func(t *testing.T) {
		var d ExtendData
		assert.NoError(t, d.Scan([]byte(`{"a": 1, "b": "x"}`)))
		assert.Equal(t, ExtendData{"a": float64(1), "b": "x"}, d)
	})

	t.Run("scan legacy array", func(t *testing.T) {
		var d ExtendData
		assert.NoError(t, d.Scan([]byte(`[{"code":"a","value":1}]`)))
		assert.Equal(t, ExtendData{"a": float64(1)}, d)
	})

	t.Run("scan nil", func(t *testing.T) {
		d := ExtendData{"a": 1}
		assert.NoError(t, d.Scan(nil))
		assert.Empty(t, d)
	})

	t.Run("scan empty text", func(t *testing.T) {
		var d ExtendData
		assert.NoError(t, d.Scan([]byte("")))
		assert.Empty(t, d)
	})

	t.Run("scan invalid type", func(t *testing.T) {
		var d ExtendData
		assert.Error(t, d.Scan(42))
	})

	t.Run("scan invalid json", func(t *testing.T) {
		var d ExtendData
		assert.Error(t, d.Scan([]byte(`{"a":`)))
	})

	t.Run("scan legacy invalid json", func(t *testing.T) {
		var d ExtendData
		assert.Error(t, d.Scan([]byte(`[{"code":`)))
	})

	t.Run("round trip", func(t *testing.T) {
		original := ExtendData{"name": "zhang", "tags": []any{"a", "b"}}
		v, err := original.Value()
		assert.NoError(t, err)
		assert.Implements(t, (*driver.Valuer)(nil), original)

		var restored ExtendData
		assert.NoError(t, restored.Scan(v))
		assert.True(t, restored.Equals(original))
	})
}

func TestExtendData_MarshalJSON(t *testing.T) {
	// Values are marshaled verbatim, without any date magic.
	d := ExtendData{"note": "2024-01-02T03:04:05Z", "n": 1}
	b, err := json.Marshal(d)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"note":"2024-01-02T03:04:05Z","n":1}`, string(b))
}

func TestExtendData_Equals(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		assert.True(t, ExtendData(nil).Equals(nil))
	})

	t.Run("one nil", func(t *testing.T) {
		assert.False(t, ExtendData{"a": 1}.Equals(nil))
		assert.False(t, ExtendData(nil).Equals(ExtendData{"a": 1}))
	})

	t.Run("whitespace-only strings equal nil", func(t *testing.T) {
		assert.True(t, ExtendData{"a": nil}.Equals(ExtendData{"a": "  "}))
	})

	t.Run("equal nested values", func(t *testing.T) {
		assert.True(t, ExtendData{
			"a": 1, "b": []any{"x", true}, "c": map[string]any{"k": "v"},
		}.Equals(ExtendData{
			"a": float64(1), "b": []any{"x", true}, "c": map[string]any{"k": "v"},
		}))
	})

	t.Run("different values", func(t *testing.T) {
		assert.False(t, ExtendData{"a": 1}.Equals(ExtendData{"a": 2}))
		assert.False(t, ExtendData{"a": 1}.Equals(ExtendData{"b": 1}))
	})
}
