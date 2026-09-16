package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

//go:embed schema.json
var schemaJSON []byte

type schema struct {
	Groups []string             `json:"groups"`
	Order  []string             `json:"order"`
	Kinds  map[string][]string  `json:"kinds"`
	Params map[string]ParamSpec `json:"params"`
	Units  []struct {
		Name     string            `json:"name"`
		Label    string            `json:"label"`
		Kind     string            `json:"kind"`
		Embed    bool              `json:"embed"`
		Model    string            `json:"model"`
		Alias    string            `json:"alias"`
		Port     int               `json:"port"`
		Defaults map[string]string `json:"defaults"`
	} `json:"units"`
}

var paramSpecs map[string]ParamSpec
var unitDefs []UnitDef
var groups []string
var order []string
var kinds map[string][]string

func init() {
	var s schema
	if err := json.Unmarshal(schemaJSON, &s); err != nil {
		panic("schema.json 解析失败：" + err.Error())
	}

	paramSpecs = s.Params
	groups = s.Groups
	order = s.Order
	kinds = s.Kinds

	embeddedUnits = make([]UnitDef, 0, len(s.Units))
	rawUnitDefaults = make(map[string]map[string]string, len(s.Units))
	for _, u := range s.Units {
		embeddedUnits = append(embeddedUnits, UnitDef{
			Name:  u.Name,
			Label: u.Label,
			Kind:  u.Kind,
			Embed: u.Embed,
			Model: u.Model,
			Alias: u.Alias,
			Port:  u.Port,
		})
		rawUnitDefaults[u.Name] = u.Defaults
	}

	rebuildUnitDefs()
}

// embeddedUnits 来自 schema.json（内置模型）；unitDefs = embeddedUnits + 用户添加的模型
var embeddedUnits []UnitDef

// rawUnitDefaults 是 schema.json 里逐 unit 的默认参数（如 embed 的 CTX=8192）。
// 旧代码把这份默认值漏掉了，导致新登记模型会拿到参数规格的全局默认值。
var rawUnitDefaults map[string]map[string]string

type ParamSpec struct {
	Key     string   `json:"-"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // int | float | enum | bool | str | model
	Core    bool     `json:"core"`
	Scope   string   `json:"scope"`
	Flag    string   `json:"flag"`
	OffFlag string   `json:"off_flag"`
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
	Default string   `json:"default"`
	Safe    string   `json:"safe"`
	Note    string   `json:"note"`
	Options []string `json:"options"`
}

type UnitDef struct {
	Name      string
	Label     string
	Kind      string // moe | dense | embed | custom
	Embed     bool
	Model     string
	Alias     string
	Port      int
	Keys      []string
	Defaults  map[string]string
	UserAdded bool // 面板里添加的模型，可以整个删掉

	ownDefaults map[string]string // 用户登记时显式指定的默认值
}

func unitDefByName(name string) *UnitDef {
	for i := range unitDefs {
		if unitDefs[i].Name == name {
			return &unitDefs[i]
		}
	}
	return nil
}

func defaultParams(u *UnitDef) map[string]string {
	if u == nil {
		return make(map[string]string)
	}
	result := make(map[string]string)
	for k, v := range u.Defaults {
		result[k] = v
	}
	return result
}

func validateParams(u *UnitDef, p map[string]string) error {
	if u == nil {
		return fmt.Errorf("未知的模型单元")
	}

	for k := range p {
		if k == "OPT" {
			// OPT 是由其它参数拼出来的派生字段，调用方回传时忽略即可，不该报错
			continue
		}
		if !stringInSlice(k, u.Keys) {
			return fmt.Errorf("不支持的参数：%s", k)
		}
	}

	for _, k := range u.Keys {
		if _, exists := p[k]; !exists {
			// 只有核心参数必须齐全（它们会变成 ExecStart 引用的环境变量）。
			// 非核心参数缺失是正常的 —— 只是不拼进 OPT 而已。
			if spec, ok := paramSpecs[k]; ok && spec.Core {
				return fmt.Errorf("缺少参数：%s", k)
			}
		}
	}

	for k, v := range p {
		spec, exists := paramSpecs[k]
		if !exists {
			continue
		}

		switch spec.Type {
		case "int":
			if v == "" {
				continue
			}
			i, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s 必须是整数", spec.Label)
			}
			if float64(i) < spec.Min || float64(i) > spec.Max {
				return fmt.Errorf("%s 超出范围（%g-%g）", spec.Label, spec.Min, spec.Max)
			}
		case "float":
			if v == "" {
				continue
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("%s 必须是数字", spec.Label)
			}
			if f < spec.Min || f > spec.Max {
				return fmt.Errorf("%s 超出范围（%g-%g）", spec.Label, spec.Min, spec.Max)
			}
		case "enum", "bool":
			if v == "" {
				continue
			}
			if !stringInSlice(v, spec.Options) {
				return fmt.Errorf("%s 取值非法：%s", spec.Label, v)
			}
		}
	}

	if v, exists := p["PORT"]; exists && v != "" {
		port, _ := strconv.Atoi(v)
		for _, other := range unitDefs {
			if other.Name != u.Name && other.Port == port {
				return fmt.Errorf("端口 %s 已被 %s 占用", v, other.Name)
			}
		}
	}

	if v, exists := p["MODEL"]; exists && v != "" {
		if !strings.HasPrefix(v, "/home/user/models/") {
			return fmt.Errorf("模型路径必须在 /home/user/models/ 下")
		}
	}

	return nil
}

func envFileName(u *UnitDef) string {
	if u == nil {
		return ""
	}
	return strings.TrimPrefix(u.Name, "llama-")
}

func paramSpecByKey(k string) (ParamSpec, bool) {
	spec, exists := paramSpecs[k]
	return spec, exists
}

func groupsOrder() []string {
	result := make([]string, len(groups))
	copy(result, groups)
	return result
}

func paramOrder() []string {
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, k := range order {
		if _, exists := paramSpecs[k]; exists {
			result = append(result, k)
			seen[k] = true
		}
	}
	if !seen["EXTRA"] {
		result = append(result, "EXTRA")
	}
	return result
}

func appliesTo(kind, scope string) bool {
	if kindList, exists := kinds[kind]; exists {
		for _, s := range kindList {
			if s == scope {
				return true
			}
		}
	}
	return scope == "all"
}

func keysForKind(kind string) []string {
	order := paramOrder()
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, k := range order {
		if spec, exists := paramSpecs[k]; exists {
			if appliesTo(kind, spec.Scope) {
				result = append(result, k)
				seen[k] = true
			}
		}
	}
	if !seen["EXTRA"] {
		result = append(result, "EXTRA")
	}
	return result
}

func defaultOptFor(kind string) string {
	if kind == "embed" {
		return ""
	}
	return "--load-mode none --jinja"
}

func stringInSlice(s string, slice []string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
