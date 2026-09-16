file: src/g01_types.go
model: coder

# 任务 g01b：参数表改为从 schema.json 加载（**覆盖式重写** src/g01_types.go）

把原来硬编码的参数表改成从 `schema.json` 读取。**对外声明的名字和签名必须保持一致**，
因为 g02~g07 都在调用它们。产出**唯一一个文件** `src/g01_types.go`，`package main`。

只允许 import：`embed` `encoding/json` `fmt` `os` `sort` `strconv` `strings`。

## schema.json 的结构（已存在于 src/schema.json，用 go:embed 读）

```json
{
  "groups": ["基础","显存","性能","采样","其他"],
  "order": ["MODEL","PORT","ALIAS","NGL","NCPUMOE","CTX","CTKV","CTVV","LOADMODE","MLOCK","KVOFFLOAD","OPOFFLOAD","SPLITMODE","MAINGPU","TSPLIT","THREADS","TBATCH","BATCH","UBATCH","FLASH","PARALLEL","CONTBATCH","JINJA","WARMUP","METRICS","TEMP","TOPK","TOPP","MINP","REPEAT","SEED","NPREDICT","EXTRA"],
  "kinds": { "moe": ["all","chat","moe"], "dense": ["all","chat"], "custom": ["all","chat","moe"], "embed": ["all"] },
  "params": { "MODEL": { "label":"模型文件", "group":"基础", "type":"model", "core":true, "scope":"all", "default":"", "safe":"", "options":[], "note":"..." }, ... },
  "units": [ { "name":"llama-general", "label":"通用对话", "kind":"moe", "embed":false, "model":"...gguf", "alias":"llama-general", "port":1231, "defaults": {"NGL":"999", ...} }, ... ]
}
```

注意：`params` 里的键就是参数名，但 JSON 对象**不保证顺序**——所以必须用顶层 `order` 数组来决定顺序；
`units` 里的 `defaults` 是该 unit 覆盖全局默认值的部分（可能没有某个键）。

## 必须有的声明

```go
type ParamSpec struct {
	Key     string
	Label   string
	Group   string
	Type    string   // int | float | enum | bool | str | model
	Core    bool     // true = 写成独立 env 变量；false = 拼进 OPT
	Scope   string   // all | chat | moe
	Flag    string   // CLI 旗标；Core 为 true 时为空
	OffFlag string   // bool 类型的关闭旗标，可能为空
	Min     float64
	Max     float64
	Default string
	Safe    string
	Note    string
	Options []string
}

type UnitDef struct {
	Name     string
	Label    string
	Kind     string // moe | dense | embed | custom
	Embed    bool
	Model    string
	Alias    string
	Port     int
	Keys     []string          // 该 unit 允许改的参数，按 order 顺序过滤
	Defaults map[string]string // 该 unit 的默认 env 值
}

var paramSpecs map[string]ParamSpec
var unitDefs []UnitDef

func unitDefByName(name string) *UnitDef
func defaultParams(u *UnitDef) map[string]string
func validateParams(u *UnitDef, p map[string]string) error
func envFileName(u *UnitDef) string
func paramSpecByKey(k string) (ParamSpec, bool)
func groupsOrder() []string
func paramOrder() []string
func appliesTo(kind, scope string) bool
func keysForKind(kind string) []string
func defaultOptFor(kind string) string
```

## 加载逻辑（写在 `func init()`）

1. `//go:embed schema.json` 到 `var schemaJSON []byte`（指令注释必须**紧贴** `var` 上一行）
2. 定义一个内部结构体反序列化，字段名对齐 JSON：`Groups []string` / `Order []string` /
   `Kinds map[string][]string` / `Params map[string]ParamSpec` / `Units []struct{...}`
   （Params 用 `map[string]ParamSpec` 接，`ParamSpec` 的 json tag 用上面结构里的小写名；
   `Key` `Core` 需要 `json:"-"` 的用 `json:"core"`、`json:"scope"` 等，`Key` 用 `json:"-"` 然后人肉填）
