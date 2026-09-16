package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// 提权层。
//
// 背景：面板原先以 root 运行，直接 systemctl / 写 /etc。这有两个问题：
//  1. /opt/llama-panel 归 user 所有 ⇒ 任何能登录 user 的人可替换二进制，而服务以 root 跑 ⇒ 本地提权
//  2. 面板自身拥有完整 root 权限，远超它实际需要
//
// 方案（同一份二进制，两种模式）：
//   - 正常模式：以非特权用户 llama-panel 运行，读操作直接做（unit 文件 0644、systemctl 只读无需特权）
//   - helper 模式：`llama-panel helper <verb> ...`，只允许 sudo 调用，逐条严格校验参数
//
// 这样必须提权的只剩「写」：启停/启用 unit、写 env、写 unit 文件、daemon-reload、自重启。
var unitNamePat = regexp.MustCompile(`^llama-[a-z0-9][a-z0-9-]{0,30}$`)

// stateDir 返回可写的状态目录。
// 迁移后（非 root 运行）用 /var/lib/llama-panel；未迁移时沿用 /opt/llama-panel。
// 这样二进制可以归 root 所有（防替换），而状态文件归面板用户所有（可写）。
func stateDir() string {
	if st, err := os.Stat("/var/lib/llama-panel"); err == nil && st.IsDir() {
		return "/var/lib/llama-panel"
	}
	return panelDir
}

func runningAsRoot() bool { return os.Geteuid() == 0 }

func selfExe() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "/opt/llama-panel/llama-panel"
}

// privRun 执行需要 root 的操作。
// 已是 root（未迁移）→ 直接本地执行；非 root（已迁移）→ 经 sudo 调自身 helper 模式。
func privRun(args ...string) (string, error) {
	if runningAsRoot() {
		return helperExec(args, nil)
	}
	full := append([]string{"-n", selfExe(), "helper"}, args...)
	out, err := exec.Command("sudo", full...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// privRunStdin 同 privRun，但把内容经标准输入传给 helper（避免把参数塞进命令行）。
func privRunStdin(stdin string, args ...string) (string, error) {
	if runningAsRoot() {
		return helperExec(args, strings.NewReader(stdin))
	}
	full := append([]string{"-n", selfExe(), "helper"}, args...)
	cmd := exec.Command("sudo", full...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// helperExec 是提权操作的唯一实现处。任何新增的特权能力都必须在这里显式列出并校验参数。
func helperExec(args []string, stdin io.Reader) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("helper 缺少子命令")
	}
	verb := args[0]

	switch verb {
	case "unit":
		if len(args) != 3 {
			return "", fmt.Errorf("用法：unit <start|stop|restart|enable|disable> <llama-name>")
		}
		action, name := args[1], strings.TrimSuffix(args[2], ".service")
		switch action {
		case "start", "stop", "restart", "enable", "disable":
		default:
			return "", fmt.Errorf("不允许的 unit 动作：%s", action)
		}
		if !unitNamePat.MatchString(name) {
			return "", fmt.Errorf("非法 unit 名：%s", name)
		}
		out, err := exec.Command("systemctl", action, name+".service").CombinedOutput()
		return strings.TrimSpace(string(out)), err

	case "daemon-reload":
		out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput()
		return strings.TrimSpace(string(out)), err

	case "env-write":
		if len(args) != 2 {
			return "", fmt.Errorf("用法：env-write <llama-name>（内容经 stdin）")
		}
		name := strings.TrimSuffix(args[1], ".service")
		if !unitNamePat.MatchString(name) {
			return "", fmt.Errorf("非法 unit 名：%s", name)
		}
		body, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			return "", err
		}
		if err := validateEnvBody(string(body)); err != nil {
			return "", err
		}
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			return "", err
		}
		return "", writeFileAtomically(envPath(name), body, 0o644)

	case "env-remove":
		if len(args) != 2 {
			return "", fmt.Errorf("用法：env-remove <llama-name>")
		}
		name := strings.TrimSuffix(args[1], ".service")
		if !unitNamePat.MatchString(name) {
			return "", fmt.Errorf("非法 unit 名：%s", name)
		}
		return "", removeIfExists(envPath(name))

	case "unit-write":
		if len(args) != 2 {
			return "", fmt.Errorf("用法：unit-write <llama-name>（内容经 stdin）")
		}
		name := strings.TrimSuffix(args[1], ".service")
		if !unitNamePat.MatchString(name) {
			return "", fmt.Errorf("非法 unit 名：%s", name)
		}
		body, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			return "", err
		}
		if err := validateUnitBody(string(body)); err != nil {
			return "", err
		}
		return "", writeFileAtomically(unitFilePath(name), body, 0o644)

	case "unit-remove":
		if len(args) != 2 {
			return "", fmt.Errorf("用法：unit-remove <llama-name>")
		}
		name := strings.TrimSuffix(args[1], ".service")
		if !unitNamePat.MatchString(name) {
			return "", fmt.Errorf("非法 unit 名：%s", name)
		}
		return "", removeIfExists(unitFilePath(name))

	case "panel-mode-user":
		return "", switchPanelToUser()

	case "panel-mode-root":
		return "", switchPanelToRoot()

	case "self-restart":
		cmd := exec.Command("sh", "-c", "sleep 1; systemctl restart llama-panel.service")
		if err := cmd.Start(); err != nil {
			return "", err
		}
		go func() { _ = cmd.Wait() }()
		return "", nil

	default:
		return "", fmt.Errorf("未知的 helper 子命令：%s", verb)
	}
}

// validateEnvBody 只允许 KEY=value 形式，且 KEY 限定为大写字母数字下划线。
// 防止有人把 shell 片段塞进 env 文件（env 文件会被 systemd 读取）。
var envKeyPat = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func validateEnvBody(body string) error {
	if len(body) > 1<<20 {
		return fmt.Errorf("内容过大")
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("env 行格式非法（需要 KEY=value）：%s", truncate(line, 60))
		}
		if !envKeyPat.MatchString(parts[0]) {
			return fmt.Errorf("env 变量名非法：%s", truncate(parts[0], 40))
		}
	}
	return nil
}

// validateUnitBody 只接受看起来是 llama unit 的 systemd 单元文件。
// 直接写 /etc/systemd/system 是最高危的操作，这里做最小但有效的约束。
func validateUnitBody(body string) error {
	if len(body) > 1<<20 {
		return fmt.Errorf("内容过大")
	}
	if !strings.HasPrefix(body, "[Unit]") {
		return fmt.Errorf("unit 文件必须以 [Unit] 开头")
	}
	if !strings.Contains(body, "[Service]") || !strings.Contains(body, "[Install]") {
		return fmt.Errorf("unit 文件缺少 [Service] 或 [Install] 段")
	}
	// 不允许指定任意 User=/ExecStart= 去跑别的东西
	execLine := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "ExecStart=") {
			execLine = line
		}
	}
	if !strings.Contains(execLine, "/usr/local/bin/llama-server") {
		return fmt.Errorf("ExecStart 必须是 llama-server（拒绝其它可执行文件）")
	}
	if !strings.Contains(execLine, "$$OPT") && !strings.Contains(execLine, "${MODEL}") {
		return fmt.Errorf("ExecStart 缺少模板变量，疑似伪造")
	}
	return nil
}

func writeFileAtomically(path string, body []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
