package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func jsonMarshalIndent(v any) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// 运行身份切换（root ↔ 专用非特权用户）。
//
// 这是「会改自己 unit 文件并重启自己」的操作，属于最容易把自己搞挂的一类，
// 所以内置了三道保险：
//  1. 改 unit 前先备份
//  2. 切换后由【脱离进程的看门狗】验证；起不来就自动回滚到 root
//  3. 绝不再使用 StateDirectory= —— 上次就是它让 systemd(init_t) 去 relabel
//     状态目录树，而树里的 SSH 私钥带 ssh_home_t 标签，SELinux 强制拒绝，
//     服务直接卡在重启循环里。
const (
	panelUnitPath    = "/etc/systemd/system/llama-panel.service"
	panelUserName    = "llama-panel"
	panelSudoersPath = "/etc/sudoers.d/llama-panel"
	newStateDir      = "/var/lib/llama-panel"
)

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// currentPanelMode 只读判断当前身份。unit 文件 0644，普通用户即可读。
func currentPanelMode() string {
	data, err := os.ReadFile(panelUnitPath)
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "User=") {
			u := strings.TrimSpace(strings.TrimPrefix(line, "User="))
			if u != "" && u != "root" {
				return "user"
			}
		}
	}
	return "root"
}

func handlePanelModeGet(w http.ResponseWriter, r *http.Request) {
	mode := currentPanelMode()
	_, uerr := exec.LookPath("useradd")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"mode":       mode,
		"user":       panelUserName,
		"unit":       panelUnitPath,
		"sudoers":    panelSudoersPath,
		"state_dir":  stateDir(),
		"can_switch": uerr == nil,
		"selinux":    strings.TrimSpace(runQuiet("getenforce")),
	})
}

func handlePanelModeSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Mode != "root" && req.Mode != "user" {
		fail(w, http.StatusBadRequest, "模式只能是 root 或 user")
		return
	}
	cur := currentPanelMode()
	if cur == req.Mode {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "当前已经是该模式，无需切换", "mode": cur})
		return
	}
	out, err := privRun("panel-mode-" + req.Mode)
	if err != nil {
		fail(w, http.StatusInternalServerError, fmt.Sprintf("切换失败：%v（%s）", err, out))
		return
	}
	msg := fmt.Sprintf("已切换到非特权用户 %s 运行；面板将在约 3 秒后重启，若起不来会自动回滚", panelUserName)
	if req.Mode == "root" {
		msg = "已切回以 root 运行；面板将在约 3 秒后重启"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": msg, "mode": req.Mode})
}

func runQuiet(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

// ---------------------------------------------------------------- helper 实现

func backupPanelUnit() (string, error) {
	data, err := os.ReadFile(panelUnitPath)
	if err != nil {
		return "", fmt.Errorf("读取 unit 失败：%v", err)
	}
	bak := fmt.Sprintf("%s.bak-%d", panelUnitPath, time.Now().Unix())
	if err := os.WriteFile(bak, data, 0o644); err != nil {
		return "", err
	}
	return bak, nil
}

// setUnitUser 设置/移除 unit 的 User=/Group=，并强制删掉 StateDirectory=。
func setUnitUser(user string) error {
	data, err := os.ReadFile(panelUnitPath)
	if err != nil {
		return err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "User=") || strings.HasPrefix(line, "Group=") ||
			strings.HasPrefix(line, "StateDirectory=") {
			continue // 一律先剔除，后面按需要重新插入
		}
		out = append(out, line)
	}
	body := strings.Join(out, "\n")
	if user != "" {
		// 插到 [Service] 之后
		body = strings.Replace(body, "[Service]",
			fmt.Sprintf("[Service]\nUser=%s\nGroup=%s", user, user), 1)
	}
	if err := os.WriteFile(panelUnitPath, []byte(body), 0o644); err != nil {
		return err
	}
	return nil
}

