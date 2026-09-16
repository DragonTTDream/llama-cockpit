package main

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"time"
)

const panelPort = 8077

var panelStart = time.Now()

type GPUView struct {
	Name        string `json:"name"`
	MemUsedMiB  int    `json:"mem_used_mib"`
	MemTotalMiB int    `json:"mem_total_mib"`
	UtilPct     int    `json:"util_pct"`
	TempC       int    `json:"temp_c"`
	// 不属于任何已知 unit 的 GPU 进程（手工起的 llama-server、测试进程等）
	OtherProcs []GPUProc `json:"other_procs"`
}

type PanelView struct {
	Port         int  `json:"port"`
	Autostart    bool `json:"autostart"`
	AuthRequired bool `json:"auth_required"`
	UptimeS      int  `json:"uptime_s"`
}

type UnitStateView struct {
	Name       string            `json:"name"`
	Label      string            `json:"label"`
	Kind       string            `json:"kind"`
	Port       int               `json:"port"`
	Model      string            `json:"model"`
	Active     bool              `json:"active"`
	State      string            `json:"state"`
	StateLabel string            `json:"state_label"`
	Enabled    bool              `json:"enabled"`
	VramMiB    int               `json:"vram_mib"`
	VramPct    float64           `json:"vram_pct"`
	Health     string            `json:"health"`
	Params     map[string]string `json:"params"`
	Resolved   map[string]string `json:"resolved"`
	Editable   bool              `json:"editable"`
	Coexist    bool              `json:"coexist"`
	Installed  bool              `json:"installed"`
	UserAdded  bool              `json:"user_added"`
}

type StateResp struct {
	OK       bool            `json:"ok"`
	Time     string          `json:"time"`
	CPU      CPUView         `json:"cpu"`
	Mem      MemView         `json:"mem"`
	GPU      GPUView         `json:"gpu"`
	OpenClaw OpenClawView    `json:"openclaw"`
	Panel    PanelView       `json:"panel"`
	Units    []UnitStateView `json:"units"`
}

func probeHealth(port int) string {
	client := http.Client{
		Timeout: 1500 * time.Millisecond,
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return "timeout"
		}
		return "down"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "ok"
	}
	return "down"
}

func buildState() StateResp {
	g := gpuInfo()
	byPID := vramByPID()
	resp := StateResp{
		OK:       true,
		Time:     time.Now().Format(time.RFC3339),
		CPU:      cpuInfo(),
		Mem:      memInfo(),
		GPU:      GPUView{Name: g.Name, MemUsedMiB: g.MemUsedMiB, MemTotalMiB: g.MemTotalMiB, UtilPct: g.UtilPct, TempC: g.TempC},
		OpenClaw: openclawInfo(),
		Panel:    PanelView{Port: panelPort, Autostart: unitEnabled("llama-panel.service"), AuthRequired: authRequired(), UptimeS: int(time.Since(panelStart).Seconds())},
		Units:    make([]UnitStateView, len(unitDefs)),
	}

	// 把 GPU 计算进程按「是否归属某个 unit」分类；不归属的单独列出来
	known := map[int]bool{os.Getpid(): true}
	for _, u := range unitDefs {
		if p := mainPID(u.Name); p > 0 {
			known[p] = true
		} else if p := mainPID(u.Name + ".service"); p > 0 {
			known[p] = true
		}
	}
	for _, pr := range gpuProcs() {
		if !known[pr.PID] {
			resp.GPU.OtherProcs = append(resp.GPU.OtherProcs, pr)
		}
	}
	for i, u := range unitDefs {
		params := defaultParams(&u)
		env, err := readEnv(u.Name)
		if err == nil {
			for k, v := range env {
				params[k] = v
			}
		}
		active := unitActive(u.Name)
		enabled := unitEnabled(u.Name)
		stateLabel := unitStateText(u.Name)
		state := "inactive"
		if active {
			state = "active"
		}
		vramMiB := unitVramFrom(u.Name, byPID)
		vramPct := 0.0
		if g.MemTotalMiB > 0 {
			vramPct = math.Round(float64(vramMiB)/float64(g.MemTotalMiB)*1000) / 10
		}
		health := "down"
		if active {
			health = probeHealth(u.Port)
		}
		resp.Units[i] = UnitStateView{
			Name:       u.Name,
			Label:      u.Label,
			Port:       u.Port,
			Model:      u.Model,
			Active:     active,
			State:      state,
			StateLabel: stateLabel,
			Enabled:    enabled,
			VramMiB:    vramMiB,
			VramPct:    vramPct,
			Health:     health,
			Params:     params,
			Resolved:   resolveValues(&u, params),
			Kind:       u.Kind,
			Editable:   true,
			Coexist:    u.Embed,
			Installed:  installedUnit(u.Name),
			UserAdded:  u.UserAdded,
		}
	}
	return resp
}