3. 反序列化失败 → `panic("schema.json 解析失败：" + err.Error())`
4. 把 `unitDefs`、`paramSpecs`、`groups`、`order`、`kinds` 存到包级变量
5. 遍历 `unitDefs`：
   - `Keys = keysForKind(u.Kind)`
   - `Defaults`：先放 `MODEL`（若 `u.Model != ""` 则 `/home/user/models/` + `u.Model`）、
     `PORT`（`strconv.Itoa(u.Port)`）、`ALIAS`（`u.Alias`）；
     再对 `Keys` 里每个 `Core == true` 的参数：若该 unit 的 `defaults` 里给了值就用它，
     否则用该参数的全局 `Default`（为空则不放这个键）；
     最后放 `OPT = defaultOptFor(u.Kind)` 与 `EXTRA = ""`（若 `EXTRA` 在 `Keys` 里）
   - `Keys` 里要**包含** `OPT` 吗？**不要**。`OPT` 不是可编辑参数，它是拼装结果，由 g03 单独处理。
     但 `Defaults` 里**要**有 `OPT` 键（除非 `defaultOptFor` 返回空串）。

## 各函数行为

- `unitDefByName(name)`：按 `Name` 精确查找，找不到返回 `nil`（用 `for i := range unitDefs` 取地址）
- `defaultParams(u)`：返回 `u.Defaults` 的**深拷贝**；`u == nil` 返回空 map
- `envFileName(u)`：`"llama-general"` → `"general"`（去掉 `llama-` 前缀）；`nil` 返回 `""`
- `paramSpecByKey(k)`：查 `paramSpecs`
- `groupsOrder()`：返回 `groups` 的副本
- `paramOrder()`：返回 `order` 的副本（注意：只返回 `paramSpecs` 里真实存在的键，顺序照 `order`；
  `order` 里有但 `paramSpecs` 里没有的键要跳过）
- `appliesTo(kind, scope)`：从 `kinds[kind]` 里找 `scope`；`kind` 未知返回 `scope == "all"`
- `keysForKind(kind)`：按 `paramOrder()` 顺序，取 `appliesTo(kind, spec.Scope)` 为真的键；
  **要额外把 `"EXTRA"` 放在最后一个**（如果它还没被包含）
- `defaultOptFor(kind)`：`kind == "embed"` 返回 `""`；其它返回 `"--load-mode none --jinja"`

- `validateParams(u, p)`：
  1. `u == nil` → `fmt.Errorf("未知的模型单元")`
  2. `p` 里的键不在 `u.Keys` → `fmt.Errorf("不支持的参数：%s", k)`
  3. `u.Keys` 里的键在 `p` 里缺失 → `fmt.Errorf("缺少参数：%s", k)`
     （例外：键不在 `paramSpecs` 里时跳过检查，例如 `OPT`）
  4. `Type == "int"`：`strconv.Atoi` 失败 → `fmt.Errorf("%s 必须是整数", spec.Label)`；
     `float64(v) < spec.Min || float64(v) > spec.Max` → `fmt.Errorf("%s 超出范围（%g-%g）", spec.Label, spec.Min, spec.Max)`
  5. `Type == "float"`：`strconv.ParseFloat` 失败 → `fmt.Errorf("%s 必须是数字", spec.Label)`；同样查范围
  6. `Type == "enum" || Type == "bool"`：值非空且不在 `spec.Options` 里 →
     `fmt.Errorf("%s 取值非法：%s", spec.Label, v)`
     **注意**：值为空串表示「不指定」，是合法的，要放行
  7. `p["PORT"]` 非空且与**其它** unit 的 `Port` 相同 → `fmt.Errorf("端口 %s 已被 %s 占用", v, 那个 unit 的 Name)`
  8. `p["MODEL"]` 非空且不以 `/home/user/models/` 开头 → `fmt.Errorf("模型路径必须在 /home/user/models/ 下")`
  9. 全部通过 → `nil`

## 硬要求

- **不要**定义 `main()`；不要定义上面未列出的顶层函数
- 不要 import 未使用的包（`sort` 只在真用到时才留）
- `gofmt` 必须干净、`go vet` 必须无输出、`go build` 必须成功
- 反序列化用的内部结构体类型名自定，不要与其它文件冲突
