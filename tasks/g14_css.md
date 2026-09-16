file: src/web/style.css
model: general

# 任务 g14：样式表 v3（四页签 + 下拉框 + 只读卡片）—— **覆盖式重写**

产出 `src/web/style.css`。纯 CSS，**不得有 `@import`、不得出现 `http://` / `https://` / `url(http...)`**，字体只用系统字体栈。

## 主题

- 底色 `#0f1115`，卡片 `#171a21`，边框 `#252a34`，正文 `#e6e9ef`，次要文字 `#8b93a7`
- 强调 `#4f8cff`、成功 `#31c48d`、警告 `#f0a33c`、危险 `#ef4d5a`
- 字体栈 `system-ui, -apple-system, "Noto Sans SC", "Microsoft YaHei", sans-serif`
- `* { box-sizing: border-box }`，`body` 外边距 0、`max-width: 1180px` 居中、左右内边距 16px
- `<h2>` 字号 15px、字色 `#8b93a7`、上方 18px 下方 8px

## 必须实现的类与 id

| 选择器 | 作用 |
|---|---|
| `.dim` | 次要文字 `#8b93a7`，13px |
| `.msg-err` | 红底浅色提示条（`#ef4d5a33` 底 + 左描边 `#ef4d5a`） |
| `.msg-ok` | 绿底浅色提示条（`#31c48d33` 底 + 左描边 `#31c48d`） |
| `.btn` | 按钮：padding 7px 14px、圆角 6px、边框 `#252a34`、底色 `#1c2029`、字色 `#e6e9ef`、指针手型 |
| `.btn:hover` | 底色变亮 `#242a35` |
| `.btn-primary` | 底色 `#4f8cff`、字色白、无边框 |
| `.btn-danger` | 底色 `#ef4d5a`、字色白 |
| `.btn-warn` | 底色 `#f0a33c`、字色 `#1a1a1a` |
| `.btn:disabled` | 不透明度 .45、`cursor: not-allowed` |
| `.bar` | 进度槽：高 10px、圆角 999px、底色 `#252a34`、`overflow: hidden` |
| `.bar-fill` | 填充：高 100%、`transition: width .3s`、底色 `linear-gradient(90deg,#4f8cff,#31c48d)` |
| `.bar-text` | 进度条右侧数字：等宽字体、13px、最小宽 140px |
| `.cores` | 每核占用容器：`display:grid`、`grid-template-columns: repeat(auto-fill, minmax(14px, 1fr))`、`gap:3px`、上边距 6px |
| `.core-bar` | 单核竖条：高 26px、圆角 3px、底色 `#252a34`、`display:flex`、`align-items:flex-end` |
| `.core-fill` | 单核填充：宽 100%、底色 `#4f8cff`、圆角 3px（高度由 JS 设） |
| `.tab` | 页签按钮：无边框、底色透明、字色 `#8b93a7`、padding 8px 14px、下边框 2px 透明、圆角 6px 6px 0 0 |
| `.tab.active` | 字色 `#e6e9ef`、下边框 `#4f8cff`、底色 `#171a21` |
| `#tabs` | `display:flex`、`gap:4px`、下边框 1px `#252a34`、下边距 16px |
| `.unit-card` | 卡片：`#171a21` 底、1px `#252a34` 边框、圆角 10px、padding 14px |
| `.unit-head` | `display:flex`、两端对齐、居中 |
| `.unit-title` | 15px、600 字重 |
| `.unit-sub` | 13px、`#8b93a7` |
| `.status-dot` | 8px 圆点、圆角 50%、`inline-block` |
| `.dot-on` | `#31c48d` + `box-shadow: 0 0 6px #31c48d` |
| `.dot-off` | `#4a5163` |
| `.dot-busy` | `#f0a33c` |
| `.unit-actions` | `display:flex`、`gap:8px`、`flex-wrap:wrap`、上边距 10px |
| `.param-group-title` | 分组标题：13px、`#8b93a7`、上边距 14px、下边距 6px、1px 下虚线 `#252a34` |
| `.param-grid` | `display:grid`、`grid-template-columns: repeat(auto-fill, minmax(260px, 1fr))`、`gap:10px`、上边距 8px |
| `.param-row` | 单参数：纵向排列、`gap:4px` |
| `.param-label` | 13px、`#8b93a7` |
| `.param-note` | 12px、`#6b7385` |
| `.param-bad` | 12px、`#ef4d5a` |
| `.param-disabled` | 整行不透明度 .45；配合 `.param-disabled select, .param-disabled input { cursor: not-allowed }` |
| `select, input[type="number"], input[type="text"], textarea` | 底色 `#0f1115`、1px 边框 `#252a34`、圆角 6px、padding 6px 8px、字色 `#e6e9ef`、宽 100%、`font-family: inherit` |
| `.dash-card` | 只读卡片：同 `.unit-card`，但左边框 3px `#252a34` |
| `.cards` | `display:grid`、`grid-template-columns: repeat(auto-fill, minmax(400px, 1fr))`、`gap:12px` |
| `#chat-out` | `min-height:180px`、`max-height:420px`、`overflow-y:auto`、底色 `#0f1115`、等宽字体、13px、padding 10px、圆角 8px、`white-space:pre-wrap`、`word-break:break-word` |
| `#login-layer` | `position:fixed`、四边 0、`background:rgba(15,17,21,.94)`、`display:flex`、居中、`z-index:100` |
| `#login-layer[hidden]` | `display:none` |
| `#login-msg` | `position:absolute`、`left:0`、`right:0`、`bottom:48px`、居中、13px、`#8b93a7` |
| `#error-bar` | `position:relative`、`z-index:200`、下边距 12px；`#error-bar[hidden] { display:none }` |
| `#panel-dash[hidden], #panel-models[hidden], #panel-load[hidden], #panel-oc[hidden]` | `display:none` |
| `#mode-badge` | 胶囊标签：padding 2px 10px、圆角 999px、`#252a34` 底、13px |
| `.mode-local` | `#31c48d33` 底 |
| `.mode-cloud` | `#4f8cff33` 底 |
| `header` | `display:flex`、两端对齐、居中、`flex-wrap:wrap`、`gap:10px`、下边距 14px |
| `.header-right` | `display:flex`、`gap:12px`、居中、`flex-wrap:wrap`、`align-items:center` |
| `@media (max-width: 640px)` | `.cards`、`.param-grid` 单列 |

## 要求

- 每个选择器都要有实际用途；不要空规则；不要写 `/* 略 */`
- 文件末尾不要留大段注释
