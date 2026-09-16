package main

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var unitNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,29}$`)

// daemonReload 需要提权
func daemonReload() error {
	out, err := privRun("daemon-reload")
	if err != nil {
		return fmt.Errorf("daemon-reload 失败：%v（%s）", err, out)
	}
	return nil
}

// writeUnitFile 写 unit 文件需要提权；内容经 stdin 交给 helper 校验后落盘。
func writeUnitFile(name, body string) error {
	out, err := privRunStdin(body, "unit-write", name)
	if err != nil {
		return fmt.Errorf("写入 unit 文件失败：%v（%s）", err, out)
	}
	return nil
}

// unitTemplate 按 kind 生成 unit 文件。模板抄自机器上现有 unit（llama-general / llama-embed）。
func unitTemplate(u *UnitDef) string {
	others := make([]string, 0, len(unitDefs))
	for _, n := range chatUnits() {
		if n != u.Name {
			others = append(others, n+".service")
		}
	}

	var sb strings.Builder
	sb.WriteString("[Unit]\n")
	sb.WriteString("Description=llama.cpp - " + u.Label + "\n")
	sb.WriteString("After=network-online.target\n")
	sb.WriteString("Wants=network-online.target\n")
	// embed 与 rerank 体量小、可与对话模型共存，不参与互斥
	if u.Kind != "embed" && u.Kind != "rerank" && len(others) > 0 {
		sb.WriteString("Conflicts=" + strings.Join(others, " ") + "\n")
	}

	sb.WriteString("\n[Service]\n")
	sb.WriteString("Type=simple\nUser=user\nGroup=user\n")
	sb.WriteString("WorkingDirectory=" + modelsDir + "\n")

	d := u.Defaults
	switch u.Kind {
	case "embed":
		sb.WriteString(fmt.Sprintf("Environment=\"MODEL=%s\" \"PORT=%d\" \"ALIAS=%s\" \"NGL=%s\" \"CTX=%s\" \"EXTRA=\" \"OPT=\"\n",
			d["MODEL"], u.Port, u.Alias, d["NGL"], d["CTX"]))
		sb.WriteString("EnvironmentFile=-" + envDir + "/" + envFileName(u) + ".env\n")
		sb.WriteString("ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model \"${MODEL}\" --host 0.0.0.0 --port \"${PORT}\" --alias \"${ALIAS}\" --embeddings -c \"${CTX}\" --n-gpu-layers \"${NGL}\" $$OPT $$EXTRA'\n")
	case "rerank":
		sb.WriteString(fmt.Sprintf("Environment=\"MODEL=%s\" \"PORT=%d\" \"ALIAS=%s\" \"NGL=%s\" \"CTX=%s\" \"EXTRA=\" \"OPT=%s\"\n",
			d["MODEL"], u.Port, u.Alias, d["NGL"], d["CTX"], d["OPT"]))
		sb.WriteString("EnvironmentFile=-" + envDir + "/" + envFileName(u) + ".env\n")
		sb.WriteString("ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model \"${MODEL}\" --host 0.0.0.0 --port \"${PORT}\" --alias \"${ALIAS}\" -c \"${CTX}\" --n-gpu-layers \"${NGL}\" $$OPT $$EXTRA'\n")
	case "vision":
		sb.WriteString(fmt.Sprintf("Environment=\"MODEL=%s\" \"MMPROJ=%s\" \"PORT=%d\" \"ALIAS=%s\" \"NGL=%s\" \"CTX=%s\" \"EXTRA=\" \"OPT=%s\"\n",
			d["MODEL"], d["MMPROJ"], u.Port, u.Alias, d["NGL"], d["CTX"], d["OPT"]))
		sb.WriteString("EnvironmentFile=-" + envDir + "/" + envFileName(u) + ".env\n")
		sb.WriteString("ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model \"${MODEL}\" --mmproj \"${MMPROJ}\" --host 0.0.0.0 --port \"${PORT}\" --alias \"${ALIAS}\" --n-gpu-layers \"${NGL}\" --ctx-size \"${CTX}\" --jinja $$OPT $$EXTRA'\n")
	case "dense":
		sb.WriteString(fmt.Sprintf("Environment=\"MODEL=%s\" \"PORT=%d\" \"ALIAS=%s\" \"NGL=%s\" \"CTX=%s\" \"CTKV=%s\" \"CTVV=%s\" \"EXTRA=\" \"OPT=%s\"\n",
			d["MODEL"], u.Port, u.Alias, d["NGL"], d["CTX"], d["CTKV"], d["CTVV"], d["OPT"]))
		sb.WriteString("EnvironmentFile=-" + envDir + "/" + envFileName(u) + ".env\n")
		sb.WriteString("ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model \"${MODEL}\" --host 0.0.0.0 --port \"${PORT}\" --alias \"${ALIAS}\" --n-gpu-layers \"${NGL}\" --ctx-size \"${CTX}\" --cache-type-k \"${CTKV}\" --cache-type-v \"${CTVV}\" $$OPT $$EXTRA'\n")
	default: // moe / custom
		sb.WriteString(fmt.Sprintf("Environment=\"MODEL=%s\" \"PORT=%d\" \"ALIAS=%s\" \"NGL=%s\" \"NCPUMOE=%s\" \"CTX=%s\" \"CTKV=%s\" \"CTVV=%s\" \"EXTRA=\" \"OPT=%s\"\n",
			d["MODEL"], u.Port, u.Alias, d["NGL"], d["NCPUMOE"], d["CTX"], d["CTKV"], d["CTVV"], d["OPT"]))
		sb.WriteString("EnvironmentFile=-" + envDir + "/" + envFileName(u) + ".env\n")
		sb.WriteString("ExecStart=/bin/sh -c 'exec /usr/local/bin/llama-server --model \"${MODEL}\" --host 0.0.0.0 --port \"${PORT}\" --alias \"${ALIAS}\" --n-gpu-layers \"${NGL}\" --n-cpu-moe \"${NCPUMOE}\" --ctx-size \"${CTX}\" --cache-type-k \"${CTKV}\" --cache-type-v \"${CTVV}\" $$OPT $$EXTRA'\n")
	}

	if u.Name == "llama-custom" {
		sb.WriteString("Restart=no\n")
	} else {
		sb.WriteString("Restart=on-failure\nRestartSec=10\n")
	}
	sb.WriteString("LimitNOFILE=65535\n\n[Install]\nWantedBy=multi-user.target\n")
	return sb.String()
}

// rewriteAllConflicts 让所有对话 unit 的 Conflicts= 行互相覆盖到最新的模型全集。
// 增删模型之后必须调，否则新模型不会被其它 unit 踢掉（互斥失效）。
func rewriteAllConflicts() error {
	all := chatUnits()
	for _, name := range all {
		path := unitFilePath(name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue // 没安装的跳过
		}
		others := make([]string, 0, len(all))
		for _, n := range all {
			if n != name {
				others = append(others, n+".service")
			}
		}
		if len(others) == 0 {
			continue
		}
		newLine := "Conflicts=" + strings.Join(others, " ")
		lines := strings.Split(string(data), "\n")

		idx := -1
		for i, l := range lines {
			if strings.HasPrefix(l, "Conflicts=") {
				idx = i
				break
			}
		}
		if idx >= 0 {
			lines[idx] = newLine
		} else {
			insertAt := -1
			for i, l := range lines {
				if strings.HasPrefix(l, "Wants=") || strings.HasPrefix(l, "Description=") {
					insertAt = i
				}
			}
			if insertAt < 0 {
				continue
			}
			tail := append([]string{}, lines[insertAt+1:]...)
			lines = append(append(lines[:insertAt+1], newLine), tail...)
		}
		if err := writeUnitFile(name, strings.Join(lines, "\n")); err != nil {
			return err
		}
	}
	return nil
}

type modelAddReq struct {
	Name      string            `json:"name"`
	Label     string            `json:"label"`
	Kind      string            `json:"kind"`
	ModelFile string            `json:"model_file"`
	Port      int               `json:"port"`
	Defaults  map[string]string `json:"defaults"`
}

func handleModelAdd(w http.ResponseWriter, r *http.Request) {
	var req modelAddReq
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := addModel(req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	name := "llama-" + sanitizeUnitName(strings.TrimPrefix(strings.TrimSpace(req.Name), "llama-"))
	port := req.Port
	if port == 0 {
		for i := range unitDefs {
			if unitDefs[i].Name == name {
				port = unitDefs[i].Port
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "name": name, "port": port,
		"msg": fmt.Sprintf("已添加 %s（端口 %d），已加入互斥、仪表盘与调试页", name, port),
	})
}

func handleModelDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	u := unitDefByName(strings.TrimSpace(req.Name))
	if u == nil {
		fail(w, http.StatusNotFound, "未知模型："+req.Name)
		return
	}
	if u.Name == "llama-custom" || u.Name == "llama-embed" {
		fail(w, http.StatusBadRequest, u.Name+" 是固定入口，不能删除")
		return
	}

	if unitActive(u.Name) {
		_ = unitAction(u.Name, "stop")
	}
	_, _ = systemctlOutput("disable", u.Name)
	if err := os.Remove(unitFilePath(u.Name)); err != nil && !os.IsNotExist(err) {
		fail(w, http.StatusInternalServerError, "删除 unit 失败："+err.Error())
		return
	}
	_ = os.Remove(envPath(u.Name))

	kept := make([]userUnit, 0, len(loadUserUnits()))
	for _, uu := range loadUserUnits() {
		if uu.Name != u.Name {
			kept = append(kept, uu)
		}
	}
	if err := saveUserUnits(kept); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	rebuildUnitDefs()
	if err := rewriteAllConflicts(); err != nil {
		fail(w, http.StatusInternalServerError, "重写互斥失败："+err.Error())
		return
	}
	if err := daemonReload(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	msg := "已删除 " + u.Name
	if !u.UserAdded {
		msg += "（内置模型，可随时「恢复」重建）"
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": msg})
}

// handleModelRestore 重建一个被删掉的内置模型 unit。参数取 schema.json 里的定义。
func handleModelRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	u := unitDefByName(strings.TrimSpace(req.Name))
	if u == nil {
		fail(w, http.StatusNotFound, "未知模型："+req.Name)
		return
	}
	if u.UserAdded {
		fail(w, http.StatusBadRequest, "用户添加的模型请用「添加模型」重新登记")
		return
	}
	if installedUnit(u.Name) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": u.Name + " 已存在，无需恢复"})
		return
	}
	if err := writeEnv(u.Name, defaultParams(u)); err != nil {
		fail(w, http.StatusInternalServerError, "写参数文件失败："+err.Error())
		return
	}
	if err := writeUnitFile(u.Name, unitTemplate(u)); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := rewriteAllConflicts(); err != nil {
		fail(w, http.StatusInternalServerError, "重写互斥失败："+err.Error())
		return
	}
	if err := daemonReload(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "已恢复 " + u.Name})
}

// handleModelFiles 列出 /home/user/models 下的 gguf，供「添加模型」下拉选择
func handleModelFiles(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取模型目录失败："+err.Error())
		return
	}
	type fileInfo struct {
		Name  string `json:"name"`
		SizeM int64  `json:"size_mib"`
		Used  bool   `json:"used"`
	}
	files := make([]fileInfo, 0)
	used := make(map[string]bool)
	for i := range unitDefs {
		if unitDefs[i].Model != "" {
			used[unitDefs[i].Model] = true
		}
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".gguf") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			Name:  e.Name(),
			SizeM: info.Size() / 1024 / 1024,
			Used:  used[e.Name()],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "dir": modelsDir, "files": files,
		"free_port": nextFreePort(), "kinds": []string{"moe", "dense", "embed"},
	})
}

// handlePanelInfo 设置页用的面板自身信息
func handlePanelInfo(w http.ResponseWriter, r *http.Request) {
	exe, _ := os.Executable()
	binMiB := 0.0
	binTime := ""
	if st, err := os.Stat(exe); err == nil {
		binMiB = float64(st.Size()) / 1024 / 1024
		binTime = st.ModTime().Format("2006-01-02 15:04:05")
	}
	host, _ := os.Hostname()
	ups, _ := systemctlOutput("is-active", "llama-panel")

	var fsStat syscall.Statfs_t
	freeGiB, totalGiB := 0.0, 0.0
	if err := syscall.Statfs("/home/user", &fsStat); err == nil {
		freeGiB = float64(fsStat.Bavail) * float64(fsStat.Bsize) / 1024 / 1024 / 1024
		totalGiB = float64(fsStat.Blocks) * float64(fsStat.Bsize) / 1024 / 1024 / 1024
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"host":           host,
		"binary":         exe,
		"binary_mib":     math.Round(binMiB*10) / 10,
		"binary_time":    binTime,
		"listen_port":    listenPort(),
		"uptime_s":       int(time.Since(panelStart).Seconds()),
		"service":        ups,
		"units_file":     userUnitsPath(),
		"models_dir":     modelsDir,
		"disk_free_gib":  math.Round(freeGiB*10) / 10,
		"disk_total_gib": math.Round(totalGiB*10) / 10,
		"go_version":     runtime.Version(),
		"user_units":     len(loadUserUnits()),
	})
}
