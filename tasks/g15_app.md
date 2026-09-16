file: src/web/app.js
model: general

# 任务 g15：前端逻辑 v3（四页签 + 全下拉框 + 适用性判断）—— **覆盖式重写**

产出 `src/web/app.js`，纯浏览器 JS（ES2020，无框架、**不得出现 `http://` / `https://`**）。
只操作 index.html 里已有的 id，**不得新增/改名**。

## ⚠️ 三条必须照做的硬约束（前两版就是栽在这里）

1. **`$` 辅助函数必须能吃掉 `#` 前缀**，而调用点写成 `$('#xxx')`：
   ```js
   const $ = (id) => document.getElementById(String(id).replace(/^#/, ''));
   ```
2. **`mem_total_mib` 在顶层 `s.gpu` 上，不在 unit 上**：写 `s.gpu.mem_total_mib`，**绝不能**写 `u.gpu.mem_total_mib`。
3. 顶层的 `$('#xxx')` 事件绑定必须写在文件**最后**（DOMContentLoaded 之前无所谓，但不要在元素被渲染前访问动态创建的元素）。

## 可用的 id

`login-layer` `login-token` `login-msg` `btn-login` `error-bar` `auth-msg`
`current-model` `mode-badge` `last-update` `autostart-toggle` `autostart-label` `auth-toggle` `auth-label`
`tabs` `btn-tab-dash` `btn-tab-models` `btn-tab-load` `btn-tab-oc` `panel-dash` `panel-models` `panel-load` `panel-oc`
`dash-cpu-bar` `dash-cpu-text` `dash-cpu-cores` `dash-load` `dash-mem-bar` `dash-mem-text` `dash-swap-text`
`dash-gpu-bar` `dash-gpu-text` `dash-gpu-util` `dash-gpu-temp` `dash-gpu-name` `dash-units`
`model-select` `model-state` `model-params` `model-msg` `btn-model-start` `btn-model-stop` `btn-model-restart` `btn-params-save` `btn-params-reset`
`custom-model` `custom-port` `custom-params` `custom-status` `btn-custom-load` `btn-custom-stop`
`chat-port` `chat-input` `chat-out` `btn-chat-send` `btn-chat-clear`
`oc-mode` `oc-primary` `oc-gateway` `oc-msg` `btn-cloud` `btn-local` `btn-gw-restart` `btn-gw-force`

## 后端 API

`GET /api/state` · `GET /api/schema` · `GET /api/models` · `POST /api/login {token}`
`GET /api/panel/auth` · `POST /api/panel/auth {enabled}`
`POST /api/panel/autostart {enabled}`
`POST /api/unit/action {name,action}` · `POST /api/unit/params {name,params}` · `GET /api/unit/defaults?name=`
`POST /api/openclaw/mode {mode}` · `POST /api/openclaw/gateway {action}`
`POST /api/custom/load {model_file,port,params}` · `POST /api/custom/stop` · `POST /api/chat {port,message,max_tokens}`

响应外壳：`{ok:true,...}` 或 `{ok:false,error:"中文"}`；401 = 未登录。

## 数据结构

```js
let SCHEMA = null;   // {params:{KEY:{...}}, order:[...], groups:[...], kinds:{kind:[scope,...]}}
let LAST = null;     // 最近一次 /api/state
let MODELS = [];     // /api/models 的 models 数组
let timer = null;
let CURTAB = 'dash';
let MODELKEY = 'llama-general';   // 模型页当前选中的 unit
```

state 结构：
```
s.cpu = {util_pct, cores:[每个核的百分比], count, load1, load5, load15}
s.mem = {total_mib, used_mib, avail_mib, swap_total_mib, swap_used_mib}
s.gpu = {name, mem_used_mib, mem_total_mib, util_pct, temp_c}
s.openclaw = {mode, primary, gateway, reachable}
s.panel = {port, autostart, auth_required, uptime_s}
s.units[i] = {name,label,kind,port,model,active,state,state_label,enabled,vram_mib,vram_pct,health,params,resolved,editable,coexist}
```

## 基础函数

- `api(path, opts)`：`fetch`（带 `Content-Type: application/json`）；`status===401` → 显示 `#login-layer` 并 `throw new Error('unauthorized')`；否则解析 JSON，`j.ok` 为假时 `throw new Error(j.error || 'HTTP '+status)`
- `showError(msg)` / `clearError()`：控制 `#error-bar`（点它自己可关闭）
- `busy(btn, on)`：`btn.disabled = on`
- `pct(a, b)`：`b > 0 ? (a / b * 100) : 0`
- 所有写操作：执行期间禁用按钮 → 成功后 `refresh()` → 失败 `showError(err.message)`

## 页签

- `setTab(name)`：给四个 `#btn-tab-*` 切 `active` 类；`#panel-dash/models/load/oc` 用 `hidden` 属性只显示当前那个；记住 `CURTAB`
- 四个按钮的 click 分别 `setTab('dash'|'models'|'load'|'oc')`

## 顶部与仪表盘

