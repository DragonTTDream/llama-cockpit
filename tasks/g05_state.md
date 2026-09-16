file: src/g05_state.go
model: coder

# 任务 g05：状态快照组装（/api/state 的数据源）

产出 `src/g05_state.go`，`package main`，只含下面列出的声明，**不得定义 main()**。
只用标准库（`fmt` `math` `net` `net/http` `time`）。
可调用前面已定义的一切：`unitDefs` `UnitDef` `defaultParams` `readEnv` `unitActive`
`unitStateText` `unitEnabled` `gpuInfo` `GPUInfo` `vramByPID` `unitVramFrom` `openclawInfo` `OpenClawView`。

## 必须有的声明

```go
const panelPort = 8077

var panelStart = time.Now()

type GPUView struct {
	Name        string `json:"name"`
	MemUsedMiB  int    `json:"mem_used_mib"`
	MemTotalMiB int    `json:"mem_total_mib"`
	UtilPct     int    `json:"util_pct"`
	TempC       int    `json:"temp_c"`
}

type PanelView struct {
	Port      int  `json:"port"`
	Autostart bool `json:"autostart"`
	UptimeS   int  `json:"uptime_s"`
}

type UnitStateView struct {
	Name       string            `json:"name"`
	Label      string            `json:"label"`
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
	Editable   bool              `json:"editable"`
	Coexist    bool              `json:"coexist"`
}

type StateResp struct {
	OK       bool            `json:"ok"`
	Time     string          `json:"time"`
	GPU      GPUView         `json:"gpu"`
	OpenClaw OpenClawView    `json:"openclaw"`
	Panel    PanelView       `json:"panel"`
	Units    []UnitStateView `json:"units"`
}

func probeHealth(port int) string
func buildState() StateResp
```

## 行为

- `probeHealth(port int) string`：
  `GET http://127.0.0.1:<port>/health`，客户端超时 `1500 * time.Millisecond`。
  2xx → `"ok"`；非 2xx → `"down"`；`err` 是超时（用 `net.Error` 的 `Timeout()` 判断）→ `"timeout"`；
  其它错误 → `"down"`。**必须 `defer resp.Body.Close()`，且不要读取 body**。
- `buildState() StateResp`：
  1. `g := gpuInfo()`；`byPID := vramByPID()`（**只调一次**，6 个 unit 复用）
  2. 遍历 `unitDefs`，每个 unit 生成一个 `UnitStateView`：
     - `Params`：先 `defaultParams(u)` 得到默认值，再用 `readEnv(u.Name)` 的结果**覆盖同键**；
       `readEnv` 报错时只用默认值（不中断）
     - `Active = unitActive(u.Name)`；`Enabled = unitEnabled(u.Name)`；`StateLabel = unitStateText(u.Name)`；
       `State` = `unitStateText` 的原始英文来源：用 `unitActive` 判断，`active` → `"active"`，否则 `"inactive"`
     - `VramMiB = unitVramFrom(u.Name, byPID)`
     - `VramPct`：`g.MemTotalMiB <= 0` 时为 `0`，否则
       `math.Round(float64(VramMiB)/float64(g.MemTotalMiB)*1000)/10`（保留 1 位小数）
     - `Health`：`Active` 为真时 `probeHealth(u.Port)`，否则 `"down"`
     - `Editable = true`；`Coexist = u.Embed`；`Name/Label/Port/Model` 取自 `u`（`Port` 用 `u.Port`）
  3. `GPU` 字段由 `g` 直接映射（字段名对应：`Name/MemUsedMiB/MemTotalMiB/UtilPct/TempC`）
  4. `OpenClaw = openclawInfo()`
  5. `Panel`：`Port = panelPort`、`Autostart = unitEnabled("llama-panel.service")`、
     `UptimeS = int(time.Since(panelStart).Seconds())`
  6. `OK = true`；`Time = time.Now().Format(time.RFC3339)`
  7. `Units` 长度必然等于 `len(unitDefs)`，顺序与 `unitDefs` 一致

## 硬要求

- 不要 import 未使用的包；不要 panic；不要定义 `main()`
- 不要重复定义 g01~g04 已有的名字（尤其 `OpenClawView`、`GPUInfo`）
- `gofmt` 干净、`go vet` 无输出
