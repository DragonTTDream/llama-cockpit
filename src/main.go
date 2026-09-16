package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed web
var webFS embed.FS

func main() {
	// helper 模式：只应由 sudo 以 root 调用，用于执行极少数必须提权的写操作
	if len(os.Args) > 1 && os.Args[1] == "helper" {
		if os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "helper 必须以 root 身份运行")
			os.Exit(1)
		}
		out, err := helperExec(os.Args[2:], os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if out != "" {
			fmt.Print(out)
		}
		return
	}

	tok, err := loadToken()
	if err != nil {
		log.Fatalf("读取令牌失败：%v", err)
	}

	mux := http.NewServeMux()
	registerAPI(mux, tok)
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("前端资源不可用：%v", err)
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 开发期会频繁更新前端，禁止浏览器启发式缓存，避免旧 JS 坑人
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		http.FileServer(http.FS(sub)).ServeHTTP(w, r)
	}))

	addr := fmt.Sprintf("0.0.0.0:%d", listenPort())
	log.Printf("llama-panel 已启动：http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("监听失败：%v", err)
	}
}
