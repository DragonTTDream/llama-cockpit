file: src/web/app.js
model: general

# 任务 g12：前端逻辑 app.js

产出 `src/web/app.js`，纯浏览器 JS（ES2020，无构建、无框架、**不得出现 `http://` / `https://`**）。
只操作 index.html 里已有的 id（见下），**不得新增/改名**。

## 可用的 id（只有这些）

`error-bar` `gpu-name` `gpu-mem-bar` `gpu-mem-text` `gpu-util` `gpu-temp` `current-model` `mode-badge`
`last-update` `autostart-toggle` `autostart-label` `units` `oc-mode` `oc-primary` `oc-gateway`
`btn-cloud` `btn-local` `btn-gw-restart` `btn-gw-force` `oc-msg` `custom-model` `custom-port`
`custom-ngl` `custom-ctx` `custom-status` `btn-custom-load` `btn-custom-stop` `chat-port`
`chat-input` `btn-chat-send` `btn-chat-clear` `chat-out` `login-layer` `login-token` `btn-login`

## 后端 API（只用这些）

`POST /api/login` `{token}` · `GET /api/state` · `GET /api/schema` · `GET /api/models`
`POST /api/unit/action` `{name,action}` · `POST /api/unit/params` `{name,params}` · `GET /api/unit/defaults?name=`
`POST /api/panel/autostart` `{enabled}` · `POST /api/openclaw/mode` `{mode}` · `POST /api/openclaw/gateway` `{action}`
`POST /api/custom/load` `{model_file,port,params}` · `POST /api/custom/stop` · `POST /api/chat` `{port,message,max_tokens}`

所有响应外壳是 `{ok:true,...}` 或 `{ok:false,error:"中文原因"}`；`/api/login` 之外的接口返回 401 表示未登录。

## 结构

```js
const $ = (id) => document.getElementById(id);
let SCHEMA = {};        // /api/schema 的 params
let LAST = null;        // 最近一次 /api/state
let timer = null;
```

- `async function api(path, opts)`：
  `fetch(path, {headers:{'Content-Type':'application/json'}, ...opts})` →
  若 `status === 401`：显示 `#login-layer` 并 `throw new Error('unauthorized')`；
  否则 `await resp.json()`，`j.ok` 为假时 `throw new Error(j.error || ('HTTP '+resp.status))`；返回 `j`。
- `showError(msg)` / `clearError()`：控制 `#error-bar` 的 `hidden` 与文字；点它自身可关闭（`addEventListener('click')`）。
- `busy(btn, on)`：`btn.disabled = on`。

## 轮询与渲染

- `async function refresh()`：`const s = await api('/api/state'); LAST = s; render(s);`
  失败时 `showError`，401 时静默返回。
- 启动：`loadSchema()`（`GET /api/schema` → `SCHEMA = j.params`，然后 `loadModels()`、`refresh()`），
  之后每 2000ms 调一次 `refresh()`；`document.hidden` 为真时**跳过**这次刷新；
  监听 `visibilitychange`，页面回到前台立刻刷新一次。
- `render(s)`：
  - `#last-update` = `new Date().toLocaleTimeString('zh-CN')`
  - `#gpu-name` = `s.gpu.name`；`#gpu-util` = `GPU 利用率 ${s.gpu.util_pct}%`；`#gpu-temp` = `温度 ${s.gpu.temp_c}℃`
  - `#gpu-mem-bar`.style.width = `(s.gpu.mem_total_mib ? s.gpu.mem_used_mib/s.gpu.mem_total_mib*100 : 0).toFixed(1) + '%'`
  - `#gpu-mem-text` = `${s.gpu.mem_used_mib} / ${s.gpu.mem_total_mib} MiB`
  - 当前模型：从 `s.units` 里找 `active === true && coexist === false` 的第一个，`#current-model` = `` `${u.label}（${u.name}）` ``，找不到填 `未运行`
  - `#mode-badge`：`s.openclaw.mode === 'local' ? '本地模式' : '云端模式'`，
    同时切换类：先 `className = ''` 再设为 `'mode-local'` / `'mode-cloud'`
  - `#autostart-toggle`.checked = `s.panel.autostart`；`#autostart-label` = `开机自启`
  - `#oc-mode` / `#oc-primary` / `#oc-gateway` 由 `s.openclaw` 填（云端/本地、主模型、网关状态）
  - `renderUnits(s)`、`renderChatPorts(s)`

## 模型卡片 `renderUnits(s)`

先清空 `#units`，再对 `s.units` 里每个 unit 生成一个 `<div class="unit-card">`：

```
<div class="unit-head">
  <div>
    <div class="unit-title">{label} <span class="dim">{name}</span></div>
    <div class="unit-sub">端口 {port} · {model}{enabled ? ' · 开机自启' : ''}</div>
  </div>
  <div><span class="status-dot {dot-*}"></span> <span class="dim">{state_label}</span></div>
</div>
<div class="bar"><div class="bar-fill" style="width:{vram_pct}%"></div></div>
<div class="bar-text">{vram_mib} / {mem_total_mib} MiB · {vram_pct}%</div>
<div class="param-grid">…每个参数一行…</div>
<div class="unit-actions">
  <button class="btn" data-act="start">启动</button>
  <button class="btn" data-act="stop">停止</button>
  <button class="btn" data-act="restart">重启</button>
  <button class="btn btn-primary" data-act="save">保存并重启</button>
  <button class="btn" data-act="reset">还原默认</button>
</div>
<div class="unit-msg dim"></div>
```

