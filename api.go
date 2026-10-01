package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"zcode-subagent-control/internal/agentfile"
	"zcode-subagent-control/internal/zconfig"
)

// API 是暴露给前端的唯一服务：ZCode 自定义子智能体（~/.zcode/agents/*.md）的
// 查看、编辑、新建、删除与权限模式切换。所有写操作都会先自动备份。
// mainWin/floatWin 由 main 直接注入（不走绑定，避免接口参数被生成成 TS）。
type API struct {
	agentsDir string
	mainWin   application.Window
	floatWin  application.Window
}

func (a *API) ServiceName() string { return "api" }

// NewAPI 用默认角色目录构造服务。
func NewAPI() *API {
	return &API{agentsDir: agentfile.DefaultDir()}
}

// AgentsDir 返回当前角色目录。
func (a *API) AgentsDir() string { return a.agentsDir }

// ListAgents 返回全部角色摘要（按文件名排序）。
func (a *API) ListAgents() ([]agentfile.Summary, error) {
	return agentfile.List(a.agentsDir)
}

// GetAgent 读取单个角色的完整定义。
func (a *API) GetAgent(fileName string) (*agentfile.Agent, error) {
	if !safeName(fileName) {
		return nil, fmt.Errorf("非法角色名: %q", fileName)
	}
	return agentfile.ParseFile(filepath.Join(a.agentsDir, fileName+".md"))
}

// Meta 返回表单下拉选项（模型、模式、工具、技能、内置智能体）。
func (a *API) Meta() *zconfig.Meta {
	return zconfig.LoadMeta(a.agentsDir)
}

// ThoughtLevels 按模型引用返回推理档位候选；无匹配时返回通用候选。
func (a *API) ThoughtLevels(ref string) []string {
	if v := zconfig.ThoughtLevelsFor(ref); len(v) > 0 {
		return v
	}
	return zconfig.DefaultThoughtLevels
}

// SaveRequest 是保存请求；OriginalFileName 用于处理改名（另存为新文件并删旧文件）。
type SaveRequest struct {
	Agent            agentfile.Agent `json:"agent"`
	OriginalFileName string          `json:"originalFileName"`
}

// SaveAgent 保存角色。改名时走"新建 + 备份删除旧文件"路径。
func (a *API) SaveAgent(req SaveRequest) (agentfile.Summary, error) {
	ag := req.Agent
	if !safeName(ag.FileName) {
		return agentfile.Summary{}, fmt.Errorf("文件名只能包含字母、数字、- 和 _")
	}
	if strings.TrimSpace(ag.Name) == "" {
		return agentfile.Summary{}, fmt.Errorf("name 不能为空")
	}
	target := filepath.Join(a.agentsDir, ag.FileName+".md")
	original := filepath.Join(a.agentsDir, req.OriginalFileName+".md")
	ag.FilePath = target

	_, targetExists := os.Stat(target)
	renamed := req.OriginalFileName != "" && req.OriginalFileName != ag.FileName

	switch {
	case renamed && targetExists == nil:
		return agentfile.Summary{}, fmt.Errorf("角色 %s 已存在，不能覆盖", ag.FileName)
	case renamed:
		// 改名：备份旧文件 → 写新文件 → 删旧文件；任一步失败都不留半成品
		if _, err := agentfile.BackupFile(original); err != nil {
			return agentfile.Summary{}, fmt.Errorf("备份旧文件失败: %w", err)
		}
		if err := writeNew(&ag); err != nil {
			return agentfile.Summary{}, err
		}
		if err := removeFile(original); err != nil {
			return agentfile.Summary{}, fmt.Errorf("删除旧文件失败: %w", err)
		}
	case os.IsNotExist(targetExists):
		// 全新角色：没有旧版本可备份，直接写
		if err := writeNew(&ag); err != nil {
			return agentfile.Summary{}, err
		}
	default:
		// 原地保存：先备份再写
		if err := ag.Save(); err != nil {
			return agentfile.Summary{}, err
		}
	}
	return ag.Summary(), nil
}

// CreateAgent 新建角色。
func (a *API) CreateAgent(ag agentfile.Agent) (agentfile.Summary, error) {
	if strings.TrimSpace(ag.Name) == "" {
		return agentfile.Summary{}, fmt.Errorf("name 不能为空")
	}
	if ag.FileName == "" {
		ag.FileName = ag.Name
	}
	if err := agentfile.Create(a.agentsDir, &ag); err != nil {
		return agentfile.Summary{}, err
	}
	return ag.Summary(), nil
}

// DeleteAgent 备份后删除角色。
func (a *API) DeleteAgent(fileName string) error {
	if !safeName(fileName) {
		return fmt.Errorf("非法角色名: %q", fileName)
	}
	return agentfile.Delete(a.agentsDir, fileName)
}

// SetPermissionMode 切换单个角色的权限模式（保存前自动备份）。
func (a *API) SetPermissionMode(fileName, mode string) (agentfile.Summary, error) {
	if !validMode(mode) {
		return agentfile.Summary{}, fmt.Errorf("未知权限模式: %s", mode)
	}
	ag, err := a.GetAgent(fileName)
	if err != nil {
		return agentfile.Summary{}, err
	}
	m := mode
	ag.PermissionMode = &m
	if err := ag.Save(); err != nil {
		return agentfile.Summary{}, err
	}
	return ag.Summary(), nil
}

