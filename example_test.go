package cfield_test

import (
	"fmt"
	"log"

	"gorm.io/gorm/clause"

	"github.com/xiaoxfan/cfield"
)

// The write path: validate and coerce a front-end payload before persisting.
func ExampleBindToDB() {
	configs := cfield.ExtendConfig{
		{Code: "region", Name: "Region", Type: cfield.TypeString, Required: true},
		{Code: "priority", Name: "Priority", Type: cfield.TypeInt},
	}

	data, err := cfield.BindToDB(cfield.ExtendBO{
		{Code: "region", Value: "EMEA"},
		{Code: "priority", Value: "2"}, // string coerced to int
	}, configs)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(data["region"], data["priority"])
	// Output: EMEA 2
}

// The read path: turn stored values into an API-friendly ordered list with
// dates rendered per the field's formatter.
func ExampleFormatToVO() {
	configs := cfield.ExtendConfig{
		{Code: "due_date", Name: "Due Date", Type: cfield.TypeDate, Formatter: "YYYY-MM-DD"},
	}
	dbData := cfield.ExtendData{"due_date": "2025-06-30T00:00:00Z"}

	for _, item := range cfield.FormatToVO(dbData, configs) {
		fmt.Println(item.Code, item.Name, item.Value)
	}
	// Output: due_date Due Date 2025-06-30
}

// Build JSONB-backed filter expressions for GORM.
func ExampleBuildQuery() {
	configs := cfield.ExtendConfig{
		{Code: "priority", Name: "Priority", Type: cfield.TypeInt},
	}

	exprs, err := cfield.BuildQuery(cfield.ExtendData{
		"priority": []any{1, 5}, // range filter
	}, configs, "orders.extend_data")
	if err != nil {
		log.Fatal(err)
	}

	sql := exprs[0].(clause.Expr)
	fmt.Println(sql.SQL)
	// Output: (orders.extend_data->>'priority' ~ ? AND (orders.extend_data->>'priority')::numeric BETWEEN ? AND ?)
}

// Validate a schema authored by an admin UI before persisting it.
func ExampleExtendConfig_Validate() {
	configs := cfield.ExtendConfig{
		{Code: "region", Name: "Region", Type: cfield.TypeString},
	}

	if err := configs.Validate(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("schema is valid")
	// Output: schema is valid
}
