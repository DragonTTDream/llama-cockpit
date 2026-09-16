file: src/g02_gpu.go
model: coder

# 任务 g02：GPU 与显存归属

产出 `src/g02_gpu.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只用标准库（`os/exec` `strconv` `strings`）。

## 必须有的声明

```go
type GPUInfo struct {
	Name        string
	MemUsedMiB  int
	MemTotalMiB int
	UtilPct     int
	TempC       int
}

func gpuInfo() GPUInfo
func vramByPID() map[int]int
func mainPID(unit string) int
func unitVramFrom(unit string, byPID map[int]int) int
func unitVram(unit string) int
```

## 行为

- `gpuInfo()`：执行
  `nvidia-smi --query-gpu=name,memory.used,memory.total,utilization.gpu,temperature.gpu --format=csv,noheader,nounits`
  取**第一行**，按 `,` 切分，每段 `strings.TrimSpace`。字段依次是 name / mem.used / mem.total / util / temp。
  任何一步失败（命令报错、行数不足、数字解析失败）都返回 `GPUInfo{Name: "unknown"}`（其余字段为 0），**不要 panic**。
- `vramByPID()`：执行
  `nvidia-smi --query-compute-apps=pid,used_memory --format=csv,noheader,nounits`
  每行 `pid, 显存MiB`。返回 `map[pid]显存MiB`。命令失败返回**空 map（非 nil）**。
  第一列解析失败的行**跳过**，不要中断。
- `mainPID(unit string) int`：执行 `systemctl show -p MainPID --value <unit>`，
  输出形如 `42857`。解析失败或为 `0` 返回 0。
- `unitVramFrom(unit string, byPID map[int]int) int`：取 `mainPID(unit)`，若 `<= 0` 返回 0，
  否则查 `byPID`，查不到返回 0。
- `unitVram(unit string) int`：`return unitVramFrom(unit, vramByPID())`。

## 硬要求

- 用 `exec.Command(...).Output()`；出错时不要打印、不要 panic、不要 `os.Exit`
- 所有 `nvidia-smi` / `systemctl` 调用的参数必须拼成独立的 `[]string`，不要走 `sh -c`
- 不要定义 `main()`
- `gofmt` 必须干净，`go vet` 必须无输出
