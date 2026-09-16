package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// 抱脸虫（Hugging Face）搜索与下载。GPU 机可直连 huggingface.co，无需代理。

const hfAPI = "https://huggingface.co/api"
const hfResolve = "https://huggingface.co"

type hfJob struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Status  string `json:"status"` // running | done | failed | cancelled
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	Error   string `json:"error,omitempty"`
	Started string `json:"started"`

	part string
	dest string
}

var (
	hfMu   sync.Mutex
	hfJobs = map[string]*hfJob{}
	hfStop = map[string]context.CancelFunc{}
)

func hfClient() *http.Client {
	return &http.Client{Timeout: 0}
}

func hfGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "llama-panel/1.0")
	return hfClient().Do(req)
}

// handleHFSearch 搜索 gguf 模型仓库
func handleHFSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		fail(w, http.StatusBadRequest, "缺少搜索关键词")
		return
	}
	rawURL := fmt.Sprintf("%s/models?search=%s&filter=gguf&sort=downloads&direction=-1&limit=25",
		hfAPI, url.QueryEscape(q))
	resp, err := hfGet(rawURL)
	if err != nil {
		fail(w, http.StatusBadGateway, "连接抱脸虫失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, http.StatusBadGateway, fmt.Sprintf("抱脸虫返回 %d", resp.StatusCode))
		return
	}
	var items []struct {
		ID        string `json:"id"`
		Downloads int64  `json:"downloads"`
		Likes     int64  `json:"likes"`
		Pipeline  string `json:"pipeline_tag"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		fail(w, http.StatusBadGateway, "解析搜索结果失败："+err.Error())
		return
	}
	type result struct {
		ID        string `json:"id"`
		Downloads int64  `json:"downloads"`
		Likes     int64  `json:"likes"`
		Task      string `json:"task"`
	}
	out := make([]result, 0, len(items))
	for _, it := range items {
		out = append(out, result{ID: it.ID, Downloads: it.Downloads, Likes: it.Likes, Task: it.Pipeline})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "query": q, "results": out})
}

// handleHFFiles 列出一个仓库里的文件（只要 gguf）
func handleHFFiles(w http.ResponseWriter, r *http.Request) {
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if repo == "" {
		fail(w, http.StatusBadRequest, "缺少仓库名")
		return
	}
	// blobs=true 才会带回文件体积，否则 size 全是 0
	resp, err := hfGet(hfAPI + "/models/" + repo + "?blobs=true")
	if err != nil {
		fail(w, http.StatusBadGateway, "连接抱脸虫失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, http.StatusBadGateway, fmt.Sprintf("仓库不存在或抱脸虫返回 %d", resp.StatusCode))
		return
	}
	var doc struct {
		ID       string `json:"id"`
		UsedMiB  int64  `json:"usedStorage"`
		Siblings []struct {
			Name string `json:"rfilename"`
			Size int64  `json:"size"`
		} `json:"siblings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		fail(w, http.StatusBadGateway, "解析仓库信息失败："+err.Error())
		return
	}
	type fileInfo struct {
		Name    string `json:"name"`
		SizeMiB int64  `json:"size_mib"`
		Local   bool   `json:"local"`
		Quant   string `json:"quant"`
	}
	files := make([]fileInfo, 0)
	for _, s := range doc.Siblings {
		if !strings.HasSuffix(strings.ToLower(s.Name), ".gguf") {
			continue
		}
		local := false
		if st, err := os.Stat(filepath.Join(modelsDir, filepath.Base(s.Name))); err == nil && st.Size() > 0 {
			local = true
		}
		files = append(files, fileInfo{
			Name:    s.Name,
			SizeMiB: s.Size / 1024 / 1024,
			Local:   local,
			Quant:   guessQuant(s.Name),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].SizeMiB < files[j].SizeMiB })
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "repo": doc.ID, "files": files})
}

func guessQuant(name string) string {
	up := strings.ToUpper(name)
	for _, q := range []string{"Q2_K", "Q3_K", "Q4_K_M", "Q4_K_S", "Q4_0", "Q4_1", "Q5_K_M", "Q5_K_S", "Q5_0", "Q6_K", "Q8_0", "IQ4_XS", "IQ4_NL", "IQ3_XS", "IQ2_M", "BF16", "F16", "F32"} {
		if strings.Contains(up, q) {
			return q
		}
	}
	return ""
}

