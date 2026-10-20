package cfield

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func validFullConfigs() ExtendConfig {
	return ExtendConfig{
		{Code: "name", Name: "Name", Type: TypeString, Required: true},
		{Code: "age", Name: "Age", Type: TypeInt},
		{Code: "score", Name: "Score", Type: TypeNumber},
		{Code: "is_student", Name: "Student", Type: TypeBool},
		{Code: "birthday", Name: "Birthday", Type: TypeDate, Formatter: "YYYY-MM-DD"},
		{Code: "gender", Name: "Gender", Type: TypeRadio, Required: true,
			Options: []Option{{Label: "Male", Value: "male"}, {Label: "Female", Value: "female"}}},
		{Code: "hobbies", Name: "Hobbies", Type: TypeCheckbox,
			Options: []Option{{Label: "Reading", Value: "read"}, {Label: "Sports", Value: "sport"}}},
		{Code: "resume", Name: "Resume", Type: TypeFile, Required: true,
			FileConfig: &FileConfig{MaxSize: 1024 * 1024, Accept: ".pdf,.doc"}},
		{Code: "disabled_field", Name: "Disabled", Type: TypeString, Disabled: true},
	}
}

func fileBO(size int64, name string) FieldBO {
	return FieldBO{Code: "resume", Value: []map[string]any{{
		"fileId": 12, "fileName": name, "fileSize": size, "fileType": "application/pdf", "objKey": "k1",
	}}}
}

func TestBindToDB(t *testing.T) {
	configs := validFullConfigs()

	t.Run("happy path with coercion", func(t *testing.T) {
		data, err := BindToDB(ExtendBO{
			{Code: "name", Value: "zhang"},
			{Code: "age", Value: "18"},
			{Code: "score", Value: 99.5},
			{Code: "is_student", Value: "是"},
			{Code: "birthday", Value: "2000-01-01"},
			{Code: "gender", Value: "male"},
			{Code: "hobbies", Value: []string{"read", "sport"}},
			fileBO(1024, "cv.pdf"),
		}, configs)

		assert.NoError(t, err)
		assert.Equal(t, "zhang", data["name"])
		assert.Equal(t, 18, data["age"])
		assert.Equal(t, 99.5, data["score"])
		assert.Equal(t, true, data["is_student"])
		assert.IsType(t, time.Time{}, data["birthday"])
		assert.Equal(t, "male", data["gender"])
		assert.Equal(t, []string{"read", "sport"}, data["hobbies"])
		assert.IsType(t, []FileInfo{}, data["resume"])
	})

	t.Run("multiple files", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "zhang"},
			{Code: "gender", Value: "male"},
			{Code: "resume", Value: []map[string]any{
				{"fileId": 12, "fileName": "cv.pdf", "fileSize": 1024, "fileType": "application/pdf", "objKey": "k1"},
				{"fileId": 13, "fileName": "cv2.pdf", "fileSize": 2048, "fileType": "application/pdf", "objKey": "k2"},
			}},
		}, configs)
		assert.NoError(t, err)
	})

	t.Run("missing required field", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{{Code: "age", Value: 18}}, configs)
		assert.ErrorContains(t, err, "required field [Name] is empty")
	})

	t.Run("invalid int", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "male"},
			{Code: "age", Value: "not_a_number"}, fileBO(1024, "cv.pdf"),
		}, configs)
		assert.ErrorContains(t, err, "field [Age]")
	})

	t.Run("radio value not in options", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "unknown"}, fileBO(1024, "cv.pdf"),
		}, configs)
		assert.ErrorContains(t, err, "not one of the valid options")
	})

	t.Run("radio accepts label", func(t *testing.T) {
		data, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "Female"}, fileBO(1024, "cv.pdf"),
		}, configs)
		assert.NoError(t, err)
		assert.Equal(t, "female", data["gender"])
	})

	t.Run("checkbox value not in options", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "male"},
			{Code: "hobbies", Value: []string{"invalid"}}, fileBO(1024, "cv.pdf"),
		}, configs)
		assert.ErrorContains(t, err, "not one of the valid options")
	})

	t.Run("checkbox duplicate value", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "male"},
			{Code: "hobbies", Value: "read,read"}, fileBO(1024, "cv.pdf"),
		}, configs)
		assert.ErrorContains(t, err, "duplicate value")
	})

	t.Run("file exceeds size limit", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "male"},
			fileBO(2*1024*1024, "cv.pdf"),
		}, configs)
		assert.ErrorContains(t, err, "exceeds the size limit")
	})

	t.Run("file type not accepted", func(t *testing.T) {
		_, err := BindToDB(ExtendBO{
			{Code: "name", Value: "li"}, {Code: "gender", Value: "male"},
			fileBO(1024, "cv.exe"),
		}, configs)
		assert.ErrorContains(t, err, "unsupported type")
	})

	t.Run("disabled field ignored", func(t *testing.T) {
		data, err := BindToDB(ExtendBO{
			{Code: "name", Value: "zhang"}, {Code: "gender", Value: "male"},
			fileBO(1024, "cv.pdf"), {Code: "disabled_field", Value: "ignored"},
		}, configs)
		assert.NoError(t, err)
		_, ok := data["disabled_field"]
		assert.False(t, ok, "disabled fields must not be persisted")
	})

	t.Run("empty optional fields are cleared", func(t *testing.T) {
		data, err := BindToDB(ExtendBO{
			{Code: "name", Value: "zhang"}, {Code: "gender", Value: "male"},
			{Code: "age", Value: ""}, fileBO(1024, "cv.pdf"),
		}, configs)
		assert.NoError(t, err)
		assert.Nil(t, data["age"])
	})

	t.Run("string max length enforced in runes", func(t *testing.T) {
		conf := ExtendConfig{{Code: "note", Name: "Note", Type: TypeString, UseMaxLength: true, MaxLength: 3}}
		_, err := BindToDB(ExtendBO{{Code: "note", Value: "你好世界"}}, conf)
		assert.ErrorContains(t, err, "exceeds the limit of 3")

		_, err = BindToDB(ExtendBO{{Code: "note", Value: "你好吗"}}, conf)
		assert.NoError(t, err)
	})
}