`render(s)` 依次做：
- `#last-update` = 当前本地时间字符串
- `#current-model` = 第一个 `active && !coexist` 的 unit 的 `${label}（${name}）`，找不到填 `未运行`
- `#mode-badge`：`s.openclaw.mode==='local' ? '本地模式' : '云端模式'`；`className` 设为 `'mode-local'` 或 `'mode-cloud'`
- `#autostart-toggle`.checked = `s.panel.autostart`
- `#auth-toggle`.checked = `s.panel.auth_required`

仪表盘：
- CPU：`#dash-cpu-bar`.style.width = `s.cpu.util_pct + '%'`；`#dash-cpu-text` = `CPU ${s.cpu.util_pct}% · ${s.cpu.count} 核`；
  `#dash-load` = `负载 ${load1} / ${load5} / ${load15}`
  `#dash-cpu-cores`：清空后对 `s.cpu.cores` 每个值生成
  `<div class="core-bar"><div class="core-fill" style="height:{v}%"></div></div>`
- 内存：`#dash-mem-bar`.width = `pct(used,total)%`；`#dash-mem-text` = `${used} / ${total} MiB (${pct}%)`；
  `#dash-swap-text` = `Swap ${swap_used} / ${swap_total} MiB`（若 swap_total 为 0 显示 `Swap 未启用`）
- GPU：`#dash-gpu-name` = `s.gpu.name`；`#dash-gpu-bar`.width = `pct(mem_used_mib, mem_total_mib)%`；
  `#dash-gpu-text` = `${mem_used_mib} / ${mem_total_mib} MiB`；`#dash-gpu-util` = `利用率 ${util_pct}%`；
  `#dash-gpu-temp` = `温度 ${temp_c}℃`
- `#dash-units`：清空后对每个 unit 生成**只读**卡片
  `<div class="dash-card">`，内含：
  - `unit-head`：左 `unit-title`（`label` + 右侧 `dim` 的 `name`）+ `unit-sub`（`端口 {port} · {model}{enabled ? ' · 开机自启' : ''}`）；
    右 `<span class="status-dot {dot}"></span> <span class="dim">{state_label}</span>`
  - `.bar > .bar-fill`（宽 `vram_pct%`）+ `.bar-text`（`{vram_mib} / {s.gpu.mem_total_mib} MiB · {vram_pct}%`）
  - 健康：`health === 'ok' ? '服务正常' : (active ? '启动中/异常' : '未运行')`，放 `.dim`
  - **不放任何按钮和输入框**
  - `dot`：`active && health==='ok'` → `dot-on`；`active` → `dot-busy`；否则 `dot-off`

## 模型页（参数调节）

`renderModels(s)`：
- `#model-select`：选项来自 `s.units`（value = `name`，文字 = `` `${label}（${name}）` ``）；
  重建后尽量保持 `MODELKEY` 选中（若已不存在则取第一项）
- `#model-state` = 当前所选 unit 的 `` `${state_label} · 端口 ${port} · ${health}` ``
- 参数区 `#model-params`：调用 `renderParamForm(container, unit, 'models')`

### 参数渲染（模型页与自定义页共用）

`renderParamForm(container, unit, mode)`：
1. `container.innerHTML = ''`；按 `SCHEMA.groups` 的顺序遍历分组；每个分组先插
   `<div class="param-group-title">{分组名}</div>`，再插一个 `.param-grid`
2. 按 `SCHEMA.order` 遍历参数键；跳过 `group` 与当前分组不符的
3. 对每个键 `k`，`spec = SCHEMA.params[k]`，判断适用性：
   ```js
   const scopes = SCHEMA.kinds[unit.kind] || ['all'];
   const ok = scopes.includes(spec.scope);
   ```
   不适用 → 整行加类 `param-disabled`，控件 `disabled`，提示文字前缀 `【该模型不适用】`
4. 控件规则（列在一个 `.param-row` 里）：
   - `spec.type === 'model'`：`<select data-key="{k}">`，选项来自 `MODELS`（value = `/home/user/models/` + `file`，文字 = `file`），
     当前值 `unit.resolved[k]` 选中；若当前值不在列表里，额外插一个同名选项
   - `spec.type === 'str'`：`<input type="text" data-key="{k}">`，值 `unit.resolved[k] || ''`
   - `spec.type === 'enum' || spec.type === 'bool'`：`<select data-key="{k}">`，
     选项 = `spec.options` 原样（第一项通常是空串，显示为 `默认`）
   - `spec.type === 'int' || spec.type === 'float'`：`<select data-key="{k}">`，
     选项 = `spec.options`（空串显示 `默认`）**再加一项** `__custom__` 显示为 `自定义…`；
     若当前值不在选项里，则自动选中 `__custom__` 并让配套的
     `<input type="number" data-custom="{k}" step="any">` 显示且填当前值
   - 控件都加 `data-key`；下面加 `.param-note`，内容 = `` `${spec.note}（安全值 ${spec.safe || '—'}）` ``
5. 选择 `__custom__` 时：显示同行的 number 输入框；否则隐藏它
6. 输入校验：int/float 且值超出 `spec.min..spec.max` → 输入框加类 `param-bad`，`.param-note` 文案改成 `超出范围（{min}-{max}）`