// handleHFDownload 启动一个后台下载任务（存 models 目录，完成后自动登记成模型）
func handleHFDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo       string `json:"repo"`
		File       string `json:"file"`
		AutoRegist bool   `json:"auto_register"`
		Port       int    `json:"port"`
		Kind       string `json:"kind"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Repo = strings.TrimSpace(req.Repo)
	req.File = strings.TrimSpace(req.File)
	if req.Repo == "" || req.File == "" {
		fail(w, http.StatusBadRequest, "缺少仓库名或文件名")
		return
	}
	if strings.Contains(req.File, "..") || strings.HasPrefix(req.File, "/") {
		fail(w, http.StatusBadRequest, "文件名非法")
		return
	}
	base := filepath.Base(req.File)
	if !strings.HasSuffix(strings.ToLower(base), ".gguf") {
		fail(w, http.StatusBadRequest, "只支持 .gguf 文件")
		return
	}

	hfMu.Lock()
	for _, j := range hfJobs {
		if j.Status == "running" {
			hfMu.Unlock()
			fail(w, http.StatusConflict, fmt.Sprintf("已有下载在进行：%s（同时只允许一个）", j.File))
			return
		}
	}
	hfMu.Unlock()

	if _, err := os.Stat(filepath.Join(modelsDir, base)); err == nil {
		fail(w, http.StatusConflict, "同名文件已存在："+base)
		return
	}

	id := fmt.Sprintf("j%d", time.Now().UnixNano())
	job := &hfJob{
		ID: id, Repo: req.Repo, File: req.File, Status: "running",
		Started: time.Now().Format("15:04:05"),
		part:    filepath.Join(modelsDir, base+".part"),
		dest:    filepath.Join(modelsDir, base),
	}

	ctx, cancel := context.WithCancel(context.Background())
	hfMu.Lock()
	hfJobs[id] = job
	hfStop[id] = cancel
	hfMu.Unlock()

	go runHfDownload(ctx, job, req)

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "msg": "已开始下载 " + base})
}

func runHfDownload(ctx context.Context, job *hfJob, req struct {
	Repo       string `json:"repo"`
	File       string `json:"file"`
	AutoRegist bool   `json:"auto_register"`
	Port       int    `json:"port"`
	Kind       string `json:"kind"`
}) {
	finish := func(status, errMsg string) {
		hfMu.Lock()
		job.Status = status
		job.Error = errMsg
		delete(hfStop, job.ID)
		hfMu.Unlock()
	}

	segments := strings.Split(req.File, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	base := filepath.Base(req.File)
	rawURL := hfResolve + "/" + req.Repo + "/resolve/main/" + strings.Join(segments, "/")

	hreq, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	hreq.Header.Set("User-Agent", "llama-panel/1.0")

	resp, err := hfClient().Do(hreq)
	if err != nil {
		if ctx.Err() != nil {
			os.Remove(job.part)
			finish("cancelled", "")
			return
		}
		finish("failed", err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		finish("failed", fmt.Sprintf("抱脸虫返回 %d（可能需要登录或该文件不允许匿名下载）", resp.StatusCode))
		return
	}

	hfMu.Lock()
	job.Total = resp.ContentLength
	hfMu.Unlock()

	f, err := os.Create(job.part)
	if err != nil {
		finish("failed", "创建文件失败："+err.Error())
		return
	}
	defer f.Close()

	buf := make([]byte, 1<<20)
	var written int64
	lastReport := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				finish("failed", "写入失败："+werr.Error())
				return
			}
			written += int64(n)
			if time.Since(lastReport) > 500*time.Millisecond {
				hfMu.Lock()
				job.Done = written
				hfMu.Unlock()
				lastReport = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if ctx.Err() != nil {
				f.Close()
				os.Remove(job.part)
				finish("cancelled", "")
				return
			}
			finish("failed", "读取中断："+rerr.Error())
			return
		}
	}

	f.Close()
	if err := os.Rename(job.part, job.dest); err != nil {
		finish("failed", "重命名失败："+err.Error())
		return
	}
	hfMu.Lock()
	job.Done = written
	if job.Total <= 0 {
		job.Total = written
	}
	hfMu.Unlock()

	// 下完自动登记成模型（用户可关掉）
	msg := ""
	if req.AutoRegist {
		addReq := modelAddReq{
			Name:      strings.TrimSuffix(base, ".gguf"),
			Label:     strings.TrimSuffix(base, ".gguf"),
			Kind:      req.Kind,
			ModelFile: base,
			Port:      req.Port,
		}
		if addReq.Kind == "" {
			addReq.Kind = "moe"
		}
		if err := addModel(addReq); err != nil {
			msg = "下载完成，但自动登记失败：" + err.Error()
		} else {
			msg = "下载完成并已登记为模型"
		}
	}
	finish("done", msg)
}

// sanitizeUnitName 把任意名字（文件名往往带 . 和大写）净化成合法的 unit 名。
func sanitizeUnitName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 30 {
		out = strings.Trim(out[:30], "-")
	}
	return out
}

// addModel 是 handleModelAdd 的可复用核心（供下载完成后自动登记调用）
func addModel(req modelAddReq) error {
	name := sanitizeUnitName(strings.TrimPrefix(strings.TrimSpace(req.Name), "llama-"))
	if name == "" {
		return fmt.Errorf("模型标识不能为空（只能包含字母、数字、连字符）")
	}
	if !unitNameRe.MatchString(name) {
		return fmt.Errorf("模型标识只能用小写字母、数字、连字符（最多 30 位），当前为 %q", name)
	}
	name = "llama-" + name
	if unitDefByName(name) != nil || installedUnit(name) {
		return fmt.Errorf("已存在同名模型：%s", name)
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "moe"
	}
	if !kindValid(kind) {
		return fmt.Errorf("模型类型必须是 moe / dense / embed")
	}
	file := strings.TrimSpace(req.ModelFile)
	if file == "" || strings.Contains(file, "/") || strings.Contains(file, "..") {
		return fmt.Errorf("模型文件名非法")
	}
	if _, err := os.Stat(filepath.Join(modelsDir, file)); err != nil {
		return fmt.Errorf("模型文件不存在：%s", file)
	}
	port := req.Port
	if port == 0 {
		port = nextFreePort()
	}
	if port < 1024 || port > 65535 {
		return fmt.Errorf("端口必须在 1024-65535")
	}
	if owner := portInUse(port, ""); owner != "" {
		return fmt.Errorf("端口 %d 已被 %s 占用", port, owner)
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = strings.TrimSuffix(file, ".gguf")
	}
	u := &UnitDef{
		Name: name, Label: label, Kind: kind, Embed: kind == "embed",
		Model: file, Alias: name, Port: port, UserAdded: true,
		ownDefaults: req.Defaults,
	}
	finalizeUnitDef(u)

	if err := writeEnv(name, u.Defaults); err != nil {
		return fmt.Errorf("写参数文件失败：%v", err)
	}
	if err := os.WriteFile(unitFilePath(name), []byte(unitTemplate(u)), 0o644); err != nil {
		return fmt.Errorf("写 unit 文件失败：%v", err)
	}
	users := loadUserUnits()
	users = append(users, userUnit{
		Name: name, Label: label, Kind: kind, Model: file,
		Alias: name, Port: port, Defaults: req.Defaults,
	})
	if err := saveUserUnits(users); err != nil {
		return fmt.Errorf("写登记表失败：%v", err)
	}
	rebuildUnitDefs()
	if err := rewriteAllConflicts(); err != nil {
		return fmt.Errorf("重写互斥失败：%v", err)
	}
	if err := daemonReload(); err != nil {
		return err
	}
	return nil
}

func handleHFProgress(w http.ResponseWriter, r *http.Request) {
	hfMu.Lock()
	list := make([]*hfJob, 0, len(hfJobs))
	for _, j := range hfJobs {
		cp := *j
		list = append(list, &cp)
	}
	hfMu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Started > list[j].Started })
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "jobs": list})
}

func handleHFCancel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	hfMu.Lock()
	cancel, ok := hfStop[req.ID]
	hfMu.Unlock()
	if !ok {
		fail(w, http.StatusNotFound, "没有正在进行的该下载")
		return
	}
	cancel()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "已请求取消下载"})
}
