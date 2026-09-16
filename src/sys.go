package main

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CPUView CPU 使用情况（利用率来自两次采样的差值，首次调用返回 0）
type CPUView struct {
	UtilPct float64   `json:"util_pct"`
	Cores   []float64 `json:"cores"`
	Count   int       `json:"count"`
	Load1   float64   `json:"load1"`
	Load5   float64   `json:"load5"`
	Load15  float64   `json:"load15"`
}

// MemView 物理内存与交换分区（单位 MiB）
type MemView struct {
	TotalMiB     int `json:"total_mib"`
	UsedMiB      int `json:"used_mib"`
	AvailMiB     int `json:"avail_mib"`
	SwapTotalMiB int `json:"swap_total_mib"`
	SwapUsedMiB  int `json:"swap_used_mib"`
}

var (
	cpuMu   sync.Mutex
	cpuPrev []cpuSample
)

type cpuSample struct {
	total float64
	idle  float64
}

// readProcStat 读 /proc/stat 的 cpu 行，返回 [总时间, 空闲时间] 序列（第 0 个是汇总）
func readProcStat() ([]cpuSample, bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var out []cpuSample
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		var vals []float64
		for _, x := range fields[1:] {
			v, err := strconv.ParseFloat(x, 64)
			if err != nil {
				v = 0
			}
			vals = append(vals, v)
		}
		var total float64
		for _, v := range vals {
			total += v
		}
		idle := vals[3] // idle
		if len(vals) > 4 {
			idle += vals[4] // iowait
		}
		out = append(out, cpuSample{total: total, idle: idle})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func pct(prev, cur cpuSample) float64 {
	dt := cur.total - prev.total
	di := cur.idle - prev.idle
	if dt <= 0 {
		return 0
	}
	p := (1 - di/dt) * 100
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return math.Round(p*10) / 10
}

func cpuInfo() CPUView {
	cur, ok := readProcStat()
	if !ok {
		return CPUView{}
	}
	v := CPUView{Count: len(cur) - 1}
	if v.Count < 0 {
		v.Count = 0
	}

	cpuMu.Lock()
	prev := cpuPrev
	cpuPrev = cur
	cpuMu.Unlock()

	if prev != nil && len(prev) == len(cur) {
		v.UtilPct = pct(prev[0], cur[0])
		for i := 1; i < len(cur); i++ {
			v.Cores = append(v.Cores, pct(prev[i], cur[i]))
		}
	} else {
		v.Cores = make([]float64, v.Count)
		time.Sleep(120 * time.Millisecond)
		again, ok2 := readProcStat()
		if ok2 && len(again) == len(cur) {
			v.UtilPct = pct(cur[0], again[0])
			for i := 1; i < len(again); i++ {
				v.Cores = append(v.Cores, pct(cur[i], again[i]))
			}
			cpuMu.Lock()
			cpuPrev = again
			cpuMu.Unlock()
		}
	}

	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(b))
		if len(parts) >= 3 {
			if x, err := strconv.ParseFloat(parts[0], 64); err == nil {
				v.Load1 = x
			}
			if x, err := strconv.ParseFloat(parts[1], 64); err == nil {
				v.Load5 = x
			}
			if x, err := strconv.ParseFloat(parts[2], 64); err == nil {
				v.Load15 = x
			}
		}
	}
	return v
}

func memInfo() MemView {
	var v MemView
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return v
	}
	kv := map[string]int{}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		if n, err := strconv.Atoi(parts[1]); err == nil {
			kv[key] = n
		}
	}
	toMiB := func(kb int) int { return kb / 1024 }
	v.TotalMiB = toMiB(kv["MemTotal"])
	v.AvailMiB = toMiB(kv["MemAvailable"])
	if v.AvailMiB == 0 {
		v.AvailMiB = toMiB(kv["MemFree"])
	}
	v.UsedMiB = v.TotalMiB - v.AvailMiB
	if v.UsedMiB < 0 {
		v.UsedMiB = 0
	}
	v.SwapTotalMiB = toMiB(kv["SwapTotal"])
	swapFree := toMiB(kv["SwapFree"])
	v.SwapUsedMiB = v.SwapTotalMiB - swapFree
	if v.SwapUsedMiB < 0 {
		v.SwapUsedMiB = 0
	}
	return v
}
