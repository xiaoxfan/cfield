# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-11-19

### Added

- Schema-driven custom fields with 9 field types: string (max length, batch
  query), int, float/number (precision), boolean, date (layouts), attachment
  (size/type constraints), radio and checkbox options.
- `ExtendData` JSONB storage type with GORM `Valuer`/`Scanner` support and
  transparent legacy array-format compatibility.
- `BindToDB` / `ValidateData` write-path validation and type coercion.
- `FormatToVO` read-path rendering with per-field date layouts.
- `BuildQuery` PostgreSQL JSONB filter expressions for GORM (text fuzzy/exact/
  batch, numeric and date ranges, checkbox containment), with regex guards
  for `::numeric` casts.
- `ExtendConfig.Validate` schema validation, including SQL-safe field codes.
- Excel import/export helpers via excelize: header generation with validation
  hints, label/value mapping and export slices.
- Readonly-after-create enforcement with field-level locking semantics.
- Deep, round-trip-safe value comparison (`CompareAny`, `CompareWithConfig`).
