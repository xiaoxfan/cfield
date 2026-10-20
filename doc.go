// Package cfield implements schema-driven custom (user-defined) fields on top
// of PostgreSQL JSONB columns and GORM.
//
// A typical SaaS or ERP application needs to let each tenant define extra
// fields on business entities (orders, work orders, customers, ...) without
// schema migrations. cfield solves this with three building blocks:
//
//   - [ExtendConfig]: the per-entity field schema (code, name, type, options,
//     validation rules). Usually stored as a JSONB column and edited by an
//     admin UI.
//   - [ExtendData]: the per-row field values, stored as a single JSONB map
//     column. It implements [driver.Valuer] and [sql.Scanner] so GORM reads
//     and writes it transparently.
//   - A set of pure functions that validate, convert, compare, query and
//     export field values according to the schema.
//
// # Write path
//
// Incoming payloads arrive as [ExtendBO] (an ordered code/value list that maps
// naturally to a JSON array from the front end). [BindToDB] validates every
// value against the schema, coerces it to its canonical Go type and produces
// the [ExtendData] map to persist:
//
//	data, err := cfield.BindToDB(req.Fields, configs)
//	if err != nil {
//		return err
//	}
//	order.Extend = data
//	db.Save(order)
//
// # Read path
//
// [FormatToVO] converts the stored map back into an ordered [ExtendVO] suitable
// for API responses, formatting dates and enriching option fields:
//
//	vo := cfield.FormatToVO(order.Extend, configs)
//
// # Query path
//
// [BuildQuery] turns filter values into GORM [gorm.io/gorm/clause.Expression]s
// backed by native JSONB operators (->>, ::numeric casts, jsonb_array_elements_text):
//
//	exprs, err := cfield.BuildQuery(filters, configs, "extend_data")
//	if err != nil {
//		return err
//	}
//	db.Where(clause.And(exprs...)).Find(&orders)
//
// # Excel import/export
//
// The excelize-based helpers generate import templates with validation hints
// and format rows for export. See [ExtendConfig.AppendHeader],
// [ExtendVO.ToExportSlice] and [FieldConfig.GetValidationText].
//
// # Limitations
//
//   - Query building targets PostgreSQL only. Storage, validation and export
//     work on any database that stores JSON in a text-compatible column.
//   - Excel import templates and boolean parsing ("是"/"否") are oriented at
//     Chinese business users; see the README for details.
package cfield
