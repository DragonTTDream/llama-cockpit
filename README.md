# llama-cockpit — llama.cpp 控制台

一个单二进制 Go Web 面板，用来管一台 **GPU 机**上的 llama.cpp 推理服务：
看谁在跑、占多少显存、改启动参数、切换 OpenClaw 的云端/本地模式、以及自定义加载 GGUF 并直接对话。

无前端框架、无 CDN、无外部资源；整个面板是一个 **10.2 MB 静态二进制**（`CGO_ENABLED=0`，前端已 `go:embed` 进去）。

## 涉及两台机器

| 角色 | 它是什么 | 面板装在哪 | 本文中的地址 |
|---|---|---|---|
| **GPU 机** | 跑 llama.cpp 推理服务（Fedora 43，单张 16 GiB 显卡），模型以 systemd unit 形式管理 | ✅ 装在这里 | `192.0.2.10` |
| **网关机** | 跑 OpenClaw（Ubuntu），提供模型路由与频道 | ❌ | `192.0.2.20` |

> 上面两个 IP 属于 RFC 5737 文档专用地址段，**是占位符**，请替换成你自己的。用户名统一写作 `you`。

## 快速开始

```bash
# 在 GPU 机上
sudo install -m 0755 llama-panel /opt/llama-panel/llama-panel
sudo systemctl enable --now llama-panel
```

- **面板地址**：<http://192.0.2.10:8077>
- **访问令牌**：`/opt/llama-panel/token`（64 位十六进制，首次启动自动生成；页面上输一次，写 Cookie 30 天）
  在 GPU 机上读取：`sudo cat /opt/llama-panel/token`

## 功能

| # | 功能 | 说明 |
|---|---|---|
| 1 | 当前运行的模型 | 顶栏「当前模型」；每个模型卡片带状态灯 |
| 2 | 资源占用（百分比条） | 每张卡片一条显存条（按 PID 归属到 systemd unit）+ GPU 利用率/温度 |
| 3 | 模型参数可调 | 卡片内参数输入框（含实测安全范围，越界飘红）→「保存并重启」 |
| 4 | 云端 / 全本地切换 | OpenClaw 区两个大按钮，改网关机上 4 处配置（不重启网关） |
| 5 | 网关重启 / 强制重启 | 两个按钮，二次确认；**会断开当前网页会话** |
| 6 | 面板开机自启 | 顶栏开关，等价 `systemctl enable/disable llama-panel` |
| 7 | 自定义加载 + 对话 | 选 GGUF + 端口 + 参数 → 加载 → 底部对话（SSE 流式） |
| 8 | 无归属进程告警 | 仪表盘列出**不属于任何 unit** 的 GPU 进程，避免"显存被谁吃了"查不到 |

## 架构

```
GPU 机
├── /opt/llama-panel/llama-panel            单静态二进制（前端已 embed）
├── /opt/llama-panel/token                  访问令牌
├── /etc/systemd/system/llama-panel.service
├── /etc/systemd/system/llama-*.service     各模型 unit（全部 EnvironmentFile 驱动）
└── /etc/llama.d/<name>.env                 每个模型的参数（面板就改这里）

网关机（面板通过 SSH 调用这几个脚本，不内联拼业务命令）
├── ~/bin/openclaw-mode.sh         cloud | local | show
├── ~/bin/openclaw-gateway-ctl.sh  restart | force | status
└── ~/bin/openclaw-primary.sh      打印当前主模型
```

跨机通道：GPU 机的 root 用一把专用密钥免密登录网关机。

### 参数类 unit 的结构

```ini
Environment="MODEL=..." "PORT=1231" ... "EXTRA="      # 默认值 = 改造前的原始参数
EnvironmentFile=-/etc/llama.d/general.env             # 面板改这里，覆盖默认值
ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model "${MODEL}" ... $$EXTRA'
```

- `Environment=` 保留原参数当默认值：`.env` 丢了服务照样能按原样启动
- `$$EXTRA` 必须交给 shell：**实测**值为空时该参数被丢掉、有值时正确拆词
  （systemd 会把空的 `${EXTRA}` 变成一个**空参数**，这是个坑）
- 聊天 unit 之间双向 `Conflicts=`，同一时刻只跑一个

## 源码与生成流水线

```
projects/llama-panel/
├── CONTRACT.md              接口契约（模型表、参数安全范围、全部 API、前端分区）
├── tasks/g*.md              每个文件的生成规格
├── src/                     Go 源码 + src/web/（index.html / style.css / app.js）
└── harness/
    ├── gen.py               生成→验收主控；代码不进监工上下文，只回一行结论
    ├── gate.sh              gofmt + go vet + go build
    ├── gate-web.sh          前端：无外部资源 + 必需 id 齐全 + js 引用的 id 存在
    ├── deploy.sh            构建 → 装二进制 → 起服务 → 冒烟
    └── verify-*.py          改参数 / 自定义加载的端到端验证
```

重新部署：

```bash
bash harness/deploy.sh      # 构建 + 安装 + 重启 + 冒烟
```

## 排错

| 现象 | 原因 / 处理 |
|---|---|
| 页面一直要令牌 | 令牌在 `/opt/llama-panel/token`；清了浏览器 Cookie 也会这样 |
| OpenClaw 区显示 `unknown` | 跨机通道断了：查 GPU 机上那把面板专用密钥与网关机的 `authorized_keys` |
| 改参数失败 | 看页面红色提示；越界、端口冲突会被后端拒（400） |
| 某模型起不来 | `journalctl -u llama-<name> -n 50`；`.env` 写坏了删掉即可，默认值兜底 |

## 已知限制

- 自定义加载的模型**不出现在**卡片列表里（它不在 unit 表内），只有状态文案
- 面板只跑 HTTP，靠令牌鉴权；**不要把它暴露到公网**
- `harness/` 下的脚本是作者的个人工具链，**按下方占位符替换后**才可直接运行

## 占位符说明

本仓库从作者自己的部署导出，**主机名 / IP / 密钥路径均已替换**：

- `192.0.2.10`、`192.0.2.20` —— RFC 5737 文档段，换成你自己的 GPU 机与网关机地址
- `/home/user/...` —— 换成你自己的家目录
- `you` / `user@` —— 换成你自己的登录用户名
