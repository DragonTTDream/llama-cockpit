file: src/g07_http.go
model: coder

# 任务 g07：HTTP 层（鉴权 + 全部 API 处理器 + 路由）

产出 `src/g07_http.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只用标准库（`crypto/rand` `encoding/hex` `encoding/json` `fmt` `net/http` `os` `strings` `time` `context`）。
可调用前面已定义的一切（`buildState` `paramSpecs` `unitDefs` `unitDefByName` `defaultParams`
`unitAction` `applyParams` `systemctlOutput` `listModelFiles` `customLoad` `customStop`
`openclawSetMode` `openclawGateway` `streamChat` `CustomReq` `panelPort`）。

## 必须有的声明

```go
const tokenPath = "/opt/llama-panel/token"
const cookieName = "panel_token"

type ChatReq struct {
	Port      int    `json:"port"`
	Message   string `json:"message"`
	MaxTokens int    `json:"max_tokens"`
}

func loadToken() (string, error)
func writeJSON(w http.ResponseWriter, code int, v any)
func fail(w http.ResponseWriter, code int, msg string)
func decodeJSON(r *http.Request, v any) error
func authed(r *http.Request, tok string) bool
func requireAuth(tok string, next http.HandlerFunc) http.HandlerFunc
func handleLogin(tok string) http.HandlerFunc
func handleState(w http.ResponseWriter, r *http.Request)
func handleSchema(w http.ResponseWriter, r *http.Request)
func handleModels(w http.ResponseWriter, r *http.Request)
func handleUnitAction(w http.ResponseWriter, r *http.Request)
func handleUnitParams(w http.ResponseWriter, r *http.Request)
func handleUnitDefaults(w http.ResponseWriter, r *http.Request)
func handlePanelAutostart(w http.ResponseWriter, r *http.Request)
func handleOpenclawMode(w http.ResponseWriter, r *http.Request)
func handleOpenclawGateway(w http.ResponseWriter, r *http.Request)
func handleCustomLoad(w http.ResponseWriter, r *http.Request)
func handleCustomStop(w http.ResponseWriter, r *http.Request)
func handleChat(w http.ResponseWriter, r *http.Request)
func registerAPI(mux *http.ServeMux, tok string)
```

## 行为

- `loadToken() (string, error)`：读 `tokenPath`，`strings.TrimSpace` 后返回。
  文件不存在时：`os.MkdirAll("/opt/llama-panel", 0o755)`，
  用 `crypto/rand` 取 32 字节 → `hex.EncodeToString` → `os.WriteFile(tokenPath, []byte(tok+"\n"), 0o600)` → 返回新令牌。
  其它读取错误原样返回。
- `writeJSON(w, code, v)`：先 `w.Header().Set("Content-Type", "application/json; charset=utf-8")`，
  再 `w.WriteHeader(code)`，最后 `json.NewEncoder(w).Encode(v)`。
- `fail(w, code, msg)`：`writeJSON(w, code, map[string]any{"ok": false, "error": msg})`。
- `decodeJSON(r, v) error`：`json.NewDecoder(r.Body).Decode(v)`，出错返回 `fmt.Errorf("请求体格式错误")`。
- `authed(r, tok) bool`：请求头 `X-Panel-Token` 等于 `tok`（非空），**或** Cookie `panel_token` 等于 `tok`。
  两者都不满足返回 false。
- `requireAuth(tok, next)`：返回一个 `http.HandlerFunc`，未通过则 `fail(w, http.StatusUnauthorized, "unauthorized")` 并返回（不调用 next）。
- `handleLogin(tok)`：解 `{"token":"..."}`；解析失败 → `fail(400, "请求体格式错误")`；
  令牌不符 → `fail(401, "令牌错误")`；正确则
  `http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 2592000})`
  并 `writeJSON(w, 200, map[string]any{"ok": true})`。
- `handleState`：`writeJSON(w, 200, buildState())`
- `handleSchema`：`writeJSON(w, 200, map[string]any{"ok": true, "params": paramSpecs})`
- `handleModels`：`listModelFiles()` 出错 → `fail(500, err.Error())`；否则 `{"ok":true,"models":[...]}`
- `handleUnitAction`：解 `{"name":"...","action":"..."}`；`unitDefByName` 为 nil → `fail(404, "未知的模型单元")`；
  `unitAction` 出错 → `fail(500, err.Error())`；否则 `{"ok":true,"msg":"已执行 start"}`
- `handleUnitParams`：解 `{"name":"...","params":{...}}`；单元未知 → 404；
  `applyParams` 出错 → `fail(400, err.Error())`（校验失败是用户问题，用 400）；否则 `{"ok":true,"msg":"已保存并重启该模型"}`
- `handleUnitDefaults`：取查询参数 `name`；未知 → 404；否则 `{"ok":true,"params":defaultParams(u)}`
- `handlePanelAutostart`：解 `{"enabled":true|false}`；
  `enabled` 为真 → `systemctlOutput("enable", "llama-panel.service")`，
  否则 `systemctlOutput("disable", "llama-panel.service")`；出错 → `fail(500, ...)`；
  否则 `{"ok":true,"msg":"已开启开机自启"}` / `"已关闭开机自启"`
- `handleOpenclawMode`：解 `{"mode":"cloud|local"}`；`openclawSetMode` 出错 → `fail(500, err.Error())`；
  否则 `{"ok":true,"mode":<mode>,"msg":"已切换到云端模式"}` 或 `"已切换到本地模式"`
- `handleOpenclawGateway`：解 `{"action":"restart|force"}`；`openclawGateway` 出错 → `fail(500, ...)`；
  否则 `{"ok":true,"msg":"已重启网关"}`
- `handleCustomLoad`：解 `CustomReq`；`customLoad` 出错 → `fail(400, err.Error())`；否则 `{"ok":true,"msg":"已加载自定义模型"}`
- `handleCustomStop`：`customStop()` 出错 → `fail(500, ...)`；否则 `{"ok":true,"msg":"已停止自定义模型"}`
- `handleChat`：解 `ChatReq`；`Message` 为空 → `fail(400, "消息不能为空")`；
  `MaxTokens <= 0` 时取默认 **512**。
  设置响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`、`X-Accel-Buffering: no`；
  `w.WriteHeader(200)`。取 `f, _ := w.(http.Flusher)`（可能为 nil，nil 时跳过 flush）。
  用 `r.Context()` 调 `streamChat`，`onDelta` 里
  `json.Marshal(map[string]string{"delta": s})` 后写 `"data: " + string(b) + "\n\n"` 并 flush。
  结束后写 `"data: {\"done\":true}\n\n"` 并 flush。
  `streamChat` 返回 error 时，写 `"data: {\\\"error\\\":\\\"...\\\"}\n\n"`（用 `json.Marshal(map[string]string{"error": err.Error()})` 生成，**不要手拼 JSON**）。
  注意：此时响应头已发出，**不要**再调用 `fail`。
- `registerAPI(mux, tok)`：注册以下路由（`/api/login` 不加鉴权，其余全部用 `requireAuth(tok, ...)` 包起来）：
  ```
  POST /api/login            -> handleLogin(tok)
  GET  /api/state            -> handleState
  GET  /api/schema           -> handleSchema
  GET  /api/models           -> handleModels
  POST /api/unit/action      -> handleUnitAction
  POST /api/unit/params      -> handleUnitParams
  GET  /api/unit/defaults    -> handleUnitDefaults
  POST /api/panel/autostart  -> handlePanelAutostart
  POST /api/openclaw/mode    -> handleOpenclawMode
  POST /api/openclaw/gateway -> handleOpenclawGateway
  POST /api/custom/load      -> handleCustomLoad
  POST /api/custom/stop      -> handleCustomStop
  POST /api/chat             -> handleChat
  ```
  用 `mux.HandleFunc("POST /api/login", ...)` 这种带方法的模式（Go 1.22+ 语法）。

## 硬要求

- 不要 import 未使用的包（`context` 只在真用到时）
- 不要 panic、不要 `os.Exit`、不要定义 `main()`
- 所有错误信息用中文，且必须是**后端真实错误**，不得假装成功
- `gofmt` 干净、`go vet` 无输出
