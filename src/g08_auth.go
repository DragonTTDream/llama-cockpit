package main

import (
	"net/http"
	"os"
)

// authFlagPath 存在 = 需要令牌；默认不存在 = 不启用令牌保护（局域网自用）
const authFlagPath = "/opt/llama-panel/require_token"

func authRequired() bool {
	_, err := os.Stat(authFlagPath)
	return err == nil
}

// handleAuthState 永远开放：前端据此决定要不要弹登录层。不回传令牌。
func handleAuthState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": authRequired()})
}

// handleAuthSet 切换令牌保护。开启时一并返回令牌并给当前浏览器种 Cookie，避免当场把自己踢出去。
func handleAuthSet(tok string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeJSON(r, &body); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Enabled {
			f, err := os.Create(authFlagPath)
			if err != nil {
				fail(w, http.StatusInternalServerError, "开启令牌保护失败："+err.Error())
				return
			}
			f.Close()
			http.SetCookie(w, &http.Cookie{
				Name:     cookieName,
				Value:    tok,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   2592000,
			})
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": true, "token": tok})
			return
		}
		if err := os.Remove(authFlagPath); err != nil && !os.IsNotExist(err) {
			fail(w, http.StatusInternalServerError, "关闭令牌保护失败："+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": false})
	}
}