func copyFileKeepMode(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(dst), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func dirOf(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

// updateKeyPathIn 把 settings.json 里的 ssh_key_path 指向新位置（两个可能的状态目录都处理）
func updateKeyPathIn(key string) {
	for _, dir := range []string{newStateDir, panelDir} {
		p := dir + "/settings.json"
		m := map[string]any{}
		if fileExists(p) {
			if b, err := os.ReadFile(p); err == nil {
				_ = jsonUnmarshal(b, &m)
			}
		}
		m["ssh_key_path"] = key
		if b, err := jsonMarshalIndent(m); err == nil {
			_ = os.WriteFile(p, b, 0o600)
		}
	}
}

// switchPanelToUser 切到专用非特权用户运行
func switchPanelToUser() error {
	if _, err := os.Stat(panelUnitPath); err != nil {
		return fmt.Errorf("找不到 %s", panelUnitPath)
	}
	if _, err := backupPanelUnit(); err != nil {
		return err
	}

	// 1) 建用户
	if exec.Command("id", "-u", panelUserName).Run() != nil {
		if out, err := exec.Command("useradd", "--system", "--no-create-home",
			"--shell", "/usr/sbin/nologin", panelUserName).CombinedOutput(); err != nil {
			return fmt.Errorf("创建用户失败：%v（%s）", err, strings.TrimSpace(string(out)))
		}
	}

	// 2) 状态目录与文件
	if err := os.MkdirAll(newStateDir, 0o700); err != nil {
		return err
	}
	for _, f := range []string{"token", "units.json", "settings.json", "master.key", "secrets.enc"} {
		src, dst := panelDir+"/"+f, newStateDir+"/"+f
		if fileExists(src) && !fileExists(dst) {
			if err := copyFileKeepMode(src, dst, 0o600); err != nil {
				return fmt.Errorf("迁移 %s 失败：%v", f, err)
			}
		}
	}

	// 3) SSH 私钥
	sshDir := newStateDir + "/.ssh"
	newKey := sshDir + "/id_ed25519_panel"
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	if !fileExists(newKey) {
		for _, cand := range []string{stateDir() + "/.ssh/id_ed25519", "/root/.ssh/id_ed25519_panel"} {
			if fileExists(cand) {
				if err := copyFileKeepMode(cand, newKey, 0o600); err != nil {
					return err
				}
				break
			}
		}
	}
	updateKeyPathIn(newKey)

	// 4) 归属（注意：绝不 chown 到别的用户后再让 systemd 去管这块目录）
	if out, err := exec.Command("chown", "-R",
		panelUserName+":"+panelUserName, newStateDir).CombinedOutput(); err != nil {
		return fmt.Errorf("chown 状态目录失败：%v（%s）", err, strings.TrimSpace(string(out)))
	}
	_ = os.Chmod(newStateDir, 0o700)

	// 5) sudoers（先落临时文件，visudo 校验通过才生效）
	tmp := "/etc/sudoers.d/.llama-panel.new"
	rule := fmt.Sprintf("# llama-panel：唯一提权通道，helper 内部逐条校验参数\n"+
		"%s ALL=(root) NOPASSWD: %s helper\n", panelUserName, selfExe())
	if err := os.WriteFile(tmp, []byte(rule), 0o440); err != nil {
		return fmt.Errorf("写 sudoers 失败：%v", err)
	}
	if out, err := exec.Command("visudo", "-c", "-f", tmp).CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sudoers 语法校验失败：%s", strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tmp, panelSudoersPath); err != nil {
		return err
	}
	_ = os.Chmod(panelSudoersPath, 0o440)

	// 6) 收紧二进制目录：这一步堵掉「能登录 user 的人替换二进制」的提权路径
	_, _ = exec.Command("chown", "-R", "root:root", panelDir).CombinedOutput()
	_ = os.Chmod(panelDir, 0o755)
	_ = os.Chmod(panelDir+"/llama-panel", 0o755)

	// 7) 改 unit（绝不加 StateDirectory）
	if err := setUnitUser(panelUserName); err != nil {
		return err
	}
	_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()

	// 8) 脱离进程的看门狗：先重启，再验证，起不来就回滚到 root
	spawnPanelWatchdog(true)
	return nil
}

// switchPanelToRoot 切回 root 运行
func switchPanelToRoot() error {
	if _, err := backupPanelUnit(); err != nil {
		return err
	}
	_ = os.Remove(panelSudoersPath)
	if err := setUnitUser(""); err != nil {
		return err
	}
	_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()
	spawnPanelWatchdog(false)
	return nil
}

// spawnPanelWatchdog 起一个脱离进程的脚本：
//
//	等 3 秒（让 HTTP 响应先发出去）→ 重启面板 → 再等 30 秒看是否 active
//	若失败且刚从 root 切到 user，则自动回滚到 root
//
// 这是防止「改自己配置把自己搞挂」的最后一道保险。
func spawnPanelWatchdog(revertToRoot bool) {
	revert := ""
	if revertToRoot {
		revert = fmt.Sprintf(`
if ! systemctl is-active --quiet llama-panel.service; then
  logger -t llama-panel-watchdog "非 root 模式启动失败，自动回滚到 root"
  systemctl stop llama-panel.service 2>/dev/null || true
  sed -i '/^User=/d;/^Group=/d;/^StateDirectory=/d' %s
  rm -f %s
  systemctl daemon-reload
  systemctl restart llama-panel.service
fi`, panelUnitPath, panelSudoersPath)
	}
	script := fmt.Sprintf(`sleep 3
systemctl restart llama-panel.service
sleep 30%s`, revert)
	cmd := exec.Command("setsid", "sh", "-c", script)
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return
	}
	go func() { _ = cmd.Wait() }()
}
