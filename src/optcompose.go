package main

import (
	"fmt"
	"strings"
)

// looksLikeValue 判断 OPT 里某个旗标后面紧跟的 token 是不是它的取值。
// 关键点：`--seed -1` 这种负数值必须被认成取值，不能被当成下一个旗标。
func looksLikeValue(t string) bool {
	if t == "" {
		return false
	}
	if !strings.HasPrefix(t, "-") {
		return true
	}
	if len(t) > 1 && t[1] >= '0' && t[1] <= '9' {
		return true
	}
	return strings.HasPrefix(t, "-.")
}

// parseOpt 把 OPT 字符串解析成「旗标 → 取值」；裸旗标（如 --jinja）取值记为空串。
func parseOpt(opt string) map[string]string {
	out := map[string]string{}
	fields := strings.Fields(opt)
	for i := 0; i < len(fields); i++ {
		t := fields[i]
		if !strings.HasPrefix(t, "-") {
			continue
		}
		val := ""
		if i+1 < len(fields) && looksLikeValue(fields[i+1]) {
			val = fields[i+1]
			i++
		}
		out[t] = val
	}
	return out
}

// kindScopes 返回「unit 类型 → 适用的 scope 列表」，前端据此决定某个参数是否可选。
func kindScopes() map[string][]string {
	return kinds
}

// resolveValues 把 .env 的原始 map 解析成「参数名 → 当前取值」，供前端回显。
// 未指定的可选参数给空串（前端显示为「默认」）。
func resolveValues(u *UnitDef, env map[string]string) map[string]string {
	out := make(map[string]string, len(u.Keys))
	for _, k := range u.Keys {
		out[k] = ""
	}
	flags := parseOpt(env["OPT"])
	for _, k := range u.Keys {
		spec, ok := paramSpecs[k]
		if !ok {
			out[k] = env[k]
			continue
		}
		if spec.Core {
			out[k] = env[k]
			continue
		}
		if spec.Flag == "" {
			continue
		}
		if spec.Type == "bool" {
			_, on := flags[spec.Flag]
			_, off := flags[spec.OffFlag]
			switch {
			case spec.OffFlag != "" && off:
				out[k] = "off"
			case on:
				out[k] = "on"
			default:
				out[k] = ""
			}
			continue
		}
		if v, ok2 := flags[spec.Flag]; ok2 {
			out[k] = v
		}
	}
	return out
}

// buildEnvFromValues 把「参数名 → 取值」组装成要写进 .env 的 map：
// Core 参数各自一个变量，其余全部拼进 OPT。
func buildEnvFromValues(u *UnitDef, values map[string]string) (map[string]string, error) {
	if err := validateParams(u, values); err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, k := range u.Keys {
		spec, ok := paramSpecs[k]
		if !ok || spec.Core {
			env[k] = strings.TrimSpace(values[k])
		}
	}
	env["OPT"] = composeOpt(u, values)
	return env, nil
}

// composeOpt 把非核心参数按规格拼成命令行串。
// finalizeUnitDef 与 buildEnvFromValues 必须共用它 —— 否则"默认值算一套、保存时算另一套"，
// 会出现默认 OPT 与实际写入的 OPT 不一致（rerank 曾因此拿到对话模型的默认串）。
func composeOpt(u *UnitDef, values map[string]string) string {
	var opt []string
	for _, k := range u.Keys {
		spec, ok := paramSpecs[k]
		if !ok || spec.Core {
			continue
		}
		v := strings.TrimSpace(values[k])
		if spec.Flag == "" || v == "" {
			continue
		}
		if spec.Type == "bool" {
			switch v {
			case "on":
				opt = append(opt, spec.Flag)
			case "off":
				if spec.OffFlag != "" {
					opt = append(opt, spec.OffFlag)
				}
			}
			continue
		}
		opt = append(opt, spec.Flag, v)
	}
	return strings.Join(opt, " ")
}

// applyValues 校验 → 组装 → 写 .env → 重启该 unit。
func applyValues(u *UnitDef, values map[string]string, restart bool) error {
	if u == nil {
		return fmt.Errorf("未知的模型单元")
	}
	merged := defaultParams(u)
	for k, v := range values {
		merged[k] = v
	}
	env, err := buildEnvFromValues(u, merged)
	if err != nil {
		return err
	}
	if err := writeEnv(u.Name, env); err != nil {
		return err
	}
	if !restart {
		// 只落盘不重启：用于批量下发参数而不惊动正在跑的模型
		return nil
	}
	return unitAction(u.Name, "restart")
}