// SetAllPermissionModes 批量切换全部角色，返回成功的数量。
func (a *API) SetAllPermissionModes(mode string) (int, error) {
	if !validMode(mode) {
		return 0, fmt.Errorf("未知权限模式: %s", mode)
	}
	summaries, err := agentfile.List(a.agentsDir)
	if err != nil {
		return 0, err
	}
	ok := 0
	var firstErr error
	for _, s := range summaries {
		if _, err := a.SetPermissionMode(s.FileName, mode); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ok++
	}
	return ok, firstErr
}

// Issue 是一条校验结果；level 为 "error"（阻止保存）或 "warn"（提示不阻止）。
type Issue struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Validate 对表单内容做静态校验。error 级问题阻止保存，warn 级仅提示。
func (a *API) Validate(ag agentfile.Agent) []Issue {
	var issues []Issue
	hard := func(msg string) { issues = append(issues, Issue{"error", msg}) }
	soft := func(msg string) { issues = append(issues, Issue{"warn", msg}) }

	if strings.TrimSpace(ag.Name) == "" {
		hard("name 不能为空")
	}
	if ag.FileName != "" && !safeName(ag.FileName) {
		hard("文件名（保存用的 .md 名）只能包含字母、数字、- 和 _")
	}
	if ag.PermissionMode != nil && *ag.PermissionMode != "" && !validMode(*ag.PermissionMode) {
		hard(fmt.Sprintf("权限模式 %q 不在客户端支持的取值内", *ag.PermissionMode))
	}
	if ag.PermissionMode != nil && *ag.PermissionMode == "auto" {
		// 源码实证：mode.auto.unimplemented，auto 是保留未实现，选中即全拒
		hard("auto 是保留未实现（mode.auto.unimplemented）：设为 auto 后该角色的全部工具调用都会被客户端拒绝，请改用 yolo/edit/default 等档位")
	}
	if ag.Color != nil && *ag.Color != "" && !knownValue(*ag.Color, agentfile.Colors) {
		hard(fmt.Sprintf("颜色 %q 不在白名单（%s）", *ag.Color, strings.Join(agentfile.Colors, "/")))
	}
	if ag.MaxTurns != nil && (*ag.MaxTurns < 1 || *ag.MaxTurns > 200) {
		hard("maxTurns 应在 1–200 之间")
	}
	if ag.Model != nil {
		m := strings.TrimSpace(*ag.Model)
		if m != "" && !strings.Contains(m, "/") {
			soft("model 建议用 providerId/modelId 形式（如 new-provider-3/step-5-preview）")
		}
	}
	if len(ag.Tools) == 0 && len(ag.DisallowedTools) == 0 {
		soft("tools 与 disallowedTools 均为空：角色将继承会话默认工具集")
	}
	known := map[string]bool{}
	for _, t := range zconfig.KnownTools() {
		known[t] = true
	}
	var unknown []string
	for _, t := range append(append([]string{}, ag.Tools...), ag.DisallowedTools...) {
		if !known[t] && !strings.HasPrefix(t, "mcp__") {
			unknown = append(unknown, t)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		soft("非内置工具名（确认存在再保存）: " + strings.Join(unknown, ", "))
	}
	return issues
}

// ---------- 窗口控制（悬浮面板） ----------

// FloatPanelVisible 返回悬浮面板当前是否可见。
func (a *API) FloatPanelVisible() bool {
	return a.floatWin != nil && a.floatWin.IsVisible()
}

// ToggleFloatPanel 切换悬浮面板可见性，返回切换后的可见性。
func (a *API) ToggleFloatPanel() bool {
	if a.floatWin == nil {
		return false
	}
	if a.floatWin.IsVisible() {
		a.floatWin.Hide()
		return false
	}
	a.floatWin.Show()
	a.floatWin.Center()
	return true
}

// HideFloatPanel 收起悬浮面板（不等同于退出）。
func (a *API) HideFloatPanel() {
	if a.floatWin != nil {
		a.floatWin.Hide()
	}
}

// ShowMainWindow 从悬浮面板切换到主窗口（切换语义：收起悬浮面板再显示主窗口）。
func (a *API) ShowMainWindow() {
	if a.floatWin != nil && a.floatWin.IsVisible() {
		a.floatWin.Hide()
	}
	if a.mainWin == nil {
		return
	}
	a.mainWin.Show()
	a.mainWin.Center()
}

// ---------- 内部辅助 ----------

func safeName(name string) bool {
	return agentfile.SafeFileName(name)
}

func statFile(p string) (os.FileInfo, error) { return os.Stat(p) }

func removeFile(p string) error { return os.Remove(p) }

func writeNew(ag *agentfile.Agent) error {
	return os.WriteFile(ag.FilePath, ag.Marshal(), 0o644)
}

func validMode(mode string) bool {
	for _, m := range agentfile.PermissionModes {
		if m == mode {
			return true
		}
	}
	return false
}

func knownValue(v string, whitelist []string) bool {
	for _, w := range whitelist {
		if v == w {
			return true
		}
	}
	return false
}
