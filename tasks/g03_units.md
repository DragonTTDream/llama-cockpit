file: src/g03_units.go
model: coder

# 任务 g03：systemd 单元控制与环境文件读写

产出 `src/g03_units.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只用标准库（`os` `os/exec` `sort` `strconv` `strings` `fmt`）。
可调用 g01 已定义的 `UnitDef` `envFileName` `validateParams`。

## 必须有的声明

```go
func systemctlOutput(args ...string) (string, error)
func unitActive(unit string) bool
func unitStateText(unit string) string
func unitEnabled(unit string) bool
func unitAction(unit, action string) error
func envPath(unit string) string
func readEnv(unit string) (map[string]string, error)
func writeEnv(unit string, params map[string]string) error
func applyParams(u *UnitDef, params map[string]string) error
```

## 行为

- `systemctlOutput(args ...string) (string, error)`：`exec.Command("systemctl", args...)`，
  返回 `CombinedOutput` 的字符串（已 `TrimSpace`）与 `err`。
- `unitActive(unit string) bool`：`systemctl is-active <unit>` 输出等于 `"active"`。
- `unitStateText(unit string) string`：把 `systemctl is-active` 的输出翻成中文：
  `active→运行中`、`inactive→已停止`、`activating→启动中`、`deactivating→停止中`、
  `failed→失败`、其它情况原样返回（空字符串时返回 `"未知"`）。
- `unitEnabled(unit string) bool`：`systemctl is-enabled <unit>` 输出等于 `"enabled"`。
- `unitAction(unit, action string) error`：
  `action` 只允许 `start` / `stop` / `restart`，其它值返回
  `fmt.Errorf("不支持的操作：%s", action)`。
  执行 `systemctl <action> <unit>`，`err != nil` 时返回
  `fmt.Errorf("systemctl %s %s 失败：%v（%s）", action, unit, err, out)`。
- `envPath(unit string) string`：去掉 `llama-` 前缀后拼成
  `"/etc/llama.d/" + 前缀去掉后的名字 + ".env"`。
  例：`envPath("llama-general") == "/etc/llama.d/general.env"`；
  `envPath("llama-embed") == "/etc/llama.d/embed.env"`。
- `readEnv(unit string) (map[string]string, error)`：
  读 `envPath(unit)`。**文件不存在时返回空 map（非 nil）且 error 为 nil**（视为用默认值）。
  逐行解析：跳过空行与以 `#` 开头的行；按**第一个** `=` 切分；键与值都 `TrimSpace`；
  无 `=` 的行跳过。其它读取错误照常返回。
- `writeEnv(unit string, params map[string]string) error`：
  1. `os.MkdirAll("/etc/llama.d", 0o755)`
  2. 键按字典序排序，输出 `KEY=VALUE\n`（**不加引号、不加空格**）
  3. **原子写**：写到 `envPath(unit)+".tmp"`，`os.Chmod` 到 `0o644`，再 `os.Rename` 覆盖正式文件
  4. 任一步失败返回带中文说明的 error
- `applyParams(u *UnitDef, params map[string]string) error`：
  `validateParams(u, params)` → 失败直接返回；成功则 `writeEnv(u.Name, params)` →
  `unitAction(u.Name, "restart")`。任一步失败返回该步的 error。
  `u == nil` 返回 `fmt.Errorf("未知的模型单元")`。

## 硬要求

- 不要 `sh -c`，不要 panic，不要 `os.Exit`
- 不要定义 `main()`，不要重复定义 g01/g02 已有的函数
- `gofmt` 必须干净，`go vet` 必须无输出
