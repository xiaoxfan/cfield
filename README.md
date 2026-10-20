# cfield

[![CI](https://github.com/xiaoxfan/cfield/actions/workflows/ci.yml/badge.svg)](https://github.com/xiaoxfan/cfield/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/xiaoxfan/cfield.svg)](https://pkg.go.dev/github.com/xiaoxfan/cfield)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8.svg)](go.mod)

**English** | [简体中文](README.zh-CN.md)

Schema-driven **custom (user-defined) fields** for Go services backed by
**GORM** and **PostgreSQL JSONB**.

Let every tenant of your SaaS/ERP define their own fields on business
entities — no schema migrations, no EAV tables, no denormalized columns.
One JSONB column per entity holds the values; a per-entity schema describes
how to validate, convert, query, compare and export them.

```go
type WorkOrder struct {
    ID     uint
    ...
    Ext    cfield.ExtendData    `gorm:"type:jsonb" json:"ext,omitempty"`
}
```

## Features

- **9 field types** — string (with max length), int, float (with precision),
  boolean, date (with layout), attachment (with size/type constraints),
  single-choice and multi-choice options
- **Strict write path** — `BindToDB` validates required fields, coerces types,
  resolves option labels to values and enforces attachment limits
- **Query building** — native JSONB operators (`->>`, `::numeric`,
  `jsonb_array_elements_text`) composed into GORM `clause.Expression`s:
  fuzzy/exact/batch text search, numeric ranges, date ranges, checkbox
  containment
- **Read path** — `FormatToVO` renders an ordered, front-end friendly list
  with dates formatted per field layout
- **Excel import/export** — header generation with per-field validation hints
  (via [excelize](https://github.com/qax-os/excelize)), label/value mapping,
  是/否 booleans and comma-separated multi-choice round-trips
- **Comparisons** — deep, type-aware equality that survives JSON round-trips
  (int vs float64, RFC 3339 vs formatted dates)
- **Readonly-after-create** — field-level locking: once a readonly field has
  a value it cannot be changed or cleared, but empty ones can still be filled
- **Legacy format tolerance** — reads the old `[{"code":…,"value":…}]` array
  format transparently

## Requirements

- Go 1.23+
- PostgreSQL with a `jsonb` column (query building; storage/validation/export
  work with any driver)
- [GORM](https://gorm.io) v1.26+ (only for `clause.Expression` composition —
  the values themselves go through plain `database/sql`)

> **Note on `lib/pq`**: cfield only uses `pq.Array` to build PostgreSQL array
> literals. It works with any PostgreSQL driver, including `pgx` in
> `database/sql` mode.

## Installation

```bash
go get github.com/xiaoxfan/cfield
```

## Quick start

### 1. Define the schema

The schema is plain data — store it per tenant/entity in a JSONB column and
edit it from an admin UI:

```go
configs := cfield.ExtendConfig{
    {Code: "region",    Name: "Region",    Type: cfield.TypeString, Required: true},
    {Code: "priority",  Name: "Priority",  Type: cfield.TypeInt},
    {Code: "due_date",  Name: "Due Date",  Type: cfield.TypeDate, Formatter: "YYYY-MM-DD"},
    {Code: "urgent",    Name: "Urgent",    Type: cfield.TypeBool},
    {Code: "material",  Name: "Material",  Type: cfield.TypeRadio,
        Options: []cfield.Option{{Label: "Steel", Value: "steel"}, {Label: "Alloy", Value: "alloy"}}},
    {Code: "tags",      Name: "Tags",      Type: cfield.TypeCheckbox,
        Options: []cfield.Option{{Label: "Export", Value: "export"}, {Label: "Rush", Value: "rush"}}},
    {Code: "drawing",   Name: "Drawing",   Type: cfield.TypeFile,
        FileConfig: &cfield.FileConfig{MaxSize: 10 << 20, Accept: ".pdf,.dwg"}},
}

// Validate before persisting a schema authored at runtime.
if err := configs.Validate(); err != nil {
    return err
}
```

`ExtendConfig` itself implements `driver.Valuer`/`sql.Scanner`, so it can be
stored the same way.

### 2. Store values

```go
// req.Fields is the JSON array [{"code":"region","value":"EMEA"}, …]
// submitted by the front end.
data, err := cfield.BindToDB(req.Fields, configs)
if err != nil {
    return err // e.g. cfield: required field [Region] is empty
}
order.Ext = data
db.Save(order)
```

`BindToDB` drops disabled fields, coerces types (`"2"` → `2`, `"是"` → `true`),
resolves option labels to values and enforces attachment constraints.

### 3. Read values back

```go
vo := cfield.FormatToVO(order.Ext, configs)
// [{"code":"due_date","name":"Due Date","value":"2025-06-30","required":false}, …]
```

Dates come back formatted with the field's layout, option fields are enriched
with their options, attachments are normalized to `[]FileInfo`.

### 4. Filter by custom fields

```go
filters := cfield.ExtendData{
    "region":   "EMEA",
    "priority": []any{1, 5},                      // numeric range
    "due_date": []string{"2025-06-01", "2025-06-30"},
    "tags":     []string{"rush"},                 // checkbox containment
}

exprs, err := cfield.BuildQuery(filters, configs, "work_orders.ext")
if err != nil {
    return err
}

var orders []WorkOrder
db.Clauses(clause.And(exprs...)).Find(&orders)
```

Generated SQL uses native JSONB operators, e.g.:

```sql
(work_orders.ext->>'priority') ~ '^-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?$'
  AND (work_orders.ext->>'priority')::numeric BETWEEN 1 AND 5
```

The regex guard keeps non-numeric junk in the column from failing the
`::numeric` cast.

### 5. Excel import/export

```go
// Import template: append headers (name + validation hint rows).
if err := configs.FilterDisabledAndFile().AppendHeader(f, sheet); err != nil { … }

// Export: values aligned with the header order, labels instead of raw values.
row := cfield.FormatToVO(order.Ext, configs).ToExportSlice(configs)
```

Template hints look like `必填，单选，请填写以下内容的一项: [Steel/Alloy]`, matching the
import conventions (是/否 booleans, comma-separated multi-choice, option labels
accepted besides raw values).

## Database schema (DDL)

cfield needs exactly two JSONB columns: one storing the schema per entity
(usually per tenant), one storing the values per row. Complete DDL and sample
data:

```sql
-- 1. Schema table: one row per tenant + entity type. The `configs` column
--    maps to cfield.ExtendConfig.
CREATE TABLE cfield_schema (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT      NOT NULL,
    entity_type VARCHAR(64) NOT NULL,               -- e.g. 'work_order'
    configs     JSONB       NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, entity_type)
);

-- 2. Business table: one JSONB column holds all custom field values.
--    The `ext` column maps to cfield.ExtendData.
CREATE TABLE work_order (
    id       BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL,
    ext      JSONB       NOT NULL DEFAULT '{}'
);

-- 3. Optional: expression indexes for fields you filter on frequently.
--    Equality / range queries on ->> text benefit from a btree index:
CREATE INDEX idx_work_order_region   ON work_order ((ext->>'region'));
CREATE INDEX idx_work_order_priority ON work_order (((ext->>'priority')::numeric));
```

Sample rows:

```sql
INSERT INTO cfield_schema (tenant_id, entity_type, configs) VALUES (
    1, 'work_order',
    '[
      {"code": "region",   "name": "Region",   "type": "string",  "required": true, "sort": 1},
      {"code": "priority", "name": "Priority", "type": "int",     "sort": 2},
      {"code": "due_date", "name": "Due Date", "type": "date",    "formatter": "YYYY-MM-DD", "sort": 3},
      {"code": "urgent",   "name": "Urgent",   "type": "boolean", "sort": 4},
      {"code": "material", "name": "Material", "type": "radio",   "sort": 5,
       "options": [{"label": "Steel", "value": "steel"}, {"label": "Alloy", "value": "alloy"}]},
      {"code": "tags",     "name": "Tags",     "type": "checkbox", "sort": 6,
       "options": [{"label": "Export", "value": "export"}, {"label": "Rush", "value": "rush"}]},
      {"code": "drawing",  "name": "Drawing",  "type": "attachment", "sort": 7,
       "fileConfig": {"maxSize": 10485760, "accept": ".pdf,.dwg"}}
    ]'::jsonb
);

INSERT INTO work_order (order_no, ext) VALUES (
    'WO-2025-001',
    '{
      "region":   "EMEA",
      "priority": 2,
      "due_date": "2025-06-30",
      "urgent":   true,
      "material": "steel",
      "tags":     ["export", "rush"]
    }'::jsonb
);
```

Matching GORM entities:

```go
type CFieldSchema struct {
    ID         int64               `gorm:"primaryKey"`
    TenantID   int64
    EntityType string
    Configs    cfield.ExtendConfig `gorm:"type:jsonb;not null;default:'[]'::jsonb"`
}

type WorkOrder struct {
    ID      int64              `gorm:"primaryKey"`
    OrderNo string
    Ext     cfield.ExtendData `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
}
```

> **Indexing notes**: the expression indexes above only serve the exact
> queries they mirror (`ext->>'region' = …`, `(ext->>'priority')::numeric
> BETWEEN …`). Fuzzy `LIKE '%…%'` searches cannot use a btree index. A plain
> `GIN (ext)` index only helps `@>` containment operators, which `BuildQuery`
> does not generate — create expression indexes for your hot filter fields
> instead.

## Field types

| Type | Coerced to | Extras |
|---|---|---|
| `string` | `string` | `maxLength` (runes), `textArea`, `batchQuery` |
| `int` | `int` | |
| `float` / `number` | `float64` | `precision` |
| `boolean` | `bool` | accepts 是/对/有/真/1 and 否/错/无/假/0 |
| `date` | `time.Time` | `formatter`: `YYYY-MM-DD HH:mm:ss`, `YYYY-MM-DD`, `YYYY-MM`, `YYYY` |
| `attachment` | `[]FileInfo` | `maxSize` (bytes), `accept` (extensions or MIME) |
| `radio` | `string` | fixed options; labels accepted on input, values stored |
| `checkbox` | `[]string` | fixed options, no duplicates |

## Readonly-after-create

```go
// On update, pass the data currently stored in the database.
data, err := cfield.BindToDBWithEditCheck(req.Fields, configs, order.Ext)
```

Fields configured with `EditCondition: cfield.ReadonlyAfterCreate` are locked
once they hold a value: they can no longer be changed or cleared, but fields
that are still empty may be filled in later (avoids dead-locking records
whose values were missing at creation).

## Comparison helpers

```go
cfield.CompareAny(18, float64(18))                                   // true
cfield.CompareWithConfig("2025-06-30", "2025-06-30T00:00:00Z", conf) // true (date layout)
order.Ext.Equals(other.Ext)                                          // deep map equality
```

## Security notes

- Field codes and column names are interpolated into SQL fragments. They must
  match `^[A-Za-z_][A-Za-z0-9_]*$` (column may be `table.column`). Both
  `ExtendConfig.Validate` and `BuildQuery` enforce this — always validate
  schemas authored at runtime.
- All values are bound as query parameters; no user data is interpolated.

## Limitations

- Query building targets PostgreSQL JSONB only.
- Excel import hints and the 是/否 boolean aliases are oriented at Chinese
  business documents; both are easy to adapt if needed.

## License

[MIT](LICENSE)
