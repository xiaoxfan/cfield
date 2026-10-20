package cfield

import (
	"encoding/json"
)

// FieldBO is one code/value pair submitted by a front end.
type FieldBO struct {
	Code  string `json:"code"`
	Value any    `json:"value"`
}

// ExtendBO is the ordered list of custom field values submitted by a front
// end (a JSON array).
type ExtendBO []FieldBO

// ToVO converts the BO into a VO without touching values.
func (b ExtendBO) ToVO() ExtendVO {
	vo := make(ExtendVO, 0, len(b))
	for _, item := range b {
		vo = append(vo, FieldVO{
			Code:  item.Code,
			Value: item.Value,
		})
	}
	return vo
}

// DirectToDB converts the BO into an ExtendData map without schema
// validation; nil values are dropped.
func (b ExtendBO) DirectToDB() ExtendData {
	data := make(ExtendData, len(b))
	for _, item := range b {
		if item.Value != nil {
			data[item.Code] = item.Value
		}
	}
	return data
}

// Equals compares two ExtendBOs by code, order-independently, using
// CompareAny for value equality.
func (b ExtendBO) Equals(other ExtendBO) bool {
	return mapsEqual(b.toCodeValueMap(), other.toCodeValueMap())
}

func (b ExtendBO) toCodeValueMap() map[string]any {
	m := make(map[string]any, len(b))
	for _, item := range b {
		m[item.Code] = item.Value
	}
	return m
}

// MarshalJSON implements [json.Marshaler]; a nil BO marshals as an empty
// array so API responses stay stable.
func (b ExtendBO) MarshalJSON() ([]byte, error) {
	if b == nil {
		return []byte("[]"), nil
	}
	type plain ExtendBO
	return json.Marshal(plain(b))
}