- `status-dot` 的类：`active && health==='ok'` → `dot-on`；`active` 但不是 ok → `dot-busy`；否则 `dot-off`
- 参数行：`<div class="param-row"><label class="param-label">{label}</label><input class="param-input" data-key="{KEY}"><div class="param-note">{note}（安全值 {safe}）</div></div>`
  输入框 `value` 取 unit.params[KEY]；`label`/`note`/`safe` 从 `SCHEMA[KEY]` 取（查不到就用 KEY 本身）
- 输入时校验：`SCHEMA[KEY].type === 'int'` 且数值超出 `min..max` → 给输入框加类 `param-bad`，
  并把 `.param-note` 文字改成 `超出范围（min-max）`，否则移除 `param-bad` 并还原提示文字
- 按钮点击（用 `addEventListener` 绑到卡片上，通过 `e.target.dataset.act` 分派）：
  - `start` / `stop` / `restart` → `POST /api/unit/action`
  - `save` → 收集该卡片所有 `input[data-key]` 的值为 `params`，`POST /api/unit/params`
  - `reset` → `GET /api/unit/defaults?name={name}`，把返回的 `params` 填回输入框，并在 `.unit-msg` 写「已填入默认值，点『保存并重启』生效」
  - 执行期间：该卡片所有按钮 `disabled`，`.unit-msg` 写「执行中…」；成功后写后端 `msg`，
    并**立刻 `refresh()`**；失败把 `err.message` 写进 `.unit-msg`

## OpenClaw 区

- `#btn-cloud` / `#btn-local` → `POST /api/openclaw/mode`，成功后 `#oc-msg` 写 `msg` 并 `refresh()`
- `#btn-gw-restart` → 先 `confirm('重启网关会断开当前会话，确定继续？')`；
  `#btn-gw-force` → 先 `confirm('强制重启会立即杀掉网关进程，确定继续？')`
  两者都用 `btn-warn` / `btn-danger` 风格；确认后 `POST /api/openclaw/gateway`
- 所有写操作执行期间按钮 `disabled`，失败写进 `#oc-msg`（带 `.msg-err` 类），成功带 `.msg-ok` 类

## 自定义加载

- `loadModels()`：`GET /api/models` → 用返回的 `models` 填 `#custom-model` 的 `<option>`（`value` = `file`，文字 = `file`）
- `#btn-custom-load` → `POST /api/custom/load`
  `{model_file: #custom-model.value, port: Number(#custom-port.value), params: {NGL: #custom-ngl.value, CTX: #custom-ctx.value}}`
  结果写 `#custom-status`
- `#btn-custom-stop` → `POST /api/custom/stop`

## 对话

- `renderChatPorts(s)`：只把 `active === true` 的聊天 unit（`coexist === false`）填进 `#chat-port`；
  先在 `<select>` 里记住当前选中的值，重建 options 后尽量恢复选中
- `#btn-chat-send`：`
  - 消息取 `#chat-input`.value.trim()`，为空直接返回
  - `#chat-out`.textContent 追加 `\n你：` + 消息 + `\n模型：`，清空输入框，发送期间禁用按钮
  - `POST /api/chat`（**不要用 `api()`，因为要读流**），`{port: Number(#chat-port.value), message, max_tokens: 512}`
  - `const reader = resp.body.getReader(); const dec = new TextDecoder(); let buf = '';`
    循环 `await reader.read()`；`done` 时结束；`buf += dec.decode(value, {stream: true})`；
    按 `'\n\n'` 切块，保留最后一段不完整的内容在 `buf`；每块若以 `data:` 开头则
    `JSON.parse(块.slice(5).trim())`：有 `delta` 就追加到 `#chat-out` 并
    `#chat-out`.scrollTop = `#chat-out`.scrollHeight`；有 `error` 就 `showError`；`done === true` 就结束循环
  - 解析异常要 `try/catch` 跳过该块，不能让整个流程崩掉
- `#chat-input` 键盘：`Enter` 且没按 `Shift` → `preventDefault()` 并触发发送
- `#btn-chat-clear` → `#chat-out`.textContent = ''

## 登录

- `#btn-login` → `POST /api/login` `{token: #login-token.value}`；
  成功：给 `#login-layer` 加 `hidden` 属性、清空错误、`loadSchema()`；失败：`showError(err.message)` 并保留遮罩
- `#login-token` 里按 `Enter` 等同点击登录

## 硬要求

- 不得使用 `innerHTML` 拼接来自后端的数据字符串（用 `textContent` / `createElement`），避免注入
- 不得出现任何外部 URL；不得写 `alert`（用 `#error-bar` 与页面内文字反馈，`confirm` 除外）
- 所有函数都要真实实现，禁止 `// TODO`、禁止省略、禁止空函数体
