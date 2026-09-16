package main

import (
	"os/exec"
	"strconv"
	"strings"
)

type GPUInfo struct {
	Name        string
	MemUsedMiB  int
	MemTotalMiB int
	UtilPct     int
	TempC       int
}

func gpuInfo() GPUInfo {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.used,memory.total,utilization.gpu,temperature.gpu", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return GPUInfo{Name: "unknown"}
	}

	line := strings.TrimSpace(string(output))
	parts := strings.Split(line, ",")
	if len(parts) < 5 {
		return GPUInfo{Name: "unknown"}
	}

	var info GPUInfo
	info.Name = strings.TrimSpace(parts[0])
	if i, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
		info.MemUsedMiB = i
	}
	if i, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil {
		info.MemTotalMiB = i
	}
	if i, err := strconv.Atoi(strings.TrimSpace(parts[3])); err == nil {
		info.UtilPct = i
	}
	if i, err := strconv.Atoi(strings.TrimSpace(parts[4])); err == nil {
		info.TempC = i
	}

	return info
}

func vramByPID() map[int]int {
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,used_memory", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return map[int]int{}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	result := make(map[int]int)
	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		pidStr := strings.TrimSpace(parts[0])
		memStr := strings.TrimSpace(parts[1])
		pid, err1 := strconv.Atoi(pidStr)
		mem, err2 := strconv.Atoi(memStr)
		if err1 != nil || err2 != nil {
			continue
		}
		result[pid] = mem
	}
	return result
}

func mainPID(unit string) int {
	cmd := exec.Command("systemctl", "show", "-p", "MainPID", "--value", unit)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	pidStr := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func unitVramFrom(unit string, byPID map[int]int) int {
	pid := mainPID(unit)
	if pid <= 0 {
		return 0
	}
	return byPID[pid]
}

func unitVram(unit string) int {
	return unitVramFrom(unit, vramByPID())
}

// GPUProc 是 GPU 上的一个计算进程。面板要能看见「不归任何 unit 管」的裸进程
// ——比如调试时手工起的 llama-server，否则它们占着显存却不出现在任何地方。
type GPUProc struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	UsedMiB int    `json:"used_mib"`
}

func gpuProcs() []GPUProc {
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,process_name,used_memory",
		"--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var res []GPUProc
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		pid, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		mem, e2 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if e1 != nil || e2 != nil {
			continue
		}
		res = append(res, GPUProc{PID: pid, Name: strings.TrimSpace(parts[1]), UsedMiB: mem})
	}
	return res
}
