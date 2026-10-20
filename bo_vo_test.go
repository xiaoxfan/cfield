package cfield

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtendBO(t *testing.T) {
	bo := ExtendBO{
		{Code: "name", Value: "zhang"},
		{Code: "age", Value: 18},
		{Code: "empty", Value: nil},
	}

	t.Run("ToVO", func(t *testing.T) {
		vo := bo.ToVO()
		assert.Equal(t, ExtendVO{
			{Code: "name", Value: "zhang"},
			{Code: "age", Value: 18},
			{Code: "empty", Value: nil},
		}, vo)
	})

	t.Run("DirectToDB drops nil values", func(t *testing.T) {
		assert.Equal(t, ExtendData{"name": "zhang", "age": 18}, bo.DirectToDB())
	})

	t.Run("MarshalJSON nil becomes empty array", func(t *testing.T) {
		b, err := json.Marshal(ExtendBO(nil))
		assert.NoError(t, err)
		assert.Equal(t, "[]", string(b))
	})

	t.Run("Equals order independent", func(t *testing.T) {
		bo := ExtendBO{
			{Code: "name", Value: "zhang"},
			{Code: "age", Value: 18},
		}
		other := ExtendBO{
			{Code: "age", Value: float64(18)}, // int vs float64 compares equal
			{Code: "name", Value: " zhang "},
		}
		assert.True(t, bo.Equals(other))
		assert.False(t, bo.Equals(ExtendBO{{Code: "name", Value: "li"}}))
		assert.True(t, ExtendBO(nil).Equals(nil))
	})
}

func TestExtendVO(t *testing.T) {
	vo := ExtendVO{
		{Code: "name", Value: "zhang"},
		{Code: "age", Value: 18},
	}

	t.Run("DirectToDB drops nil values", func(t *testing.T) {
		assert.Equal(t, ExtendData{"name": "zhang", "age": 18}, vo.DirectToDB())
	})

	t.Run("ToExportBO", func(t *testing.T) {
		assert.Equal(t, ExtendBO{
			{Code: "name", Value: "zhang"},
			{Code: "age", Value: 18},
		}, vo.ToExportBO())
	})

	t.Run("MarshalJSON nil becomes empty array", func(t *testing.T) {
		b, err := json.Marshal(ExtendVO(nil))
		assert.NoError(t, err)
		assert.Equal(t, "[]", string(b))
	})

	t.Run("Equals", func(t *testing.T) {
		other := ExtendVO{{Code: "age", Value: 18}, {Code: "name", Value: "zhang"}}
		assert.True(t, vo.Equals(other))
		assert.False(t, vo.Equals(ExtendVO{{Code: "name", Value: "li"}}))
	})
}
