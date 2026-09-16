# llama-panel 接口契约 v1（冻结）

> 唯一事实来源。所有生成任务都只依据本文件；实现不得偏离。
> 位置：GPU 机 `192.0.2.10`（Fedora 43）·部署目录 `/opt/llama-panel/` ·源码 `projects/llama-panel/src/`

## 0. 目标与非目标

**做**：单二进制 Go Web 面板，监控 + 控制 `GPU 机` 上的 llama.cpp systemd units，并跨机控制 `网关机` 上的 OpenClaw。
**不做**：不做用户系统、不做 HTTPS、不做数据库、不引入任何第三方 Go 模块（只用标准库）。

## 1. 运行形态

| 项 | 值 |
|---|---|
| 监听 | `0.0.0.0:8077`（本机 `127.0.0.1:8077` 与局域网均可访问） |
| 鉴权 | 令牌。令牌文件 `/opt/llama-panel/token`（`0600`，64 位十六进制，首次启动自动生成） |
| 会话 | 登录成功下发 Cookie `panel_token`（`HttpOnly`、`SameSite=Lax`），有效期 30 天 |
| 二进制 | `/opt/llama-panel/llama-panel`（Go，**静态**，`CGO_ENABLED=0`） |
| 前端 | `web/index.html`，用 `//go:embed web` 打进二进制，**单文件、无构建步骤、无 CDN** |
| 依赖 | Go 标准库 only |

**模块名** `llamapanel`，Go 1.26，`go.mod` 只含 `module llamapanel` + `go 1.26`。

## 2. 受管对象

### 2.1 llama units（静态表，写在代码里）

| unit | 端口 | alias | 模型文件（`/home/user/models/`） | 默认参数 |
|---|---|---|---|---|
| `llama-general` | 1231 | llama-general | `Qwen3-30B-A3B-Instruct-2507-Q4_K_M.gguf` | NGL=999 NCPUMOE=32 CTX=98304 CTKV=q8_0 CTVV=q8_0 |
| `llama-carnice` | 1232 | llama-carnice | `Carnice-Qwen3.6-MoE-35B-A3B-APEX-I-Mini.gguf` | NGL=999 NCPUMOE=32 CTX=65536 CTKV=q8_0 CTVV=q8_0 |
| `llama-heretic` | 1233 | llama-heretic | `Qwen3.8-27B-Heretic-Ara-iq4_xs-2.0.gguf` | NGL=48 CTX=32768 CTKV=q8_0 CTVV=q8_0 |
| `llama-coder` | 1234 | llama-coder | `Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf` | NGL=999 NCPUMOE=32 CTX=98304 CTKV=q8_0 CTVV=q8_0 |
| `llama-embed` | 1235 | Qwen3-Embedding-0.6B | `Qwen3-Embedding-0.6B-Q8_0.gguf` | NGL=999 CTX=8192（`--embeddings`） |
| `llama-qwen38` | 1236 | llama-qwen38 | `qwen3.8-27b-mtp-IQ4_XS-pure.gguf` | NGL=48 CTX=32768 CTKV=q8_0 CTVV=q8_0 |

- 前 5 个聊天 unit 通过 systemd `Conflicts=` **互斥**（同时只跑一个）；`llama-embed` 与它们共存。
- 全部 `disabled`（不开机自启），按需拉起。
- `llama-embed` **不参与** 参数编辑里的 NCPUMOE/CTKV/CTVV。

### 2.2 参数环境文件

```
/etc/llama.d/<unit 去掉 llama- 前缀>.env      # 例：/etc/llama.d/general.env
```
- 权限 `0644`，`KEY=VALUE` 每行一条，**无引号、无空格**。
- 键固定为：`MODEL` `PORT` `ALIAS` `NGL` `NCPUMOE` `CTX` `CTKV` `CTVV` `EXTRA`
  （`llama-embed` 用 `MODEL` `PORT` `ALIAS` `NGL` `CTX` `EXTRA`）
- `EXTRA` 为追加的原始参数串，可空。