func TestValidateData(t *testing.T) {
	configs := validFullConfigs()

	valid := ExtendData{
		"name": "zhang", "age": 18, "score": 99.5, "is_student": true,
		"birthday": "2000-01-01", "gender": "male", "hobbies": []string{"read"},
		"resume": []map[string]any{{"fileId": 12, "fileName": "cv.pdf", "fileSize": 1024}},
	}
	assert.NoError(t, ValidateData(valid, configs))

	assert.Error(t, ValidateData(nil, configs), "nil data with required fields must fail")

	minimal := ExtendData{"name": "zhang", "gender": "male",
		"resume": []map[string]any{{"fileId": 12, "fileName": "cv.pdf", "fileSize": 1024}}}
	assert.NoError(t, ValidateData(minimal, configs))

	emptyRequired := ExtendData{"age": 18}
	err := ValidateData(emptyRequired, configs)
	assert.ErrorContains(t, err, "required field [Name] is empty")

	badType := ExtendData{"name": "zhang", "age": "not_a_number", "gender": "male",
		"resume": []map[string]any{{"fileId": 12, "fileName": "cv.pdf", "fileSize": 1024}}}
	err = ValidateData(badType, configs)
	assert.ErrorContains(t, err, "field [Age]")

	rfc3339Date := ExtendData{"name": "zhang", "gender": "male",
		"birthday": "2000-01-01T00:00:00+08:00",
		"resume":   []map[string]any{{"fileId": 12, "fileName": "cv.pdf", "fileSize": 1024}}}
	assert.NoError(t, ValidateData(rfc3339Date, configs), "RFC 3339 strings survive JSON round-trips")
}

func TestFormatToVO(t *testing.T) {
	configs := ExtendConfig{
		{Code: "name", Name: "Name", Type: TypeString},
		{Code: "birthday", Name: "Birthday", Type: TypeDate, Formatter: "YYYY-MM-DD"},
		{Code: "file", Name: "File", Type: TypeFile, FileConfig: &FileConfig{MaxSize: 100}},
		{Code: "gender", Name: "Gender", Type: TypeRadio, Options: []Option{{Label: "Male", Value: "male"}}},
		{Code: "hidden", Name: "Hidden", Type: TypeString, Disabled: true},
	}

	// Simulate data loaded from JSON: dates arrive as strings.
	dbDataFromJSON := ExtendData{
		"name":     "wang",
		"birthday": "2023-10-01T00:00:00Z",
		"file":     []map[string]any{{"fileId": 12, "fileName": "test.txt", "fileSize": 100, "objKey": "k2"}},
		"gender":   "male",
		"hidden":   "secret",
	}

	vo := FormatToVO(dbDataFromJSON, configs)

	assert.Len(t, vo, 4, "disabled fields are skipped")
	byCode := map[string]FieldVO{}
	for _, item := range vo {
		byCode[item.Code] = item
	}

	assert.Equal(t, "wang", byCode["name"].Value)
	assert.Equal(t, "2023-10-01", byCode["birthday"].Value, "dates use the configured layout")
	assert.Equal(t, "Name", byCode["name"].Name)
	assert.True(t, byCode["name"].Required == false)

	files, ok := byCode["file"].Value.([]FileInfo)
	if assert.True(t, ok) {
		assert.Len(t, files, 1)
		assert.Equal(t, "test.txt", files[0].FileName)
	}
	assert.NotNil(t, byCode["file"].FileConfig)

	assert.Equal(t, []Option{{Label: "Male", Value: "male"}}, byCode["gender"].Options)

	_, exists := byCode["hidden"]
	assert.False(t, exists, "disabled fields must not appear")

	t.Run("nil map", func(t *testing.T) {
		vo := FormatToVO(nil, configs)
		assert.Len(t, vo, 4)
		for _, item := range vo {
			assert.Nil(t, item.Value)
		}
	})
}

