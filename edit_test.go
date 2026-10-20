package cfield

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func editCheckConfigs() ExtendConfig {
	return ExtendConfig{
		{Code: "name", Name: "Name", Type: TypeString},
		{Code: "discount", Name: "Discount", Type: TypeInt, EditCondition: EditableAfterCreate},
		{Code: "promised_date", Name: "Promised Date", Type: TypeDate, Formatter: "YYYY-MM-DD", EditCondition: ReadonlyAfterCreate},
		{Code: "remark", Name: "Remark", Type: TypeString, EditCondition: ReadonlyAfterCreate},
	}
}

func TestGetReadonlyFieldCodes(t *testing.T) {
	assert.Equal(t, []string{"promised_date", "remark"}, GetReadonlyFieldCodes(editCheckConfigs()))
}

func TestBindToDBWithEditCheck(t *testing.T) {
	configs := editCheckConfigs()

	t.Run("create path skips readonly check", func(t *testing.T) {
		_, err := BindToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-29"},
			{Code: "remark", Value: "hello"},
		}, configs, nil)
		assert.NoError(t, err)
	})

	t.Run("unchanged readonly fields pass", func(t *testing.T) {
		original := ExtendData{"promised_date": "2026-08-29T00:00:00+08:00", "remark": "hello"}
		_, err := BindToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "discount", Value: 20},
			{Code: "promised_date", Value: "2026-08-29"},
			{Code: "remark", Value: "hello"},
		}, configs, original)
		assert.NoError(t, err)
	})

	t.Run("changed readonly field fails", func(t *testing.T) {
		original := ExtendData{"promised_date": "2026-08-29T00:00:00+08:00", "remark": "hello"}
		_, err := BindToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-30"},
			{Code: "remark", Value: "hello"},
		}, configs, original)
		assert.ErrorContains(t, err, "[Promised Date] is readonly after create")
	})

	t.Run("date compares across representations", func(t *testing.T) {
		original := ExtendData{
			"promised_date": time.Date(2026, 8, 29, 0, 0, 0, 0, time.Local),
			"remark":        "hello",
		}
		_, err := BindToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-29"},
			{Code: "remark", Value: "hello"},
		}, configs, original)
		assert.NoError(t, err)
	})

	t.Run("cleared readonly value fails", func(t *testing.T) {
		original := ExtendData{"promised_date": "2026-08-29T00:00:00+08:00", "remark": "hello"}
		_, err := BindToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-29"},
		}, configs, original)
		assert.ErrorContains(t, err, "[Remark] is readonly after create")
	})

	t.Run("empty readonly value may be filled later", func(t *testing.T) {
		// Field-level locking: an empty original value can still be filled in.
		original := ExtendData{"promised_date": "2026-08-29T00:00:00+08:00"}
		_, err := BindToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-29"},
			{Code: "remark", Value: "hello"},
		}, configs, original)
		assert.NoError(t, err)
	})

	t.Run("validation errors surface unchanged", func(t *testing.T) {
		strict := ExtendConfig{{Code: "name", Name: "Name", Type: TypeString, Required: true}}
		_, err := BindToDBWithEditCheck(ExtendBO{}, strict, nil)
		assert.ErrorContains(t, err, "required field [Name] is empty")
	})
}

func TestDirectToDBWithEditCheck(t *testing.T) {
	configs := editCheckConfigs()

	t.Run("create path skips readonly check", func(t *testing.T) {
		_, err := DirectToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-29"},
		}, configs, nil)
		assert.NoError(t, err)
	})

	t.Run("editable fields pass through", func(t *testing.T) {
		original := ExtendData{"promised_date": "2026-08-29T00:00:00+08:00", "remark": "hello"}
		data, err := DirectToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "renamed"},
			{Code: "promised_date", Value: "2026-08-29"},
			{Code: "remark", Value: "hello"},
		}, configs, original)
		assert.NoError(t, err)
		assert.Equal(t, "renamed", data["name"])
	})

	t.Run("changed readonly field fails", func(t *testing.T) {
		original := ExtendData{"promised_date": "2026-08-29T00:00:00+08:00", "remark": "hello"}
		_, err := DirectToDBWithEditCheck(ExtendBO{
			{Code: "name", Value: "test"},
			{Code: "promised_date", Value: "2026-08-30"},
			{Code: "remark", Value: "hello"},
		}, configs, original)
		assert.ErrorContains(t, err, "[Promised Date] is readonly after create")
	})
}
