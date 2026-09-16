package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// 面板的可编辑设置与加密密钥存储。
//
// 加密方案（自托管服务的通行做法）：
//   - 主密钥 master.key：32 字节随机，0600，仅 root 可读，首次使用时生成
//   - AES-256-GCM（Go 标准库，无外部依赖），每次写入随机 nonce
//   - 密文落 secrets.enc（0600），写入走 临时文件 + 原子替换
//   - 接口从不回传密钥值，只回「是否已设置」
const (
	panelDir      = "/opt/llama-panel"
	settingsFile  = "settings.json"
	masterKeyFile = "master.key"
	secretsFile   = "secrets.enc"
	panelSSHSub   = ".ssh"
)

// secretSSHKey 是「粘贴进来的私钥」的存储键名
const secretSSHKey = "ssh_private_key"

// 状态文件路径在运行时拼接（见 stateDir()）
func settingsPath() string  { return stateDir() + "/" + settingsFile }
func masterKeyPath() string { return stateDir() + "/" + masterKeyFile }
func secretsPath() string   { return stateDir() + "/" + secretsFile }
func panelSSHDir() string   { return stateDir() + "/" + panelSSHSub }

// panelSettings 跨机控制 OpenClaw 所需的连接信息（都不算密钥，可明文存）
type panelSettings struct {
	SSHHost    string `json:"ssh_host"`
	SSHUser    string `json:"ssh_user"`
	SSHPort    int    `json:"ssh_port"`
	SSHKeyPath string `json:"ssh_key_path"`
	ScriptsDir string `json:"scripts_dir"`
	ListenPort int    `json:"listen_port"`
}

func defaultSettings() panelSettings {
	return panelSettings{
		SSHHost:    "192.0.2.20",
		SSHUser:    "user",
		SSHPort:    22,
		SSHKeyPath: "/root/.ssh/id_ed25519_panel",
		ScriptsDir: "/home/user/bin",
		ListenPort: 8077,
	}
}

func loadSettings() panelSettings {
	s := defaultSettings()
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return defaultSettings()
	}
	d := defaultSettings()
	if strings.TrimSpace(s.SSHHost) == "" {
		s.SSHHost = d.SSHHost
	}
	if strings.TrimSpace(s.SSHUser) == "" {
		s.SSHUser = d.SSHUser
	}
	if s.SSHPort <= 0 || s.SSHPort > 65535 {
		s.SSHPort = d.SSHPort
	}
	if strings.TrimSpace(s.SSHKeyPath) == "" {
		s.SSHKeyPath = d.SSHKeyPath
	}
	if strings.TrimSpace(s.ScriptsDir) == "" {
		s.ScriptsDir = d.ScriptsDir
	}
	if s.ListenPort < 1024 || s.ListenPort > 65535 {
		s.ListenPort = d.ListenPort
	}
	return s
}

// listenPort 返回面板实际监听的端口（设置里改过就用设置里的）
func listenPort() int {
	return loadSettings().ListenPort
}

func saveSettings(s panelSettings) error {
	if err := os.MkdirAll(panelDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath())
}

// ---------- 加密密钥存储 ----------

// loadMasterKey 读取主密钥；不存在则生成 32 字节随机密钥（0600）。
func loadMasterKey() ([]byte, error) {
	if b, err := os.ReadFile(masterKeyPath()); err == nil && len(b) == 32 {
		return b, nil
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(panelDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(masterKeyPath(), key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func secretsLoad() (map[string]string, error) {
	raw, err := os.ReadFile(secretsPath())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	key, err := loadMasterKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return map[string]string{}, errors.New("密钥文件已损坏（长度不足）")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return map[string]string{}, errors.New("密钥解密失败：主密钥可能已被更换")
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return map[string]string{}, err
	}
	return out, nil
}

func secretsSave(m map[string]string) error {
	key, err := loadMasterKey()
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(m)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	out := gcm.Seal(nonce, nonce, plain, nil)
	tmp := secretsPath() + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, secretsPath())
}

// effectiveKeyPath 决定这次 SSH 用哪个私钥文件：
// 若存过「粘贴的私钥」，就解密落盘到 0600 文件再用；否则用设置里的路径。
func effectiveKeyPath() (string, error) {
	sec, err := secretsLoad()
	if err != nil {
		return "", err
	}
	material := strings.TrimSpace(sec[secretSSHKey])
	if material == "" {
		return loadSettings().SSHKeyPath, nil
	}
	if err := os.MkdirAll(panelSSHDir(), 0o700); err != nil {
		return "", err
	}
	p := panelSSHDir() + "/id_ed25519"
	if err := os.WriteFile(p, []byte(material+"\n"), 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// handlePortSet 修改面板监听端口（保存后需重启面板服务才生效）
func handlePortSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port int `json:"port"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Port < 1024 || req.Port > 65535 {
		fail(w, http.StatusBadRequest, "端口必须在 1024-65535（<1024 需要 root 绑定，不推荐）")
		return
	}
	s := loadSettings()
	old := s.ListenPort
	s.ListenPort = req.Port
	if err := saveSettings(s); err != nil {
		fail(w, http.StatusInternalServerError, "保存失败："+err.Error())
		return
	}
	msg := fmt.Sprintf("端口已改为 %d", req.Port)
	if old == req.Port {
		msg = fmt.Sprintf("端口本来就是 %d", req.Port)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "msg": msg, "old_port": old, "new_port": req.Port,
		"note": "需重启面板服务后生效",
	})
}

// handlePanelRestart 重启面板自身。用 detached 子进程 + 延迟，先把响应发出去再重启。
func handlePanelRestart(w http.ResponseWriter, r *http.Request) {
	if out, err := privRun("self-restart"); err != nil {
		fail(w, http.StatusInternalServerError, fmt.Sprintf("无法触发重启：%v（%s）", err, out))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":  true,
		"msg": "已触发面板重启，约 2~3 秒后恢复（页面会自动重连）",
	})
}
