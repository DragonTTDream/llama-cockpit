file: src/web/index.html
model: general

# 任务 g13：前端骨架 v3（四个页签）—— **覆盖式重写**

产出 `src/web/index.html`，单文件静态骨架。硬性约束：

- **不得引用任何外部资源**（禁止 `http://`、`https://`、`//cdn`、外链字体）
- 用 `<link rel="stylesheet" href="/style.css">` 与 `<script src="/app.js" defer></script>`
- `<html lang="zh-CN">`、`<meta charset="utf-8">`、viewport
- **只写结构与文案**：不写 `<style>`、不写行内 JS、不写 `onclick`
- 全部文案简体中文；不要省略记号（如 `<!-- 更多略 -->`）

## 必须出现的 id（一个都不能少，拼写一致）

**登录层**：`login-layer` `login-token` `login-msg` `btn-login`
**全局**：`error-bar` `auth-msg` `current-model` `mode-badge` `last-update` `autostart-toggle` `autostart-label` `auth-toggle` `auth-label`
**页签**：`tabs` `btn-tab-dash` `btn-tab-models` `btn-tab-load` `btn-tab-oc` `panel-dash` `panel-models` `panel-load` `panel-oc`

**仪表盘（panel-dash，只读）**：
`dash-cpu-bar` `dash-cpu-text` `dash-cpu-cores` `dash-load`
`dash-mem-bar` `dash-mem-text` `dash-swap-text`
`dash-gpu-bar` `dash-gpu-text` `dash-gpu-util` `dash-gpu-temp` `dash-gpu-name`
`dash-units`

**模型（panel-models，可调）**：
`model-select` `model-state` `model-params` `model-msg`
`btn-model-start` `btn-model-stop` `btn-model-restart` `btn-params-save` `btn-params-reset`

**加载与对话（panel-load）**：
`custom-model` `custom-port` `custom-params` `custom-status` `btn-custom-load` `btn-custom-stop`
`chat-port` `chat-input` `chat-out` `btn-chat-send` `btn-chat-clear`

**OpenClaw（panel-oc）**：
`oc-mode` `oc-primary` `oc-gateway` `oc-msg` `btn-cloud` `btn-local` `btn-gw-restart` `btn-gw-force`

## 结构

1. `#login-layer`（遮罩，默认可见）：标题「需要令牌」+ 说明 + `#login-token`（type=password）+ `#btn-login` + `#login-msg`
2. `#error-bar`（`hidden`，空容器）
3. `#auth-msg`（class `dim`，空容器）
4. `<header>`：左边 `<h1>llama 控制台</h1>` + `<span id="current-model">—</span>`；
   右边 `<span id="mode-badge">—</span>`、
   `<label><input id="autostart-toggle" type="checkbox"><span id="autostart-label">开机自启</span></label>`、
   `<label><input id="auth-toggle" type="checkbox"><span id="auth-label">令牌保护</span></label>`、
   `<span id="last-update">—</span>`
5. `<nav id="tabs">`：四个按钮
   `<button id="btn-tab-dash" class="tab active">仪表盘</button>`、
   `<button id="btn-tab-models" class="tab">模型</button>`、
   `<button id="btn-tab-load" class="tab">加载与对话</button>`、
   `<button id="btn-tab-oc" class="tab">OpenClaw</button>`
6. 四个面板容器（**不要**写 `hidden`，由 JS 控制）：
   - `<section id="panel-dash">`：
     * `<h2>CPU</h2>`：`<div class="bar"><div id="dash-cpu-bar" class="bar-fill"></div></div>`
       与 `<div id="dash-cpu-text" class="bar-text">—</div>`；下面 `<div id="dash-cpu-cores" class="cores"></div>`、
       `<div id="dash-load" class="dim">—</div>`
     * `<h2>内存</h2>`：`#dash-mem-bar`（同上结构）+ `#dash-mem-text` + `<div id="dash-swap-text" class="dim">—</div>`
     * `<h2>GPU</h2>`：`<div id="dash-gpu-name" class="dim">—</div>` + `#dash-gpu-bar` + `#dash-gpu-text` +
       `<div class="dim"><span id="dash-gpu-util">—</span> · <span id="dash-gpu-temp">—</span></div>`
     * `<h2>模型状态</h2>` + `<div id="dash-units"></div>`
   - `<section id="panel-models">`：
     * 一行：`<label>模型 <select id="model-select"></select></label>` + `<span id="model-state" class="dim">—</span>`
     * `<div class="unit-actions">` 五个按钮：`btn-model-start`「启动」`btn-model-stop`「停止」
       `btn-model-restart`「重启」`btn-params-save`「保存并重启」`btn-params-reset`「还原默认」
     * `<div id="model-params"></div>`（参数区，由 JS 渲染；里面可写一句「载入中…」作初始文案）
     * `<div id="model-msg" class="dim"></div>`
   - `<section id="panel-load">`：
     * `<h2>自定义加载</h2>`：`<label>模型文件 <select id="custom-model"></select></label>`、
       `<label>端口 <input id="custom-port" type="number" value="1240"></label>`、
       `<div id="custom-params"></div>`、
       `<button id="btn-custom-load" class="btn btn-primary">加载</button>` +
       `<button id="btn-custom-stop" class="btn btn-danger">停止</button>` +
       `<div id="custom-status" class="dim">—</div>`
     * `<h2>对话测试</h2>`：`<label>端口 <select id="chat-port"></select></label>`、
       `<textarea id="chat-input" rows="3" placeholder="输入一句话，回车发送（Shift+回车换行）"></textarea>`、
       `<button id="btn-chat-send" class="btn btn-primary">发送</button>` +
       `<button id="btn-chat-clear" class="btn">清空</button>`、
       `<pre id="chat-out"></pre>`
   - `<section id="panel-oc">`：
     * `<h2>当前状态</h2>`：`<span id="oc-mode">—</span>` / `<span id="oc-primary">—</span>` / `<span id="oc-gateway">—</span>`
     * `<div class="unit-actions">`：`btn-cloud`「切换到云端」`btn-local`「切换到本地」
       `btn-gw-restart`「重启网关」`btn-gw-force`「强制重启」
     * `<div id="oc-msg" class="dim"></div>`
     * 一行提示：「重启网关会断开当前会话，请先确认。」
7. `<footer>`：一句「llama-panel · 本页面由 GPU 机本地模型生成」

## 要求

- 语义清晰，嵌套不超过 4 层，不用 `table` 布局
- 不写 `hidden` 属性的面板（JS 负责显示/隐藏）
