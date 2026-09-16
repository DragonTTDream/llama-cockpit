file: src/g04_openclaw.go
model: coder

# 任务 g04：跨机控制网关机上的 OpenClaw

产出 `src/g04_openclaw.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只用标准库（`fmt` `os/exec` `strings` `time` `context`）。
本文件是 GPU 机面板与网关机之间的**唯一**通道：只调用两个既有脚本，绝不内联拼业务命令。

## 必须有的常量与声明

```go
const loongHost = "192.0.2.20"
const loongUser = "user"
const loongSSHKey = "/root/.ssh/id_ed25519_panel"

type OpenClawView struct {
	Mode      string `json:"mode"`
	Primary   string `json:"primary"`
	Gateway   string `json:"gateway"`
	Reachable bool   `json:"reachable"`
}

func sshLoong(script string, timeout time.Duration) (string, error)
func openclawInfo() OpenClawView
func openclawSetMode(mode string) error
func openclawGateway(action string) error
```

## 行为

- `sshLoong(script string, timeout time.Duration) (string, error)`：
  用 `context.WithTimeout` + `exec.CommandContext` 执行
  `ssh -o BatchMode=yes -o IdentitiesOnly=yes -o ConnectTimeout=5 -i <loongSSHKey> user@<loongHost> <script>`。
  返回 `CombinedOutput` 的字符串（已 `strings.TrimSpace`）与 err。
  调用方给 `loongSSHKey` 里的路径，**不要**做路径存在性检查。
- `openclawInfo() OpenClawView`：调用 `sshLoong` 执行下面这段脚本（超时 8 秒），
  脚本运行在网关机上，输出**两行**：第一行是主模型、第二行是网关状态。
  ```
  jq -r '.agents.defaults.model.primary // "unknown"' ~/.openclaw/openclaw.json
  XDG_RUNTIME_DIR=/run/user/1000 systemctl --user is-active openclaw-gateway 2>/dev/null || echo inactive
  ```
  解析：按 `\n` 切分，第 1 段 → `Primary`，第 2 段 → `Gateway`。
  `Mode`：`Primary` 含子串 `"llamacpp-"` → `"local"`，否则 `"cloud"`。
  `Reachable`：命令 `err == nil` 且至少解析出 1 段；否则 `false`。
  **任何失败都不能 panic**：失败时返回 `OpenClawView{Mode: "cloud", Primary: "unknown", Gateway: "unknown", Reachable: false}`。
- `openclawSetMode(mode string) error`：
  `mode` 只允许 `"cloud"` / `"local"`，否则返回 `fmt.Errorf("不支持的模式：%s", mode)`。
  执行 `sshLoong("/home/user/bin/openclaw-mode.sh "+mode, 150*time.Second)`，
  失败返回 `fmt.Errorf("切换模式失败：%v（%s）", err, out)`。
- `openclawGateway(action string) error`：
  `action` 只允许 `"restart"` / `"force"`，否则返回 `fmt.Errorf("不支持的操作：%s", action)`。
  执行 `sshLoong("/home/user/bin/openclaw-gateway-ctl.sh "+action, 120*time.Second)`，
  失败返回 `fmt.Errorf("网关操作失败：%v（%s）", err, out)`。

## 硬要求

- `time` 与 `context` 都要用到；不要 import 未使用的包
- 不要 `sh -c`、不要 panic、不要 `os.Exit`
- 不要定义 `main()`；`gofmt` 干净、`go vet` 无输出
