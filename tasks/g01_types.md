file: src/g01_types.go
model: coder

# 任务 g01：静态数据表与参数校验

产出**唯一一个文件** `src/g01_types.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只依赖标准库（`fmt` `sort` `strconv` `strings`）。

## 必须有的声明（签名一字不差）

```go
type ParamSpec struct {
	Key     string
	Label   string
	Type    string   // "int" | "enum"
	Min     int
	Max     int
	Safe    string
	Note    string
	Options []string // 仅 Type=="enum" 时非空
}

type UnitDef struct {
	Name     string            // systemd unit 名，如 "llama-general"
	Label    string            // 中文短标签
	Model    string            // /home/user/models/ 下的文件名
	Alias    string            // llama-server --alias
	Port     int
	Embed    bool              // true = 嵌入模型，键表不同、不与其它互斥
	Keys     []string          // 该 unit 允许的 .env 键，顺序即前端显示顺序
	Defaults map[string]string // 默认值
}

var paramSpecs map[string]ParamSpec
var unitDefs []UnitDef

func unitDefByName(name string) *UnitDef
func defaultParams(u *UnitDef) map[string]string
func validateParams(u *UnitDef, p map[string]string) error
func envFileName(u *UnitDef) string
```

## paramSpecs（逐字录入）

| Key | Label | Type | Min | Max | Safe | Note | Options |
|---|---|---|---|---|---|---|---|
| NGL | GPU 层数 | int | 0 | 999 | 999 | 999 = 全部卸载到 GPU | — |
| NCPUMOE | CPU 专家层 | int | 4 | 64 | 32 | 98k 上下文下低于 16 会 OOM（实测） | — |
| CTX | 上下文长度 | int | 4096 | 131072 | 98304 | 98304 实测占 12.0 GiB | — |
| CTKV | K 量化 | enum | 0 | 0 | q8_0 | q8_0 / f16 / q4_0；q8_0 实测比 q4_0 更省显存 | f16,q8_0,q4_0 |
| CTVV | V 量化 | enum | 0 | 0 | q8_0 | 同上 | f16,q8_0,q4_0 |
| PORT | 端口 | int | 1024 | 65535 |  | 不得与其他服务冲突 | — |
| MODEL | 模型文件 | enum | 0 | 0 |  | 从 /home/user/models/*.gguf 选 | 留空（运行期填充） |
| ALIAS | 别名 | enum | 0 | 0 |  | 一般不用改 | 留空 |
| EXTRA | 追加参数 | enum | 0 | 0 |  | 追加的原始参数串，可空 | 留空 |

## unitDefs（6 条，逐字录入）

聊天 unit 的 `Keys` 顺序固定为：`MODEL PORT ALIAS NGL NCPUMOE CTX CTKV CTVV EXTRA`
`llama-embed` 的 `Keys` 顺序固定为：`MODEL PORT ALIAS NGL CTX EXTRA`

| Name | Label | Model | Alias | Port | Embed | Defaults |
|---|---|---|---|---|---|---|
| llama-general | 通用对话 | Qwen3-30B-A3B-Instruct-2507-Q4_K_M.gguf | llama-general | 1231 | false | NGL=999 NCPUMOE=32 CTX=98304 CTKV=q8_0 CTVV=q8_0 EXTRA= |
| llama-carnice | Carnice 35B | Carnice-Qwen3.6-MoE-35B-A3B-APEX-I-Mini.gguf | llama-carnice | 1232 | false | NGL=999 NCPUMOE=32 CTX=65536 CTKV=q8_0 CTVV=q8_0 EXTRA= |
| llama-heretic | Heretic 27B | Qwen3.8-27B-Heretic-Ara-iq4_xs-2.0.gguf | llama-heretic | 1233 | false | NGL=48 NCPUMOE= (不存在，不放) CTX=32768 CTKV=q8_0 CTVV=q8_0 EXTRA= |
| llama-coder | 代码 | Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf | llama-coder | 1234 | false | NGL=999 NCPUMOE=32 CTX=98304 CTKV=q8_0 CTVV=q8_0 EXTRA= |
| llama-embed | 嵌入向量 | Qwen3-Embedding-0.6B-Q8_0.gguf | Qwen3-Embedding-0.6B | 1235 | true | NGL=999 CTX=8192 EXTRA= |
| llama-qwen38 | Qwen3.8 27B | qwen3.8-27b-mtp-IQ4_XS-pure.gguf | llama-qwen38 | 1236 | false | NGL=48 CTX=32768 CTKV=q8_0 CTVV=q8_0 EXTRA= |

注意：`llama-heretic` 与 `llama-qwen38` 的 `Defaults` 里**没有** `NCPUMOE`，其 `Keys` 里**也要去掉 NCPUMOE**
（它们是非 MoE 的 dense 模型）。二者 Keys 顺序为：`MODEL PORT ALIAS NGL CTX CTKV CTVV EXTRA`。
每条 unit 的 `Defaults` 里 `MODEL` 固定为 `/home/user/models/<Model>`，`PORT` 为上面端口号的十进制字符串，`ALIAS` 为 Alias。

## 函数行为

- `unitDefByName(name string) *UnitDef`：按 Name 精确查找，找不到返回 `nil`。用 `for i := range unitDefs` 取地址。
- `defaultParams(u *UnitDef) map[string]string`：返回 `u.Defaults` 的**深拷贝**（不得返回内部 map）。`u == nil` 返回空 map。
- `envFileName(u *UnitDef) string`：`"llama-general"` → `"general"`（去掉 `llama-` 前缀）。`u == nil` 返回 `""`。
- `validateParams(u *UnitDef, p map[string]string) error`：
  1. `u == nil` → `fmt.Errorf("未知的模型单元")`
  2. 遍历 `p` 的键，键不在 `u.Keys` 里 → `fmt.Errorf("不支持的参数：%s", k)`
  3. 键在 `u.Keys` 里则**必须**出现在 `p`（缺项报 `fmt.Errorf("缺少参数：%s", k)`）
  4. `Type=="int"`：`strconv.Atoi` 失败 → `fmt.Errorf("%s 必须是整数", spec.Label)`；
     小于 Min 或大于 Max → `fmt.Errorf("%s 超出范围（%d-%d）", spec.Label, spec.Min, spec.Max)`
  5. `Type=="enum"` 且 `Options` 非空：取值不在 Options 里 → `fmt.Errorf("%s 取值非法：%s", spec.Label, v)`
  6. `p["PORT"]` 存在且与**其它** unit 的 Port 相同 → `fmt.Errorf("端口 %s 已被 %s 占用", v, 那个 unit 的 Name)`
  7. `p["MODEL"]` 存在且不以 `/home/user/models/` 开头 → `fmt.Errorf("模型路径必须在 /home/user/models/ 下")`
  8. 全部通过 → `nil`
  另外提供 `func paramSpecByKey(k string) (ParamSpec, bool)`（查 `paramSpecs`，第二个返回值表示是否存在）。

## 硬要求

- 不要 import 未使用的包；`gofmt` 必须干净
- 不要定义 `main()`、不要定义上面未列出的顶层函数
- 不要写 `// ...` 之类的省略注释，每条 unit、每个 spec 都要写全
