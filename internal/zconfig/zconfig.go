// Package zconfig 读取 ZCode 客户端配置，为 GUI 提供下拉选项：
// provider/模型（config.json）、已装技能、内置智能体覆盖（agents-state.json）。
// 注意：只读名称与模型清单，绝不读取或外传任何 API Key。
package zconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EnvHome 覆盖 ~/.zcode（测试用）。
const EnvHome = "ZCODE_HOME"

// ModeOption 是权限模式下拉项。
type ModeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Desc  string `json:"desc"`
	// Internal 标注该取值在客户端内部被归一到哪一档。
	Internal string `json:"internal"`
}

// ModelOption 是一个可选的模型引用（provider/model）。
type ModelOption struct {
	Ref          string `json:"ref"`
	ProviderName string `json:"providerName"`
	Model        string `json:"model"`
}

// ToolGroup 是工具复选框分组。
type ToolGroup struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools"`
}

// BuiltInAgent 是内置智能体（无文件，只读展示）。
type BuiltInAgent struct {
	Name  string `json:"name"`
	Model string `json:"model,omitempty"`
	Desc  string `json:"desc"`
}

// Meta 是前端表单需要的全部静态选项。
type Meta struct {
	Modes          []ModeOption    `json:"modes"`
	Colors         []string        `json:"colors"`
	ThoughtLevels  []string        `json:"thoughtLevels"`
	ToolGroups     []ToolGroup     `json:"toolGroups"`
	Skills         []string        `json:"skills"`
	Models         []ModelOption   `json:"models"`
	BuiltIns       []BuiltInAgent  `json:"builtIns"`
	AgentsDir      string          `json:"agentsDir"`
	ZCodeHome      string          `json:"zcodeHome"`
	ConfigFound    bool            `json:"configFound"`
}

// ModeOptions 复刻客户端模式选择器的文案（中文）。
func ModeOptions() []ModeOption {
	return []ModeOption{
		{"yolo", "完全访问", "不弹任何确认，编辑与命令直接执行", "yolo"},
		{"bypassPermissions", "绕过权限", "跳过权限检查（与 yolo 同档）", "yolo"},
		{"dontAsk", "无需询问", "跳过常规确认（归 yolo 档）", "yolo"},
		{"edit", "自动编辑", "自动应用文件编辑，其他高风险操作仍确认", "edit"},
		{"autoEdit", "自动编辑", "自动编辑（客户端归 build 档）", "build"},
		{"acceptEdits", "接受编辑", "自动接受文件编辑——注意客户端归 build 档，仍会逐次确认", "build"},
		{"default", "默认", "编辑和高风险操作前询问（客户端 build 档）", "build"},
		{"build", "变更前确认", "改文件前先问我", "build"},
		{"plan", "计划", "先计划，确认后执行", "plan"},
		{"auto", "自动", "自动选择权限模式", "auto"},
	}
}

// Colors 与 agentfile.Colors 保持一致（这里单独一份避免循环依赖）。
var Colors = []string{"red", "blue", "green", "yellow", "purple", "orange", "pink", "cyan"}

// DefaultThoughtLevels 是无法解析模型时的候选档位。
var DefaultThoughtLevels = []string{"off", "none", "low", "medium", "high", "xhigh", "max", "enabled", "adaptive"}

func home() string {
	if h := os.Getenv(EnvHome); h != "" {
		return h
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".zcode")
	}
	return filepath.Join(u, ".zcode")
}

func v2() string { return filepath.Join(home(), "v2") }

// ToolGroups 是工具清单（名称与客户端内置工具对齐）。
func ToolGroups() []ToolGroup {
	return []ToolGroup{
		{"文件", []string{"Read", "Write", "Edit", "ApplyPatch", "NotebookEdit"}},
		{"搜索", []string{"Glob", "Grep", "WebFetch", "WebSearch", "web_search"}},
		{"命令", []string{"Bash", "BashOutput", "KillShell"}},
		{"计划", []string{"TodoWrite", "TodoRead", "GoalRead", "EnterPlanMode", "ExitPlanMode"}},
		{"协作", []string{"Agent", "Task", "TaskOutput", "TaskStop", "SendMessage", "RespondToCoordinator", "ReadSessionContext", "AskUserQuestion"}},
		{"节点", []string{"js", "js_reset", "js_add_node_module_dir", "mcp__node_repl__js"}},
		{"其他", []string{"Skill"}},
	}
}

// KnownTools 返回全部内置工具名（用于校验提示）。
func KnownTools() []string {
	var out []string
	for _, g := range ToolGroups() {
		out = append(out, g.Tools...)
	}
	return out
}

// LoadMeta 汇总前端需要的选项。
func LoadMeta(agentsDir string) *Meta {
	m := &Meta{
		Modes:         ModeOptions(),
		Colors:        Colors,
		ThoughtLevels: DefaultThoughtLevels,
		ToolGroups:    ToolGroups(),
		Skills:        loadSkills(),
		Models:        loadModels(),
		BuiltIns: []BuiltInAgent{
			{"general-purpose", "", "全工具通用智能体"},
			{"Explore", "", "只读搜索智能体"},
		},
		AgentsDir:   agentsDir,
		ZCodeHome:   home(),
		ConfigFound: false,
	}
	applyBuiltInOverrides(m)
	return m
}