### 2.3 参数安全范围（**实测得来，必须用于前端校验**）

| 键 | 标签 | 类型 | 最小 | 最大 | 安全值 | 提示文案 |
|---|---|---|---|---|---|---|
| `NGL` | GPU 层数 | int | 0 | 999 | 999 | `999 = 全部卸载到 GPU` |
| `NCPUMOE` | CPU 专家层 | int | 4 | 64 | 32 | `98k 上下文下低于 16 会 OOM（实测）` |
| `CTX` | 上下文长度 | int | 4096 | 131072 | 98304 | `98304 实测占 12.0 GiB` |
| `CTKV` | K 量化 | enum | — | — | `q8_0` | `q8_0 / f16 / q4_0；q8_0 实测比 q4_0 更省显存` |
| `CTVV` | V 量化 | enum | — | — | `q8_0` | 同上 |
| `PORT` | 端口 | int | 1024 | 65535 | 见 2.1 | `不得与其他服务冲突` |
| `MODEL` | 模型文件 | enum | — | — | — | 从 `/home/user/models/*.gguf` 选 |

## 3. HTTP API

所有 `/api/*`（除 `/api/login`）要求有效 Cookie **或** 请求头 `X-Panel-Token`。失败返回 `401 {"ok":false,"error":"unauthorized"}`。

统一响应外壳：成功 `{"ok":true, ...}`；失败 `{"ok":false,"error":"<人类可读中文>"}` + 合适状态码。

| 方法 | 路径 | 请求体 | 响应 |
|---|---|---|---|
| GET | `/` | — | `index.html`（未登录也返回页面，页面自己弹令牌输入框） |
| POST | `/api/login` | `{"token":"..."}` | `{"ok":true}` + Set-Cookie；错令牌 `401` |
| GET | `/api/state` | — | 见 3.1 |
| GET | `/api/schema` | — | `{"ok":true,"params":<2.3 表>}` |
| GET | `/api/models` | — | `{"ok":true,"models":[{"file":"...gguf","size_mib":17698}]}` |
| POST | `/api/unit/action` | `{"name":"llama-general","action":"start\|stop\|restart"}` | `{"ok":true,"msg":"..."}` |
| POST | `/api/unit/params` | `{"name":"llama-general","params":{"CTX":"65536"}}` | 校验失败 `400`；成功写入 `.env` 并 `systemctl restart` |
| GET | `/api/unit/defaults?name=` | — | `{"ok":true,"params":{...}}`（2.1 默认值，供「还原」按钮） |
| POST | `/api/panel/autostart` | `{"enabled":true}` | `{"ok":true,"msg":"..."}`（`systemctl enable/disable llama-panel.service`） |
| POST | `/api/openclaw/mode` | `{"mode":"cloud\|local"}` | `{"ok":true,"msg":"...","mode":"..."}` |
| POST | `/api/openclaw/gateway` | `{"action":"restart\|force"}` | `{"ok":true,"msg":"..."}` |
| POST | `/api/custom/load` | `{"model_file":"...gguf","port":1240,"params":{...}}` | `{"ok":true,"msg":"..."}` |
| POST | `/api/custom/stop` | — | `{"ok":true}` |
| POST | `/api/chat` | `{"port":1231,"message":"你好","max_tokens":256}` | `text/event-stream`，逐块 `data: {"delta":"..."}`，结束 `data: {"done":true}` |

### 3.1 `/api/state` 响应

```json
{
  "ok": true,
  "time": "2026-09-16T03:30:00+08:00",
  "gpu": {"name":"NVIDIA GeForce RTX 4070 Ti SUPER","mem_used_mib":14093,"mem_total_mib":16376,"util_pct":0,"temp_c":58},
  "openclaw": {"mode":"cloud","primary":"deepseek/deepseek-flash","gateway":"active","reachable":true},
  "panel": {"port":8077,"autostart":true,"uptime_s":123},
  "units": [{
    "name":"llama-general","label":"通用对话","port":1231,"model":"Qwen3-...gguf",
    "active":true,"state":"active","state_label":"运行中","enabled":false,
    "vram_mib":14093,"vram_pct":86,"gpu":"llama-server",
    "health":"ok","params":{"NGL":"999","CTX":"98304"},
    "editable":true,"coexist":false,"default_active":true
  }]
}
```

