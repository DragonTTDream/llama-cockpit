file: src/web/style.css
model: general

# 任务 g10：样式表 style.css

产出 `src/web/style.css`，纯 CSS，**不得有 `@import`、不得出现任何 `http://` / `https://` / `url(http...)`**，
不得引用外部字体（字体只用系统字体栈）。

## 主题与基础

- 暗色：页面底色 `#0f1115`，卡片 `#171a21`，边框 `#252a34`，正文 `#e6e9ef`，次要文字 `#8b93a7`
- 强调色 `#4f8cff`；成功 `#31c48d`；警告 `#f0a33c`；危险 `#ef4d5a`
- 字体栈：`system-ui, -apple-system, "Noto Sans SC", "Microsoft YaHei", sans-serif`
- `* { box-sizing: border-box }`，`body` 外边距 0，`max-width: 1180px` 居中，左右内边距 16px
- `<section>` 之间 14px 间距；`<h2>` 字号 15px、字色 `#8b93a7`、下方 8px

## 必须实现的类（app.js 会用到，名字一字不差）

| 类 | 作用 |
|---|---|
| `.dim` | 次要文字色 `#8b93a7`，字号 13px |
| `.msg-err` | 红底浅色提示条（`#ef4d5a` 20% 透明底 + 红色左描边） |
| `.msg-ok` | 绿底浅色提示条（`#31c48d` 20% 透明底 + 绿色左描边） |
| `.btn` | 通用按钮：padding 7px 14px、圆角 6px、边框 `#252a34`、底色 `#1c2029`、字色 `#e6e9ef`、指针手型、hover 变亮 |
| `.btn-primary` | 底色 `#4f8cff`、字色白、无边框 |
| `.btn-danger` | 底色 `#ef4d5a`、字色白、无边框 |
| `.btn-warn` | 底色 `#f0a33c`、字色 `#1a1a1a`、无边框 |
| `.btn:disabled` | 不透明度 0.5、`cursor: not-allowed` |
| `.bar` | 进度条外槽：高 10px、圆角 999px、底色 `#252a34`、`overflow: hidden` |
| `.bar-fill` | 内填充：高 100%、`transition: width .3s`、底色渐变（`#4f8cff → #31c48d`） |
| `.bar-text` | 进度条右侧数字，等宽字体、13px、最小宽 130px |
| `.unit-card` | 模型卡片：`background:#171a21`、1px 边框、圆角 10px、padding 14px、下边距 12px |
| `.unit-head` | 卡片首行：`display:flex`、`justify-content:space-between`、`align-items:center` |
| `.unit-title` | 卡片标题：15px、600 字重 |
| `.unit-sub` | 标题下的小字：13px、`#8b93a7` |
| `.status-dot` | 8px 圆点、`inline-block`、圆角 50% |
| `.dot-on` | 底色 `#31c48d`，带柔和外发光 `box-shadow: 0 0 6px #31c48d` |
| `.dot-off` | 底色 `#4a5163` |
| `.dot-busy` | 底色 `#f0a33c` |
| `.unit-actions` | 按钮行：`display:flex`、`gap:8px`、`flex-wrap:wrap`、上边距 10px |
| `.param-grid` | 参数区：`display:grid`、`grid-template-columns: repeat(auto-fill, minmax(240px, 1fr))`、`gap:10px`、上边距 12px |
| `.param-row` | 单个参数：纵向排列、`gap:4px` |
| `.param-label` | 参数名标签：13px、`#8b93a7` |
| `.param-input` | 输入框：底色 `#0f1115`、1px 边框 `#252a34`、圆角 6px、padding 6px 8px、字色 `#e6e9ef`、宽 100% |
| `.param-note` | 参数提示：12px、`#6b7385` |
| `.param-bad` | 越界提示：12px、`#ef4d5a`（同时给输入框加红边框：`.param-input.param-bad { border-color:#ef4d5a }`） |

## 布局

- `#login-layer`：`position:fixed`、四边 0、`background:rgba(15,17,21,.92)`、`display:flex` 居中、
  `z-index:100`；其内的卡片宽 340px、居中文字。**默认 `display:flex`（可见）**，JS 会加 `hidden` 属性隐藏它——
  因此必须写 `#login-layer[hidden] { display:none }`
- `#error-bar`：上边距 0 下边距 12px，带 `.msg-err` 的观感；`#error-bar[hidden] { display:none }`
- `#units`：`display:grid`、`grid-template-columns: repeat(auto-fill, minmax(430px, 1fr))`、`gap:12px`
- `#chat-out`：`min-height:180px`、`max-height:420px`、`overflow-y:auto`、底色 `#0f1115`、
  等宽字体栈 `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`、13px、padding 10px、圆角 8px、
  `white-space:pre-wrap`、`word-break:break-word`
- `#gpu-util` / `#gpu-temp` / `#current-model` / `#mode-badge`：`#mode-badge` 做成胶囊标签
  （padding 2px 10px、圆角 999px、`#252a34` 底）；模式是 local 时由 JS 加类 `.mode-local`（底色 `#31c48d33`），
  cloud 时加 `.mode-cloud`（底色 `#4f8cff33`）——这两个类也要写
- `@media (max-width: 640px)`：`#units` 单列；`.param-grid` 单列

## 要求

- 每个选择器都要有实际用途，不要写空规则、不要写 `/* 略 */`
- 文件末尾不要留大段注释
