package main

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
)

// 面板自己维护的东西
func userUnitsPath() string { return stateDir() + "/" + userUnitsFile }

const (
	userUnitsFile = "units.json"          // 用户添加的模型登记表（实际路径见 userUnitsPath()）
	unitDir       = "/etc/systemd/system" // systemd unit 目录
	envDir        = "/etc/llama.d"        // 参数环境文件目录
)

// userUnit 是「添加模型」持久化的结构。unit 文件可以从这里随时重建。
type userUnit struct {
	Name     string            `json:"name"`
	Label    string            `json:"label"`
	Kind     string            `json:"kind"`
	Model    string            `json:"model"`
	Alias    string            `json:"alias"`
	Port     int               `json:"port"`
	Defaults map[string]string `json:"defaults,omitempty"`
}

func loadUserUnits() []userUnit {
	data, err := os.ReadFile(userUnitsPath())
	if err != nil {
		return nil
	}
	var doc struct {
		Units []userUnit `json:"units"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	return doc.Units
}

func saveUserUnits(units []userUnit) error {
	if units == nil {
		units = []userUnit{}
	}
	doc := struct {
		Units []userUnit `json:"units"`
	}{Units: units}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(userUnitsPath(), append(data, '\n'), 0o644)
}

// rebuildUnitDefs 把 schema.json 的内置模型与用户添加的模型合并成 unitDefs。
// 任何增删模型之后都要调它，否则改动不会体现到状态页和调试页。
func rebuildUnitDefs() {
	user := loadUserUnits()
	defs := make([]UnitDef, 0, len(embeddedUnits)+len(user))
	defs = append(defs, embeddedUnits...)
	for _, uu := range user {
		defs = append(defs, UnitDef{
			Name:        uu.Name,
			Label:       uu.Label,
			Kind:        uu.Kind,
			Embed:       uu.Kind == "embed",
			Model:       uu.Model,
			Alias:       uu.Alias,
			Port:        uu.Port,
			UserAdded:   true,
			ownDefaults: uu.Defaults,
		})
	}
	for i := range defs {
		finalizeUnitDef(&defs[i])
	}
	unitDefs = defs
}

// finalizeUnitDef 按「参数规格默认值 → 内置逐 unit 默认值 → 用户显式默认值 → 身份字段」
// 的顺序算出这个 unit 的完整默认参数。顺序不能乱：后者覆盖前者。
func finalizeUnitDef(u *UnitDef) {
	u.Keys = keysForKind(u.Kind)
	d := make(map[string]string)

	// 规格里的 default 对「所有」参数生效 —— 不只是 core。
	// 之前只管 core，导致 LOADMODE/JINJA 这类非核心参数的 default 形同虚设，
	// 只能靠按 kind 硬编码的默认串来补，于是出现"默认值与保存值不一致"。
	for _, k := range u.Keys {
		if spec, ok := paramSpecs[k]; ok && spec.Default != "" {
			d[k] = spec.Default
		}
	}
	for k, v := range rawUnitDefaults[u.Name] {
		if stringInSlice(k, u.Keys) {
			d[k] = v
		}
	}
	for k, v := range u.ownDefaults {
		if stringInSlice(k, u.Keys) {
			d[k] = v
		}
	}

	// MODEL 始终存在（即便是空串）—— llama-custom 的模型是加载时才指定的，
	// 字段缺失会让核心参数校验误判为"缺少参数"。
	d["MODEL"] = ""
	if u.Model != "" {
		if strings.HasPrefix(u.Model, "/") {
			d["MODEL"] = u.Model
		} else {
			d["MODEL"] = modelsDir + "/" + u.Model
		}
	}
	d["PORT"] = strconv.Itoa(u.Port)
	d["ALIAS"] = u.Alias
	d["EXTRA"] = ""
	// OPT 由参数派生（与保存路径同一套逻辑），不再按 kind 写死
	d["OPT"] = composeOpt(u, d)
	u.Defaults = d
}

// chatUnits 返回需要参与互斥的 unit。
// embed 与 rerank 体积小、可与对话模型共存，不参与互斥。
func chatUnits() []string {
	names := make([]string, 0, len(unitDefs))
	for i := range unitDefs {
		if unitDefs[i].Kind != "embed" && unitDefs[i].Kind != "rerank" {
			names = append(names, unitDefs[i].Name)
		}
	}
	sort.Strings(names)
	return names
}

func unitFilePath(name string) string {
	return unitDir + "/" + name + ".service"
}

// installedUnit 判断 unit 文件是否真的在磁盘上（登记表里有、但文件被删掉的情况要能看出来）
func installedUnit(name string) bool {
	_, err := os.Stat(unitFilePath(name))
	return err == nil
}

func portInUse(port int, except string) string {
	for i := range unitDefs {
		if unitDefs[i].Name != except && unitDefs[i].Port == port {
			return unitDefs[i].Name
		}
	}
	return ""
}

func nextFreePort() int {
	for p := 1241; p < 1400; p++ {
		if portInUse(p, "") == "" {
			return p
		}
	}
	return 1241
}

func kindValid(k string) bool {
	return k == "moe" || k == "dense" || k == "embed" || k == "rerank" || k == "vision"
}
