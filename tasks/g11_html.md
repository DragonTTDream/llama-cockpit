file: src/web/index.html
model: general

# 任务 g11：前端骨架 index.html

产出 `src/web/index.html`，**单文件静态骨架**。
硬性约束：
- **不得引用任何外部资源**（禁止 `http://`、`https://`、`//cdn`、任何 CDN 或外链字体）
- 用 `<link rel="stylesheet" href="/style.css">` 和 `<script src="/app.js" defer></script>` 引同源文件
- `<html lang="zh-CN">`、`<meta charset="utf-8">`、`<meta name="viewport" content="width=device-width,initial-scale=1">`
- **只写结构与文案，不写任何 `<style>` 块、不写任何行内 JS**（样式在 style.css，逻辑在 app.js）
- 全部文案用简体中文

## 必须出现的 id（一个都不能少，拼写一致；app.js 依赖它们）

顶栏：`error-bar` `gpu-name` `gpu-mem-bar` `gpu-mem-text` `gpu-util` `gpu-temp`
　　　`current-model` `mode-badge` `last-update` `autostart-toggle` `autostart-label`
模型区：`units`
OpenClaw 区：`oc-mode` `oc-primary` `oc-gateway` `btn-cloud` `btn-local` `btn-gw-restart` `btn-gw-force` `oc-msg`
自定义区：`custom-model` `custom-port` `custom-ngl` `custom-ctx` `custom-status` `btn-custom-load` `btn-custom-stop`
对话区：`chat-port` `chat-input` `btn-chat-send` `btn-chat-clear` `chat-out`
登录层：`login-layer` `login-token` `btn-login`

## 结构（按顺序）

1. `<div id="login-layer">`：遮罩层。标题「需要令牌」+ 说明一句 + `<input id="login-token" type="password">` + `<button id="btn-login">进入</button>`
2. `<div id="error-bar" hidden>`：顶部错误条（空容器，内容由 JS 填）
3. `<header>`：
   - 左侧：`<h1>llama 控制台</h1>` + `<span id="current-model">—</span>`
   - 右侧：`<span id="mode-badge">—</span>`、`<label><input id="autostart-toggle" type="checkbox"> <span id="autostart-label">开机自启</span></label>`、`<span id="last-update">—</span>`
4. `<section id="gpu">`：`<h2>GPU</h2>` + 一行 `<span id="gpu-name">—</span>`
   - 显存：一行含 `<div class="bar"><div id="gpu-mem-bar" class="bar-fill"></div></div>` 与 `<span id="gpu-mem-text">—</span>`
   - `<span id="gpu-util">—</span>`、`<span id="gpu-temp">—</span>`
5. `<section>`：`<h2>模型单元</h2>` + `<div id="units"></div>`（卡片由 JS 渲染；可在里面放一句「载入中…」作为初始文案）
6. `<section>`：`<h2>OpenClaw</h2>`
   - 一行三个只读项：`<span id="oc-mode">—</span>`、`<span id="oc-primary">—</span>`、`<span id="oc-gateway">—</span>`
   - 按钮：`<button id="btn-cloud" class="btn btn-primary">切换到云端</button>`、`<button id="btn-local" class="btn">切换到本地</button>`、`<button id="btn-gw-restart" class="btn btn-warn">重启网关</button>`、`<button id="btn-gw-force" class="btn btn-danger">强制重启</button>`
   - `<div id="oc-msg" class="dim"></div>`，并附一句提示：「重启网关会断开当前会话，请先确认。」
7. `<section>`：`<h2>自定义加载</h2>`
   - `<select id="custom-model"></select>`、`<input id="custom-port" type="number" value="1240">`、`<input id="custom-ngl" type="number" value="999">`、`<input id="custom-ctx" type="number" value="32768">`
   - 每个输入框前有 `<label>` 说明（模型文件 / 端口 / GPU 层数 / 上下文长度）
   - `<button id="btn-custom-load" class="btn btn-primary">加载</button>`、`<button id="btn-custom-stop" class="btn btn-danger">停止</button>`
   - `<div id="custom-status" class="dim">—</div>`
8. `<section>`：`<h2>对话测试</h2>`
   - `<select id="chat-port"></select>`、`<textarea id="chat-input" rows="3" placeholder="输入一句话，回车发送（Shift+回车换行）"></textarea>`
   - `<button id="btn-chat-send" class="btn btn-primary">发送</button>`、`<button id="btn-chat-clear" class="btn">清空</button>`
   - `<pre id="chat-out"></pre>`
9. 末尾：`<footer>` 写一句「llama-panel · 本页面由 GPU 机本地模型生成」

## 要求

- 语义清晰，不要嵌套超过 4 层；不要用 `table` 做布局
- 不写 `onclick` 等行内事件（app.js 用 `addEventListener` 绑定）
- 不要在文件里出现任何省略记号（如 `<!-- 更多略 -->`）