func TestVOToDB(t *testing.T) {
	configs := ExtendConfig{
		{Code: "a", Name: "A", Type: TypeString},
		{Code: "b", Name: "B", Type: TypeString, Disabled: true},
	}
	data := VOToDB(ExtendVO{
		{Code: "a", Value: "x"},
		{Code: "b", Value: "y"},
		{Code: "unknown", Value: "z"},
	}, configs)
	assert.Equal(t, ExtendData{"a": "x"}, data)
}

func TestConvertValue_FloatPrecision(t *testing.T) {
	conf := FieldConfig{Code: "score", Name: "Score", Type: TypeFloat, UsePrecision: true, Precision: 2}

	res, err := convertValue("12.3456", conf)
	assert.NoError(t, err)
	assert.Equal(t, 12.35, res)

	conf.Precision = 0
	res, err = convertValue("12.6", conf)
	assert.NoError(t, err)
	assert.Equal(t, 13.0, res)
}

func TestConvertValue_BoolAliases(t *testing.T) {
	conf := FieldConfig{Code: "b", Type: TypeBool}

	for _, input := range []any{"是", "对", "有", "真", "1", true, 1} {
		res, err := convertValue(input, conf)
		assert.NoError(t, err)
		assert.Equal(t, true, res, "input %v", input)
	}
	for _, input := range []any{"否", "错", "无", "假", "0", "", false, 0} {
		res, err := convertValue(input, conf)
		assert.NoError(t, err)
		assert.Equal(t, false, res, "input %v", input)
	}

	_, err := convertValue("maybe", conf)
	assert.Error(t, err)
}

func TestConvertValue_File(t *testing.T) {
	conf := FieldConfig{Code: "f", Name: "F", Type: TypeFile,
		FileConfig: &FileConfig{MaxSize: 100, Accept: ".txt"}}

	res, err := convertValue([]map[string]any{{"fileId": 12, "fileName": "a.txt", "fileSize": 50, "objKey": "k"}}, conf)
	assert.NoError(t, err)
	assert.IsType(t, []FileInfo{}, res)

	_, err = convertValue([]map[string]any{{"fileId": 12, "fileName": "a.txt", "fileSize": 150, "objKey": "k"}}, conf)
	assert.ErrorContains(t, err, "exceeds the size limit")

	_, err = convertValue([]map[string]any{{"fileId": 12, "fileName": "a.png", "fileSize": 50, "objKey": "k"}}, conf)
	assert.ErrorContains(t, err, "unsupported type")

	_, err = convertValue("not json", conf)
	assert.ErrorContains(t, err, "invalid attachment payload")

	_, err = convertValue([]map[string]any{{"fileName": "a.txt", "fileSize": 50}}, conf)
	assert.ErrorContains(t, err, "fileId and fileName are required")
}

func TestConvertValue_FileAcceptMime(t *testing.T) {
	conf := FieldConfig{Code: "f", Name: "F", Type: TypeFile,
		FileConfig: &FileConfig{Accept: "image/*"}}

	_, err := convertValue([]map[string]any{{"fileId": 1, "fileName": "a.png", "fileSize": 1, "fileType": "image/png"}}, conf)
	assert.NoError(t, err)

	_, err = convertValue([]map[string]any{{"fileId": 1, "fileName": "a.pdf", "fileSize": 1, "fileType": "application/pdf"}}, conf)
	assert.Error(t, err)
}