- `vram_pct = vram_mib / mem_total_mib * 100`，保留 1 位小数；归属靠 `nvidia-smi --query-compute-apps` 的 **PID → systemd unit** 映射（`systemctl show -p MainPID`）。
- `health`：对该端口 `GET /health`，2xx → `"ok"`，超时 `"timeout"`，未监听 `"down"`。
- `state_label` 中文：`运行中` / `已停止` / `启动中` / `失败`。
- `openclaw.mode`：读 `user` 上 `~/.openclaw/openclaw.json` 的 `agents.defaults.model.primary`——含 `llamacpp-` 判 `local`，否则 `cloud`。

## 4. 跨机动作（`GPU 机` → `user`）

一律走 SSH（密钥 `~/.ssh/id_ed25519_panel`，用户 `you@192.0.2.20`），只调用两个脚本，**不内联拼命令**：

```
~/bin/openclaw-mode.sh cloud|local      # 改 4 处配置 + 重启网关
~/bin/openclaw-gateway-ctl.sh restart|force
```

- `force` = `pkill -9` 网关进程后再 `systemctl --user start`。
- 走 `systemctl --user` 必须带 `XDG_RUNTIME_DIR=/run/user/1000`。
- 脚本不存在 / SSH 失败 → 面板返回 `{"ok":false,"error":"..."}`，**不得**假装成功。

## 5. 前端（`web/index.html`）

单文件，内嵌 CSS/JS，**无任何外部资源**。暗色。每 2 秒 `fetch('/api/state')` 一次（页面隐藏时暂停）。分区：

1. **顶栏**：GPU 显存条（已用/总量 + 百分比）· GPU 利用率 · 温度 · 当前运行模型名 · OpenClaw 模式徽章（云/本地，可点切换）· 面板自启开关
2. **模型卡片**（6 张，每张一个 unit）：状态灯 · 名称/端口/模型文件 · **显存百分比条** · 参数表（可编辑输入框，逐项显示安全范围与提示，超范围飘红）· 按钮：启动 / 停止 / 重启 / 还原默认 · 「保存并重启」
3. **OpenClaw 区**：当前模式与四处配置预览 · 「切换到云端」/「切换到本地」大按钮 · 「重启网关」/「强制重启」（**二次确认**，并提示会断开当前会话）
4. **自定义加载**：GGUF 下拉（来自 `/api/models`）· 端口 · 参数 · 加载 / 停止
5. **对话测试**：端口选择 · 输入框 · 流式输出区 · 发送
6. **错误条**：任何 API 失败在顶部显示一条可关闭的红色提示（中文）

交互要求：所有写操作按钮点击后**禁用并显示进行中**，完成后刷新状态；失败必须显示后端 `error` 原文。

## 6. 机器验收门（每步都要过）

1. `gofmt -l .` 输出为空
2. `go vet ./...` 无输出
3. `CGO_ENABLED=0 go build -o llama-panel .` 成功，产物 `file` 显示 `statically linked`
4. `curl -s 127.0.0.1:8077/api/state | jq -e '.ok==true'` 为真
5. 每个 API 至少一条 `curl` 冒烟（带令牌，断言 HTTP 码与 JSON 字段）
6. 前端：`grep -c "http://\|https://" web/index.html` **必须为 0**（无外部资源）

## 7. 禁止事项

- 不引入第三方模块、不使用 CDN、不写 `go.sum` 依赖
- 不整文件重写别人的文件（每个文件只由一个任务生成，边界固定）
- 不删除 `/etc/systemd/system/llama-*.service` 原件（备份在 `llama-backup-2026-09-16/`）
- 未实测通过的模型不得用于生成（本任务派 `llama-coder` 写 Go，`llama-general` 写 HTML/文案）
