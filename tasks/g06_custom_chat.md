file: src/g06_custom_chat.go
model: coder

# 任务 g06：自定义模型加载 + 对话调用

产出 `src/g06_custom_chat.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只用标准库（`bufio` `bytes` `context` `encoding/json` `fmt` `net/http` `os` `path/filepath` `strings` `time`）。
可调用：`UnitDef` `readEnv` `writeEnv` `unitActive` `unitAction` `applyParams` `systemctlOutput` `paramSpecs`。

## 必须有的声明

```go
const modelsDir = "/home/user/models"
const customUnit = "llama-custom"

type CustomReq struct {
	ModelFile string            `json:"model_file"`
	Port      int               `json:"port"`
	Params    map[string]string `json:"params"`
}

type ModelFile struct {
	File    string `json:"file"`
	SizeMiB int64  `json:"size_mib"`
}

func customUnitDef() UnitDef
func listModelFiles() ([]ModelFile, error)
func customLoad(req CustomReq) error
func customStop() error
func customStatus() (bool, int)
func chatOnce(port int, message string, maxTokens int) (string, error)
func streamChat(ctx context.Context, port int, message string, maxTokens int, onDelta func(string) error) error
```

## 行为

- `customUnitDef() UnitDef`：返回
  `Name: "llama-custom"`、`Label: "自定义"`、`Model: ""`、`Alias: "llama-custom"`、`Port: 0`、`Embed: false`、
  `Keys: []string{"MODEL","PORT","ALIAS","NGL","CTX","EXTRA"}`、
  `Defaults: map[string]string{"NGL":"999","CTX":"32768","ALIAS":"llama-custom","EXTRA":""}`
- `listModelFiles() ([]ModelFile, error)`：`filepath.Glob(modelsDir + "/*.gguf")`，
  每个取 `os.Stat` 的字节数换成 MiB（`int64(size/1024/1024)`），`File` 只放**文件名**（不含目录）。
  按 `File` 字典序排序。无匹配返回空切片与 nil。
- `customLoad(req CustomReq) error`：
  1. `req.ModelFile` 不能为空，且不得包含 `/` 或 `..`，否则 `fmt.Errorf("模型文件名非法")`
  2. 拼出 `full := modelsDir + "/" + req.ModelFile`，用 `os.Stat` 确认存在，否则 `fmt.Errorf("模型文件不存在：%s", req.ModelFile)`
  3. 端口：`req.Port < 1024 || req.Port > 65535` → `fmt.Errorf("端口必须在 1024-65535")`；
     再遍历 `unitDefs`，若 `req.Port` 等于某 unit 的 `Port` → `fmt.Errorf("端口 %d 已被 %s 占用", req.Port, u.Name)`
  4. 组装参数 map：`{"MODEL": full, "PORT": strconv.Itoa(req.Port), "ALIAS": "llama-custom", "NGL": "999", "CTX": "32768"}`
     再用 `req.Params` 覆盖同键（`req.Params` 里出现 `Keys` 之外的键 → `fmt.Errorf("不支持的参数：%s", k)`）
  5. `writeEnv(customUnit, params)`，然后 `unitAction(customUnit, "restart")`
- `customStop() error`：`unitAction(customUnit, "stop")`
- `customStatus() (bool, int)`：`unitActive(customUnit)` 与读 `readEnv(customUnit)["PORT"]` 解析出的端口（解析失败为 0）
- `chatOnce(port int, message string, maxTokens int) (string, error)`：
  `POST http://127.0.0.1:<port>/v1/chat/completions`，JSON 体
  `{"model":"local","messages":[{"role":"user","content":message}],"max_tokens":<maxTokens>,"stream":false}`，
  超时 180 秒。返回 `choices[0].message.content`。非 2xx 返回 `fmt.Errorf("模型返回 %d", resp.StatusCode)`。
- `streamChat(ctx, port, message, maxTokens, onDelta) error`：
  同样的 URL，体里 `"stream":true`，`http.NewRequestWithContext(ctx, ...)`。
  用 `bufio.Scanner` 逐行读：只处理以 `"data: "` 开头的行；
  内容为 `[DONE]` 时结束；否则 `json.Unmarshal` 成
  `struct{ Choices []struct{ Delta struct{ Content string `json:"content"` } `json:"delta"` } `json:"choices"` }`，
  取 `Choices[0].Delta.Content`，非空就调 `onDelta(content)`，`onDelta` 返回 error 时立刻返回该 error。
  非 2xx 返回 `fmt.Errorf("模型返回 %d", resp.StatusCode)`。`scanner.Err()` 要检查并返回。

## 硬要求

- 不要 import 未使用的包（`bytes` 只在真的用到时才 import）
- 不要 panic、不要 `os.Exit`；不要定义 `main()`
- `gofmt` 干净、`go vet` 无输出