// ---------- providers / models ----------

type providerFile struct {
	Provider map[string]providerEntry `json:"provider"`
}

type providerEntry struct {
	Name   string                     `json:"name"`
	Models map[string]modelEntry      `json:"models"`
}

type modelEntry struct {
	Reasoning *struct {
		Enabled  bool     `json:"enabled"`
		Variants []string `json:"variants"`
	} `json:"reasoning"`
}

// loadModels 汇总可选模型引用。主源是 provider_config.json 的 providerRules
// （客户端现行的 providerId/modelId 解析源，含 new-provider-3 这类短 id），
// 辅源是 config.json 的 provider 字典（UUID 形式引用，旧角色文件在用）。
// 只读取 id/名称/模型名，不触碰任何凭证字段。
func loadModels() []ModelOption {
	var out []ModelOption
	seen := map[string]bool{}
	add := func(ref, providerName, model string) {
		if ref == "" || model == "" || seen[ref] {
			return
		}
		seen[ref] = true
		out = append(out, ModelOption{Ref: ref, ProviderName: providerName, Model: model})
	}

	// 主源：provider_config.json
	if data, err := os.ReadFile(filepath.Join(v2(), "provider_config.json")); err == nil {
		var doc struct {
			Config struct {
				ProviderConfigRules struct {
					ProviderRules []struct {
						ProviderID   string `json:"providerId"`
						ProviderName string `json:"providerName"`
						Enabled      *bool  `json:"enabled"`
						Config       struct {
							PersonalModelIDs []string `json:"personalModelIds"`
							ModelOrder       []string `json:"modelOrder"`
						} `json:"config"`
					} `json:"providerRules"`
				} `json:"providerConfigRules"`
			} `json:"config"`
		}
		if json.Unmarshal(data, &doc) == nil {
			for _, r := range doc.Config.ProviderConfigRules.ProviderRules {
				if r.Enabled != nil && !*r.Enabled {
					continue // 已停用的 provider 不进下拉
				}
				name := r.ProviderName
				if name == "" {
					name = r.ProviderID
				}
				for _, m := range append(append([]string{}, r.Config.PersonalModelIDs...), r.Config.ModelOrder...) {
					add(r.ProviderID+"/"+m, name, m)
				}
			}
		}
	}

	// 辅源：config.json
	if data, err := os.ReadFile(filepath.Join(v2(), "config.json")); err == nil {
		var pf providerFile
		if json.Unmarshal(data, &pf) == nil {
			for id, p := range pf.Provider {
				pname := p.Name
				if pname == "" {
					pname = id
				}
				for model := range p.Models {
					add(id+"/"+model, pname, model)
					add(pname+"/"+model, pname, model)
				}
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

// ThoughtLevelsFor 按模型引用解析推理档位候选；找不到返回 nil（前端用通用候选）。
func ThoughtLevelsFor(ref string) []string {
	provider, model, ok := strings.Cut(ref, "/")
	if !ok || model == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(v2(), "config.json"))
	if err != nil {
		return nil
	}
	var pf providerFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil
	}
	for id, p := range pf.Provider {
		if id != provider && p.Name != provider {
			continue
		}
		if me, ok := p.Models[model]; ok && me.Reasoning != nil && me.Reasoning.Enabled {
			return me.Reasoning.Variants
		}
	}
	return nil
}

// ---------- skills ----------

func loadSkills() []string {
	candidates := []string{
		filepath.Join(home(), "..", ".agents", "skills"),
		filepath.Join(home(), "skills"),
		filepath.Join(home(), "..", ".claude", "skills"),
	}
	seen := map[string]bool{}
	var out []string
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || seen[e.Name()] {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err != nil {
				continue
			}
			seen[e.Name()] = true
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// ---------- agents-state.json（只读展示内置覆盖） ----------

type agentsState struct {
	DisabledAgentIDs []string `json:"disabledAgentIds"`
	BuiltInModelOverrides map[string]string `json:"builtInModelOverrides"`
	BuiltInModelSelectionOverrides map[string]struct {
		ProviderID string `json:"providerId"`
		ModelID    string `json:"modelId"`
		Options    struct {
			ReasoningLevel string `json:"reasoningLevel"`
		} `json:"options"`
	} `json:"builtInModelSelectionOverrides"`
}

func applyBuiltInOverrides(m *Meta) {
	data, err := os.ReadFile(filepath.Join(v2(), "agents-state.json"))
	if err != nil {
		return
	}
	var st agentsState
	if err := json.Unmarshal(data, &st); err != nil {
		return
	}
	disabled := map[string]bool{}
	for _, id := range st.DisabledAgentIDs {
		disabled[id] = true
	}
	for i := range m.BuiltIns {
		name := m.BuiltIns[i].Name
		if ov, ok := st.BuiltInModelOverrides[name]; ok {
			m.BuiltIns[i].Model = ov
		}
		if ov, ok := st.BuiltInModelSelectionOverrides[name]; ok {
			m.BuiltIns[i].Model = ov.ProviderID + "/" + ov.ModelID
		}
		if disabled[name] || disabled["built-in:"+name] {
			m.BuiltIns[i].Desc += "（已停用）"
		}
	}
}
