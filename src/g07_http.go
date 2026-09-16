package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const tokenFile = "token"
const cookieName = "panel_token"

type ChatReq struct {
	Port      int    `json:"port"`
	Message   string `json:"message"`
	MaxTokens int    `json:"max_tokens"`
}

func tokenPath() string { return stateDir() + "/" + tokenFile }

func loadToken() (string, error) {
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(stateDir(), 0o755)
			buf := make([]byte, 32)
			if _, err := rand.Read(buf); err != nil {
				return "", err
			}
			tok := hex.EncodeToString(buf)
			if err := os.WriteFile(tokenPath(), []byte(tok+"\n"), 0o600); err != nil {
				return "", err
			}
			return tok, nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("请求体格式错误")
	}
	return nil
}

func authed(r *http.Request, tok string) bool {
	if tok == "" {
		return false
	}
	if r.Header.Get("X-Panel-Token") == tok {
		return true
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return cookie.Value == tok
}

func requireAuth(tok string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authRequired() && !authed(r, tok) {
			fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func handleLogin(tok string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		if err := decodeJSON(r, &req); err != nil {
			fail(w, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if req.Token != tok {
			fail(w, http.StatusUnauthorized, "令牌错误")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    tok,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   2592000,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildState())
}

func handleSchema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"params": paramSpecs,
		"order":  paramOrder(),
		"groups": groupsOrder(),
		"kinds":  kindScopes(),
	})
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := listModelFiles()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "models": models})
}

func handleUnitAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	u := unitDefByName(req.Name)
	if u == nil {
		fail(w, http.StatusNotFound, "未知的模型单元")
		return
	}
	if err := unitAction(req.Name, req.Action); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": fmt.Sprintf("已执行 %s", req.Action)})
}

func handleUnitParams(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string            `json:"name"`
		Params  map[string]string `json:"params"`
		Restart *bool             `json:"restart"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	u := unitDefByName(req.Name)
	if u == nil {
		fail(w, http.StatusNotFound, "未知的模型单元")
		return
	}
	restart := true
	if req.Restart != nil {
		restart = *req.Restart
	}
	if err := applyValues(u, req.Params, restart); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := "已保存并重启该模型"
	if !restart {
		msg = "已保存参数（未重启，下次启动生效）"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": msg})
}

func handleUnitDefaults(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	u := unitDefByName(name)
	if u == nil {
		fail(w, http.StatusNotFound, "未知的模型单元")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "params": defaultParams(u)})
}

func handlePanelAutostart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	var action string
	if req.Enabled {
		action = "enable"
	} else {
		action = "disable"
	}
	if _, err := systemctlOutput(action, "llama-panel.service"); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	msg := "已开启开机自启"
	if !req.Enabled {
		msg = "已关闭开机自启"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": msg})
}

func handleOpenclawMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := openclawSetMode(req.Mode); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	msg := "已切换到云端模式"
	if req.Mode == "local" {
		msg = "已切换到本地模式"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": req.Mode, "msg": msg})
}

func handleOpenclawGateway(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := openclawGateway(req.Action); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "已重启网关"})
}

func handleCustomLoad(w http.ResponseWriter, r *http.Request) {
	var req CustomReq
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := customLoad(req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "已加载自定义模型"})
}

func handleCustomStop(w http.ResponseWriter, r *http.Request) {
	if err := customStop(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "已停止自定义模型"})
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatReq
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if req.Message == "" {
		fail(w, http.StatusBadRequest, "消息不能为空")
		return
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 512
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	f, _ := w.(http.Flusher)
	ctx := r.Context()
	err := streamChat(ctx, req.Port, req.Message, req.MaxTokens, func(s string) error {
		b, _ := json.Marshal(map[string]string{"delta": s})
		_, err := fmt.Fprintf(w, "data: %s\n\n", string(b))
		if err != nil {
			return err
		}
		if f != nil {
			f.Flush()
		}
		return nil
	})
	if err != nil {
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
		if f != nil {
			f.Flush()
		}
		return
	}
	_, _ = fmt.Fprintf(w, "data: {\"done\":true}\n\n")
	if f != nil {
		f.Flush()
	}
}

func registerAPI(mux *http.ServeMux, tok string) {
	mux.HandleFunc("POST /api/login", handleLogin(tok))
	mux.HandleFunc("GET /api/panel/auth", handleAuthState)
	mux.HandleFunc("POST /api/panel/auth", requireAuth(tok, handleAuthSet(tok)))
	mux.HandleFunc("GET /api/state", requireAuth(tok, handleState))
	mux.HandleFunc("GET /api/schema", requireAuth(tok, handleSchema))
	mux.HandleFunc("GET /api/models", requireAuth(tok, handleModels))
	mux.HandleFunc("POST /api/unit/action", requireAuth(tok, handleUnitAction))
	mux.HandleFunc("POST /api/unit/params", requireAuth(tok, handleUnitParams))
	mux.HandleFunc("GET /api/unit/defaults", requireAuth(tok, handleUnitDefaults))
	mux.HandleFunc("POST /api/panel/autostart", requireAuth(tok, handlePanelAutostart))
	mux.HandleFunc("POST /api/openclaw/mode", requireAuth(tok, handleOpenclawMode))
	mux.HandleFunc("POST /api/openclaw/gateway", requireAuth(tok, handleOpenclawGateway))
	mux.HandleFunc("POST /api/custom/load", requireAuth(tok, handleCustomLoad))
	mux.HandleFunc("POST /api/custom/stop", requireAuth(tok, handleCustomStop))
	mux.HandleFunc("POST /api/chat", requireAuth(tok, handleChat))

	// 模型管理（添加 / 删除 / 恢复 / 磁盘上的模型文件）
	mux.HandleFunc("GET /api/model-files", requireAuth(tok, handleModelFiles))
	mux.HandleFunc("POST /api/models/add", requireAuth(tok, handleModelAdd))
	mux.HandleFunc("POST /api/models/delete", requireAuth(tok, handleModelDelete))
	mux.HandleFunc("POST /api/models/restore", requireAuth(tok, handleModelRestore))
	mux.HandleFunc("GET /api/panel/info", requireAuth(tok, handlePanelInfo))

	// 抱脸虫模型仓库（搜索 / 列文件 / 下载 / 进度 / 取消）
	mux.HandleFunc("GET /api/hf/search", requireAuth(tok, handleHFSearch))
	mux.HandleFunc("GET /api/hf/files", requireAuth(tok, handleHFFiles))
	mux.HandleFunc("POST /api/hf/download", requireAuth(tok, handleHFDownload))
	mux.HandleFunc("GET /api/hf/progress", requireAuth(tok, handleHFProgress))
	mux.HandleFunc("POST /api/hf/cancel", requireAuth(tok, handleHFCancel))

	// 设置页：跨机连接信息与加密密钥
	mux.HandleFunc("GET /api/panel/ssh", requireAuth(tok, handleSSHGet))
	mux.HandleFunc("POST /api/panel/ssh", requireAuth(tok, handleSSHSet))
	mux.HandleFunc("POST /api/panel/ssh/secret", requireAuth(tok, handleSSHSecret))
	mux.HandleFunc("POST /api/panel/ssh/test", requireAuth(tok, handleSSHTest))
	mux.HandleFunc("POST /api/panel/port", requireAuth(tok, handlePortSet))
	mux.HandleFunc("POST /api/panel/restart", requireAuth(tok, handlePanelRestart))
	mux.HandleFunc("GET /api/panel/mode", requireAuth(tok, handlePanelModeGet))
	mux.HandleFunc("POST /api/panel/mode", requireAuth(tok, handlePanelModeSet))
}
