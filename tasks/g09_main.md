file: src/main.go
model: coder

# 任务 g09：程序入口（唯一允许定义 main 的文件）

产出 `src/main.go`，`package main`，只含下面列出的声明。
只用标准库（`embed` `fmt` `io/fs` `log` `net/http`）。
可调用 `loadToken` `registerAPI` `panelPort`。

## 必须有的声明

```go
//go:embed web
var webFS embed.FS

func main()
```

## 行为

`main()` 依次做四件事，任何一步失败用 `log.Fatalf` 退出（**不要 panic**）：

1. `tok, err := loadToken()`；`err != nil` → `log.Fatalf("读取令牌失败：%v", err)`
2. 建路由：
   ```go
   mux := http.NewServeMux()
   registerAPI(mux, tok)
   sub, err := fs.Sub(webFS, "web")
   if err != nil { log.Fatalf("前端资源不可用：%v", err) }
   mux.Handle("/", http.FileServer(http.FS(sub)))
   ```
3. `addr := fmt.Sprintf("0.0.0.0:%d", panelPort)`，并
   `log.Printf("llama-panel 已启动：http://%s", addr)`
4. `if err := http.ListenAndServe(addr, mux); err != nil { log.Fatalf("监听失败：%v", err) }`

## 硬要求

- `//go:embed web` 必须是**紧跟在 `var webFS embed.FS` 上一行**的指令注释，中间不能有空行
- 必须 import `embed`（用于 `embed.FS` 类型）与 `io/fs`
- 不要写任何额外的 flag、不要读环境变量、不要注册 `/health` 之类的额外路由
- 不要重复定义别的文件里已有的函数
- `gofmt` 干净、`go vet` 无输出、`go build` 必须成功
