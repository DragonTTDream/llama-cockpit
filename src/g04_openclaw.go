package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// OpenClawView 是「OpenClaw」页要显示的实时状态。
type OpenClawView struct {
	Mode          string   `json:"mode"`
	Primary       string   `json:"primary"`
	Fallbacks     []string `json:"fallbacks"`
	Gateway       string   `json:"gateway"`
	GatewaySub    string   `json:"gateway_sub"`
	GatewaySince  string   `json:"gateway_since"`
	Restarts      string   `json:"gateway_restarts"`
	GatewayPID    string   `json:"gateway_pid"`
	GatewayMemMiB float64  `json:"gateway_mem_mib"`
	Version       string   `json:"version"`
	Reachable     bool     `json:"reachable"`
	Detail        string   `json:"detail"`
	CheckedAt     string   `json:"checked_at"`
	Target        string   `json:"target"`
	LatencyMs     int64    `json:"latency_ms"`
}

// sshLoong 按「设置页」里的连接信息执行远端脚本。
// 私钥优先用加密存储里粘贴的那把，否则用设置里的路径。
func sshLoong(script string, timeout time.Duration) (string, error) {
	s := loadSettings()
	keyPath, err := effectiveKeyPath()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "ConnectTimeout=5",
		"-p", strconv.Itoa(s.SSHPort),
		"-i", keyPath,
		s.SSHUser + "@" + s.SSHHost,
		script,
	}
	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func openclawTarget() string {
	s := loadSettings()
	return s.SSHUser + "@" + s.SSHHost
}

// openclawInfo 读取实时状态。优先用 user 上的 openclaw-status.sh（输出 JSON），
// 脚本缺失时回退到旧的两行式读取。
func openclawInfo() OpenClawView {
	s := loadSettings()
	view := OpenClawView{
		Mode:      "cloud",
		Primary:   "unknown",
		Gateway:   "unknown",
		Target:    openclawTarget(),
		CheckedAt: time.Now().Format("15:04:05"),
	}

	start := time.Now()
	out, err := sshLoong(s.ScriptsDir+"/openclaw-status.sh", 10*time.Second)
	view.LatencyMs = time.Since(start).Milliseconds()

	if err == nil && strings.HasPrefix(strings.TrimSpace(out), "{") {
		var j struct {
			Primary         string   `json:"primary"`
			Fallbacks       []string `json:"fallbacks"`
			Mode            string   `json:"mode"`
			GatewayState    string   `json:"gateway_state"`
			GatewaySub      string   `json:"gateway_sub"`
			GatewaySince    string   `json:"gateway_since"`
			GatewayRestarts string   `json:"gateway_restarts"`
			GatewayPID      string   `json:"gateway_pid"`
			GatewayMemMiB   float64  `json:"gateway_mem_mib"`
			Version         string   `json:"version"`
		}
		if err := json.Unmarshal([]byte(out), &j); err != nil {
			view.Detail = "状态脚本输出无法解析"
			return view
		}
		view.Primary = j.Primary
		view.Fallbacks = j.Fallbacks
		if j.Mode != "" {
			view.Mode = j.Mode
		} else if strings.Contains(j.Primary, "llamacpp-") {
			view.Mode = "local"
		}
		view.Gateway = j.GatewayState
		view.GatewaySub = j.GatewaySub
		view.GatewaySince = j.GatewaySince
		view.Restarts = j.GatewayRestarts
		view.GatewayPID = j.GatewayPID
		view.GatewayMemMiB = j.GatewayMemMiB
		view.Version = j.Version
		view.Reachable = true
		return view
	}

	// 回退路径：状态脚本没装（例如刚换过脚本目录）
	fallback := s.ScriptsDir + `/openclaw-primary.sh
XDG_RUNTIME_DIR=/run/user/1000 systemctl --user is-active openclaw-gateway 2>/dev/null || echo inactive`
	out2, err2 := sshLoong(fallback, 8*time.Second)
	if err2 != nil {
		view.Detail = fmt.Sprintf("无法连接 %s：%v", view.Target, err2)
		return view
	}
	parts := strings.Split(strings.TrimSpace(out2), "\n")
	if len(parts) < 2 {
		view.Detail = "远端返回内容不完整（可能缺少 openclaw-status.sh）"
		return view
	}
	view.Primary = strings.TrimSpace(parts[0])
	view.Gateway = strings.TrimSpace(parts[1])
	if strings.Contains(view.Primary, "llamacpp-") {
		view.Mode = "local"
	}
	view.Reachable = true
	view.Detail = "使用回退读取（建议部署 openclaw-status.sh 以获得完整状态）"
	return view
}

