package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func systemctlOutput(args ...string) (string, error) {
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func unitActive(unit string) bool {
	out, _ := systemctlOutput("is-active", unit)
	return out == "active"
}

func unitStateText(unit string) string {
	out, _ := systemctlOutput("is-active", unit)
	switch out {
	case "active":
		return "运行中"
	case "inactive":
		return "已停止"
	case "activating":
		return "启动中"
	case "deactivating":
		return "停止中"
	case "failed":
		return "失败"
	default:
		if out == "" {
			return "未知"
		}
		return out
	}
}

func unitEnabled(unit string) bool {
	out, _ := systemctlOutput("is-enabled", unit)
	return out == "enabled"
}

// unitAction 启停需要提权（读操作用 systemctlOutput 直接做即可）
func unitAction(unit, action string) error {
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("不支持的操作：%s", action)
	}
	out, err := privRun("unit", action, unit)
	if err != nil {
		return fmt.Errorf("systemctl %s %s 失败：%v（%s）", action, unit, err, out)
	}
	return nil
}

func envPath(unit string) string {
	name := strings.TrimPrefix(unit, "llama-")
	return "/etc/llama.d/" + name + ".env"
}

func readEnv(unit string) (map[string]string, error) {
	path := envPath(unit)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	result := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		result[key] = value
	}
	return result, nil
}

// writeEnv 写参数文件需要提权 —— 内容经 stdin 交给 helper，
// 由 helper 校验「只允许 KEY=value」后再落盘。
func writeEnv(unit string, params map[string]string) error {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(params[k])
		sb.WriteString("\n")
	}
	out, err := privRunStdin(sb.String(), "env-write", unit)
	if err != nil {
		return fmt.Errorf("写入参数文件失败：%v（%s）", err, out)
	}
	return nil
}

func applyParams(u *UnitDef, params map[string]string) error {
	if u == nil {
		return fmt.Errorf("未知的模型单元")
	}
	err := validateParams(u, params)
	if err != nil {
		return err
	}
	err = writeEnv(u.Name, params)
	if err != nil {
		return err
	}
	err = unitAction(u.Name, "restart")
	if err != nil {
		return err
	}
	return nil
}