`collectValues(container, unit)`：
- 遍历 `container` 里所有 `select[data-key]` / `input[data-key]`：
  - 值为 `__custom__` 时取同键的 `input[data-custom]` 的值
  - 空串表示「不指定」，原样保留空串
- **只收集 `unit.resolved` 里存在的键**（不适用/未列出的键不要提交）
- 返回 `{KEY: value}`

### 模型页按钮

- `btn-model-start` / `btn-model-stop` / `btn-model-restart` → `POST /api/unit/action {name: MODELKEY, action}`
- `btn-params-save` → `POST /api/unit/params {name: MODELKEY, params: collectValues(...)}`
- `btn-params-reset` → `GET /api/unit/defaults?name=MODELKEY`，把返回的 `params` 当作 `resolved` 重新渲染一次并提示「已填入默认值，点『保存并重启』生效」
- 结果/错误写进 `#model-msg`（成功加 `msg-ok`，失败加 `msg-err`）

## 加载与对话页

- `renderCustom(s)`：填 `#custom-model`（来自 `MODELS`，value = `/home/user/models/` + file）；
  参数区 `#custom-params` 用 `renderParamForm(container, customUnit, 'custom')`，
  其中 `customUnit = {name:'llama-custom', kind:'custom', resolved: (s.units 里 name==='llama-custom' 的那个 resolved)}`；
  若 `s.units` 里没有 `llama-custom`，用空 `resolved`（此时所有控件都按不适用处理）
- `btn-custom-load` → 取 `#custom-model`.value 与 `Number(#custom-port.value)`，
  `params = collectValues(#custom-params, customUnit)`，
  `POST /api/custom/load {model_file: 文件名部分, port, params}`；结果写 `#custom-status`
  （`model_file` 只取最后一段文件名，不要带目录）
- `btn-custom-stop` → `POST /api/custom/stop`
- `renderChatPorts(s)`：把 `active===true && coexist===false` 的 unit 填进 `#chat-port`（重建后尽量恢复选中）
- `btn-chat-send`：消息为空直接返回；`#chat-out` 追加 `\n你：{消息}\n模型：`；清空输入；
  `fetch('/api/chat', {method:'POST', headers:{'Content-Type':'application/json'},
   body: JSON.stringify({port: Number(#chat-port.value), message, max_tokens: 512})})`，
  用 `resp.body.getReader()` + `TextDecoder` 逐块读；按 `'\n\n'` 切块，块以 `data:` 开头则 `JSON.parse(块.slice(5))`；
  有 `delta` 就追加到 `#chat-out` 并滚到底；有 `error` 就 `showError`；`done === true` 结束；解析异常 `try/catch` 跳过该块
- `#chat-input`：`Enter` 且无 `Shift` → `preventDefault()` 并发送
- `btn-chat-clear` → `#chat-out`.textContent = ''

## OpenClaw 页

- `#oc-mode` = `云端模式`/`本地模式`；`#oc-primary` = `s.openclaw.primary`；`#oc-gateway` = `网关 ${s.openclaw.gateway}`
- 四个按钮：`btn-cloud` → mode cloud；`btn-local` → mode local；
  `btn-gw-restart` 先 `confirm('重启网关会断开当前会话，确定继续？')`；
  `btn-gw-force` 先 `confirm('强制重启会立即杀掉网关进程，确定继续？')`
- 结果写 `#oc-msg`（成功 `msg-ok`，失败 `msg-err`）

## 顶部开关与登录

- `#autostart-toggle` change → `POST /api/panel/autostart {enabled: checked}`
- `#auth-toggle` change → `POST /api/panel/auth {enabled: checked}`；
  开启且响应里有 `token` → `#auth-msg`.textContent = `已启用令牌保护，请保存令牌：` + token；
  关闭 → `已关闭令牌保护：局域网内任何人打开本页即可操作`；失败回滚勾选状态
- `doLogin(token)`：`POST /api/login {token}` → 成功：`#login-layer` 加 `hidden`、`localStorage.setItem('panel_ok','1')`、启动轮询、`loadSchema()`；
  失败：写 `#login-msg`（`令牌不正确`）+ 保持遮罩
- `#btn-login` click → `doLogin(#login-token.value.trim())`；`#login-token` 回车等同点击
- 初始化：`DOMContentLoaded` → `loadSchema()` → `refresh()`；若 `LAST` 为空（401）就让登录层留着；
  轮询 `setInterval(refresh, 2000)`（`document.hidden` 时 `refresh` 直接 return）；
  `visibilitychange` 回到前台立刻刷一次
- `loadSchema()`：`GET /api/schema` → `SCHEMA = j`（同时 `GET /api/models` 填 `MODELS`）

## 硬要求

- 禁止 `innerHTML` 拼接后端返回的字符串（用 `textContent` / `createElement`）
- 禁止 `alert`（`confirm` 可以）；禁止任何外部 URL
- 所有函数都要真实实现，禁止 `// TODO`、禁止空函数体、禁止省略