func openclawSetMode(mode string) error {
	if mode != "cloud" && mode != "local" {
		return fmt.Errorf("不支持的模式：%s", mode)
	}
	s := loadSettings()
	out, err := sshLoong(s.ScriptsDir+"/openclaw-mode.sh "+mode, 180*time.Second)
	if err != nil {
		return fmt.Errorf("切换模式失败：%v（%s）", err, truncate(out, 200))
	}
	return nil
}

func openclawGateway(action string) error {
	if action != "restart" && action != "force" {
		return fmt.Errorf("不支持的操作：%s", action)
	}
	s := loadSettings()
	out, err := sshLoong(s.ScriptsDir+"/openclaw-gateway-ctl.sh "+action, 150*time.Second)
	if err != nil {
		return fmt.Errorf("网关操作失败：%v（%s）", err, truncate(out, 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------- 设置页：SSH 连接信息与密钥 ----------

// handleSSHGet 回传连接设置 + 各密钥「是否已设置」。绝不回传密钥值本身。
func handleSSHGet(w http.ResponseWriter, r *http.Request) {
	s := loadSettings()
	sec, err := secretsLoad()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, keyErr := effectiveKeyPath()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"settings":    s,
		"key_set":     strings.TrimSpace(sec[secretSSHKey]) != "",
		"key_error":   errString(keyErr),
		"key_path":    s.SSHKeyPath,
		"target":      openclawTarget(),
		"master_key":  masterKeyPath(),
		"secrets_enc": secretsPath(),
	})
}

func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

// handleSSHSet 更新连接设置（不含密钥）
func handleSSHSet(w http.ResponseWriter, r *http.Request) {
	var req panelSettings
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.SSHHost) == "" || strings.TrimSpace(req.SSHUser) == "" {
		fail(w, http.StatusBadRequest, "地址与用户名不能为空")
		return
	}
	if req.SSHPort <= 0 || req.SSHPort > 65535 {
		fail(w, http.StatusBadRequest, "端口必须在 1-65535")
		return
	}
	if err := saveSettings(req); err != nil {
		fail(w, http.StatusInternalServerError, "保存失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "连接设置已保存", "target": openclawTarget()})
}

// handleSSHSecret 写入/清除「粘贴的私钥」。只写不回读。
func handleSSHSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PrivateKey string `json:"private_key"` // 空字符串 = 清除
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	sec, err := secretsLoad()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	material := strings.TrimSpace(req.PrivateKey)
	if material == "" {
		delete(sec, secretSSHKey)
	} else {
		if !strings.Contains(material, "PRIVATE KEY") {
			fail(w, http.StatusBadRequest, "这看起来不是一份私钥（缺少 PRIVATE KEY 标记）")
			return
		}
		sec[secretSSHKey] = material
	}
	if err := secretsSave(sec); err != nil {
		fail(w, http.StatusInternalServerError, "加密保存失败："+err.Error())
		return
	}
	// 清除时不留下明文残留
	if material == "" {
		_ = removeIfExists(panelSSHDir() + "/id_ed25519")
	}
	msg := "私钥已加密保存"
	if material == "" {
		msg = "已清除保存的私钥，回退到「密钥路径」"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": msg, "key_set": material != ""})
}

func removeIfExists(p string) error {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// handleSSHTest 实测一次连接（并顺带报告能否读到状态）
func handleSSHTest(w http.ResponseWriter, r *http.Request) {
	s := loadSettings()
	start := time.Now()
	out, err := sshLoong("echo PANEL-OK; hostname; "+s.ScriptsDir+"/openclaw-status.sh 2>/dev/null | head -c 200", 12*time.Second)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "latency_ms": ms,
			"msg":    fmt.Sprintf("连接失败：%v", err),
			"raw":    truncate(out, 300),
			"target": openclawTarget(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "latency_ms": ms,
		"msg":    fmt.Sprintf("连接成功（%d ms）", ms),
		"raw":    truncate(out, 300),
		"target": openclawTarget(),
	})
}
