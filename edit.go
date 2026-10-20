package cfield

import (
	"fmt"
)

// GetReadonlyFieldCodes returns the codes of fields configured as
// readonly-after-create.
func GetReadonlyFieldCodes(configs ExtendConfig) []string {
	result := make([]string, 0)
	for _, conf := range configs {
		if conf.Disabled {
			continue
		}
		if conf.EditCondition == ReadonlyAfterCreate {
			result = append(result, conf.Code)
		}
	}
	return result
}

// BindToDBWithEditCheck behaves like BindToDB and additionally enforces the
// readonly-after-create rule. Pass the original ExtendData loaded from the
// database, or nil on the create path to skip the check.
//
// Locking semantics (field-level): once a readonly field holds a value it is
// locked and can no longer be changed or cleared, but a field that is still
// empty may be filled in later. This avoids dead-locking records whose
// required fields were empty at creation time.
func BindToDBWithEditCheck(items ExtendBO, configs ExtendConfig, originalData ExtendData) (ExtendData, error) {
	newData, err := BindToDB(items, configs)
	if err != nil {
		return nil, err
	}
	if err := checkReadonlyFields(newData, configs, originalData); err != nil {
		return nil, err
	}
	return newData, nil
}

// DirectToDBWithEditCheck converts the submitted ExtendBO into ExtendData
// without type validation (like ExtendBO.DirectToDB) but still enforces the
// readonly-after-create rule. Use it on edit paths that only need the
// readonly guarantee, e.g. when values were validated elsewhere.
func DirectToDBWithEditCheck(items ExtendBO, configs ExtendConfig, originalData ExtendData) (ExtendData, error) {
	newData := items.DirectToDB()
	if err := checkReadonlyFields(newData, configs, originalData); err != nil {
		return nil, err
	}
	return newData, nil
}

func checkReadonlyFields(newData ExtendData, configs ExtendConfig, originalData ExtendData) error {
	if originalData == nil {
		return nil // create path
	}

	for _, conf := range configs {
		if conf.Disabled || conf.EditCondition != ReadonlyAfterCreate {
			continue
		}

		// Field-level lock: empty original values may still be filled in.
		if isEmptyQueryValue(originalData[conf.Code]) {
			continue
		}

		if !CompareWithConfig(newData[conf.Code], originalData[conf.Code], conf) {
			return fmt.Errorf("cfield: field [%s] is readonly after create", conf.Name)
		}
	}
	return nil
}
