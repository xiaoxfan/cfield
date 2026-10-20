# cfield

[![CI](https://github.com/xiaoxfan/cfield/actions/workflows/ci.yml/badge.svg)](https://github.com/xiaoxfan/cfield/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/xiaoxfan/cfield.svg)](https://pkg.go.dev/github.com/xiaoxfan/cfield)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8.svg)](go.mod)

[English](README.md) | **简体中文**

基于 **GORM** + **PostgreSQL JSONB** 的**自定义（用户定义）字段**方案。

让 SaaS/ERP 的每个租户在业务实体上定义自己的字段——无需表结构迁移、无需 EAV 表、无需加列。
每个实体只需一个 JSONB 列存值，一套按实体维护的 Schema 负责校验、转换、查询、比较和导出。

```go
type WorkOrder struct {
    ID   uint
    ...
    Ext  cfield.ExtendData `gorm:"type:jsonb" json:"ext,omitempty"`
}
```

## 特性

- **9 种字段类型** —— 文本（限长）、整数、小数（精度）、布尔、日期（格式）、附件（大小/类型限制）、单选、多选
- **严格的写入链路** —— `BindToDB` 校验必填、强转类型、选项 label 归一化为 value、附件约束校验
- **查询构建** —— 原生 JSONB 操作符（`->>`、`::numeric`、`jsonb_array_elements_text`）组装成 GORM `clause.Expression`：文本模糊/精确/批量搜索、数字区间、日期区间、多选包含匹配
- **读取链路** —— `FormatToVO` 输出有序的、前端友好的列表，日期按字段配置的格式渲染
- **Excel 导入/导出** —— 基于 [excelize](https://github.com/qax-os/excelize) 生成带校验提示的表头，label/value 双向映射，支持 是/否 布尔与逗号分隔多选的往返
- **值比较** —— 深度、类型感知的相等判断，能扛住 JSON round-trip（int vs float64、RFC 3339 vs 格式化日期）
- **创建后只读** —— 字段级锁定：只读字段一旦有值便不可修改或清空，但尚为空的字段仍可补填
- **旧格式兼容** —— 透明读取旧版 `[{"code":…,"value":…}]` 数组格式

## 环境要求

- Go 1.23+
- PostgreSQL 且实体表有 `jsonb` 列（查询构建强依赖 PG；存储/校验/导出对驱动无要求）
- [GORM](https://gorm.io) v1.26+（仅用于 `clause.Expression` 组合——值本身走标准 `database/sql` 接口）

> **关于 `lib/pq`**：cfield 只使用 `pq.Array` 生成 PostgreSQL 数组字面量，
> 兼容任何 PG 驱动（包括 `database/sql` 模式下的 `pgx`）。

## 安装

```bash
go get github.com/xiaoxfan/cfield
```

## 快速上手

### 1. 定义字段 Schema

Schema 就是纯数据——按租户/实体存到 JSONB 列里，由管理端 UI 编辑：

```go
configs := cfield.ExtendConfig{
    {Code: "region",   Name: "区域",   Type: cfield.TypeString, Required: true},
    {Code: "priority", Name: "优先级", Type: cfield.TypeInt},
    {Code: "due_date", Name: "交期",   Type: cfield.TypeDate, Formatter: "YYYY-MM-DD"},
    {Code: "urgent",   Name: "加急",   Type: cfield.TypeBool},
    {Code: "material", Name: "材质",   Type: cfield.TypeRadio,
        Options: []cfield.Option{{Label: "钢材", Value: "steel"}, {Label: "合金", Value: "alloy"}}},
    {Code: "tags",     Name: "标签",   Type: cfield.TypeCheckbox,
        Options: []cfield.Option{{Label: "出口", Value: "export"}, {Label: "加急", Value: "rush"}}},
    {Code: "drawing",  Name: "图纸",   Type: cfield.TypeFile,
        FileConfig: &cfield.FileConfig{MaxSize: 10 << 20, Accept: ".pdf,.dwg"}},
}

// 运行时编辑的 Schema，持久化前先校验。
if err := configs.Validate(); err != nil {
    return err
}
```

`ExtendConfig` 自身实现了 `driver.Valuer`/`sql.Scanner`，可以用同样的方式存储。

### 2. 写入字段值

```go
// req.Fields 是前端提交的 JSON 数组 [{"code":"region","value":"EMEA"}, …]。
data, err := cfield.BindToDB(req.Fields, configs)
if err != nil {
    return err // 例如：cfield: required field [区域] is empty
}
order.Ext = data
db.Save(order)
```

`BindToDB` 会丢弃禁用字段、强转类型（`"2"` → `2`、`"是"` → `true`）、把选项
label 归一化为 value，并校验附件约束。

### 3. 读取字段值

```go
vo := cfield.FormatToVO(order.Ext, configs)
// [{"code":"due_date","name":"交期","value":"2025-06-30","required":false}, …]
```

日期按字段配置的格式返回；选项字段附带选项配置；附件归一化为 `[]FileInfo`。

### 4. 按自定义字段筛选

```go
filters := cfield.ExtendData{
    "region":   "EMEA",
    "priority": []any{1, 5},                      // 数字区间
    "due_date": []string{"2025-06-01", "2025-06-30"},
    "tags":     []string{"rush"},                 // 多选包含
}

exprs, err := cfield.BuildQuery(filters, configs, "work_orders.ext")
if err != nil {
    return err
}

var orders []WorkOrder
db.Clauses(clause.And(exprs...)).Find(&orders)
```

生成的 SQL 使用原生 JSONB 操作符，例如：

```sql
(work_orders.ext->>'priority') ~ '^-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?$'
  AND (work_orders.ext->>'priority')::numeric BETWEEN 1 AND 5
```

正则守卫用于排除列中混入的非数字内容，避免 `::numeric` 强转直接报错。

### 5. Excel 导入/导出

```go
// 导入模板：追加表头（字段名 + 校验提示行）。
if err := configs.FilterDisabledAndFile().AppendHeader(f, sheet); err != nil { … }

// 导出：值与表头顺序对齐，输出 label 而非原始 value。
row := cfield.FormatToVO(order.Ext, configs).ToExportSlice(configs)
```

模板提示形如 `必填，单选，请填写以下内容的一项: [钢材/合金]`，与导入约定一致
（是/否 布尔、逗号分隔多选、除原始 value 外也接受 label）。

## 数据库表结构（DDL）

cfield 只需要两个 JSONB 列：一个按实体（通常再按租户）存 Schema，一个按行存值。
完整 DDL 与示例数据：

```sql
-- 1. Schema 表：每个 租户 + 实体类型 一行。configs 列对应 cfield.ExtendConfig。
CREATE TABLE cfield_schema (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT      NOT NULL,
    entity_type VARCHAR(64) NOT NULL,               -- 例如 'work_order'
    configs     JSONB       NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, entity_type)
);

-- 2. 业务表：一个 JSONB 列存所有自定义字段值。ext 列对应 cfield.ExtendData。
CREATE TABLE work_order (
    id       BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(64) NOT NULL,
    ext      JSONB       NOT NULL DEFAULT '{}'
);

-- 3. 可选：为高频筛选字段建表达式索引。
--    ->> 文本的等值/区间查询可以走 btree 索引：
CREATE INDEX idx_work_order_region   ON work_order ((ext->>'region'));
CREATE INDEX idx_work_order_priority ON work_order (((ext->>'priority')::numeric));
```

示例数据：

```sql
INSERT INTO cfield_schema (tenant_id, entity_type, configs) VALUES (
    1, 'work_order',
    '[
      {"code": "region",   "name": "区域",   "type": "string",  "required": true, "sort": 1},
      {"code": "priority", "name": "优先级", "type": "int",     "sort": 2},
      {"code": "due_date", "name": "交期",   "type": "date",    "formatter": "YYYY-MM-DD", "sort": 3},
      {"code": "urgent",   "name": "加急",   "type": "boolean", "sort": 4},
      {"code": "material", "name": "材质",   "type": "radio",   "sort": 5,
       "options": [{"label": "钢材", "value": "steel"}, {"label": "合金", "value": "alloy"}]},
      {"code": "tags",     "name": "标签",   "type": "checkbox", "sort": 6,
       "options": [{"label": "出口", "value": "export"}, {"label": "加急", "value": "rush"}]},
      {"code": "drawing",  "name": "图纸",   "type": "attachment", "sort": 7,
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

对应的 GORM 实体：

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

> **索引说明**：上面的表达式索引只服务于与之一致的查询（`ext->>'region' = …`、
> `(ext->>'priority')::numeric BETWEEN …`）。模糊搜索 `LIKE '%…%'` 无法使用
> btree 索引；普通 `GIN (ext)` 索引只加速 `@>` 包含查询，而 `BuildQuery`
> 并不生成该操作符——请为高频筛选字段建表达式索引。

## 字段类型

| 类型 | 强转为 | 附加能力 |
|---|---|---|
| `string` | `string` | `maxLength`（按字符数）、`textArea`、`batchQuery` |
| `int` | `int` | |
| `float` / `number` | `float64` | `precision` |
| `boolean` | `bool` | 接受 是/对/有/真/1 与 否/错/无/假/0 |
| `date` | `time.Time` | `formatter`：`YYYY-MM-DD HH:mm:ss`、`YYYY-MM-DD`、`YYYY-MM`、`YYYY` |
| `attachment` | `[]FileInfo` | `maxSize`（字节）、`accept`（扩展名或 MIME） |
| `radio` | `string` | 固定选项；输入接受 label，存储 value |
| `checkbox` | `[]string` | 固定选项，不允许重复 |

## 创建后只读

```go
// 更新时传入数据库中当前存储的值。
data, err := cfield.BindToDBWithEditCheck(req.Fields, configs, order.Ext)
```

配置了 `EditCondition: cfield.ReadonlyAfterCreate` 的字段一旦有值即被锁定：
不可修改、不可清空；但仍为空的字段允许后续补填（避免创建时缺值的记录被
"必填校验 + 只读锁定" 死锁）。

## 值比较

```go
cfield.CompareAny(18, float64(18))                                   // true
cfield.CompareWithConfig("2025-06-30", "2025-06-30T00:00:00Z", conf) // true（按日期格式）
order.Ext.Equals(other.Ext)                                          // 深度 map 相等
```

## 安全说明

- 字段编码与列名会被拼进 SQL 片段，必须匹配 `^[A-Za-z_][A-Za-z0-9_]*$`
  （列名可为 `表名.列名`）。`ExtendConfig.Validate` 与 `BuildQuery` 都会强制
  校验——**运行时编辑的 Schema 一定要先 Validate 再落库**。
- 所有值均通过查询参数绑定，不拼接任何用户数据。

## 限制

- 查询构建仅支持 PostgreSQL JSONB。
- Excel 导入提示与 是/否 布尔别名面向中文业务文档；如有需要易于适配。

## 许可证

[MIT](LICENSE)
