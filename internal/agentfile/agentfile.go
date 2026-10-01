// Package agentfile 负责读写 ZCode 自定义子智能体定义（~/.zcode/agents/*.md）。
// 文件格式与客户端 parser/serializer 对齐：YAML frontmatter + 正文 system prompt。
// 写格式刻意复刻客户端 generateSubagentMarkdown 的输出（字段顺序、列表缩进、引号规则），
// 保证本工具写出的文件能被 ZCode 原样读回。
package agentfile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnvDir 覆盖角色目录（测试用）。
const EnvDir = "ZCODE_AGENTS_DIR"

// EnvBackupDir 覆盖备份目录（测试用）。
const EnvBackupDir = "ZCODE_BACKUP_DIR"

// PermissionModes 是客户端 parser 接受的全部取值。
var PermissionModes = []string{
	"default", "yolo", "plan", "edit", "acceptEdits",
	"auto", "dontAsk", "bypassPermissions", "autoEdit", "build",
}

// Colors 是客户端表单支持的颜色白名单。
var Colors = []string{"red", "blue", "green", "yellow", "purple", "orange", "pink", "cyan"}

// Agent 是一个角色文件的完整内存表示。
// 字符串字段用指针区分"未设置"与"显式设为空"；布尔用指针同理。
type Agent struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Color            *string  `json:"color"`
	Model            *string  `json:"model"`
	ThoughtLevel     *string  `json:"thoughtLevel"`
	Tools            []string `json:"tools"`
	DisallowedTools  []string `json:"disallowedTools"`
	Skills           []string `json:"skills"`
	PermissionMode   *string  `json:"permissionMode"`
	MaxTurns         *int     `json:"maxTurns"`
	Background       *bool    `json:"background"`
	InjectAgentsMd   *bool    `json:"injectAgentsMd"`
	MCPServers       []string `json:"mcpServers"`
	Body             string   `json:"body"`
	UnknownLists     map[string][]string `json:"unknownLists,omitempty"`
	UnknownLines     []string          `json:"unknownLines,omitempty"`

	// 非 frontmatter 元数据
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
	Raw      string `json:"raw,omitempty"`
}

// Summary 是列表用摘要。
type Summary struct {
	FileName       string   `json:"fileName"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	PermissionMode string   `json:"permissionMode"`
	Model          string   `json:"model"`
	ToolCount      int      `json:"toolCount"`
	SkillCount     int      `json:"skillCount"`
	MaxTurns       int      `json:"maxTurns"`
	BodyLines      int      `json:"bodyLines"`
	ModifiedAt     string   `json:"modifiedAt"`
}

// DefaultDir 返回角色目录。
func DefaultDir() string {
	if d := os.Getenv(EnvDir); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".zcode", "agents")
	}
	return filepath.Join(home, ".zcode", "agents")
}

func backupRoot() string {
	if d := os.Getenv(EnvBackupDir); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".zcode", "v2", "backups", "subagent-control")
	}
	return filepath.Join(home, ".zcode", "v2", "backups", "subagent-control")
}

// List 列出目录下全部角色摘要（按文件名排序）。
func List(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Summary{}, nil
		}
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		a, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", e.Name(), err)
		}
		out = append(out, a.Summary())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FileName < out[j].FileName })
	if out == nil {
		out = []Summary{}
	}
	return out, nil
}

// Summary 生成列表摘要。
func (a *Agent) Summary() Summary {
	s := Summary{
		FileName:       a.FileName,
		Name:           a.Name,
		Description:    a.Description,
		ToolCount:      len(a.Tools),
		SkillCount:     len(a.Skills),
	}
	if a.PermissionMode != nil {
		s.PermissionMode = *a.PermissionMode
	}
	if a.Model != nil {
		s.Model = *a.Model
	}
	if a.MaxTurns != nil {
		s.MaxTurns = *a.MaxTurns
	}
	if a.FilePath != "" {
		if st, err := os.Stat(a.FilePath); err == nil {
			s.ModifiedAt = st.ModTime().Format("2006-01-02 15:04")
		}
	}
	s.BodyLines = len(strings.Split(strings.Trim(a.Body, "\n"), "\n"))
	if strings.Trim(a.Body, "\n") == "" {
		s.BodyLines = 0
	}
	return s
}

// ParseFile 读取并解析角色文件。
func ParseFile(path string) (*Agent, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	a, err := Parse(string(b), path)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// Parse 解析 markdown 内容。frontmatter 缺失时按纯正文处理（name 回退为文件名）。
func Parse(content, path string) (*Agent, error) {
	a := &Agent{FilePath: path, Raw: content}
	if path != "" {
		a.FileName = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	fm, body, ok := splitFrontmatter(content)
	if ok {
		a.parseFrontmatter(fm)
	} else {
		body = content
	}
	a.Body = strings.Trim(body, "\n")
	if a.Name == "" {
		a.Name = a.FileName
	}
	return a, nil
}

// splitFrontmatter 切出首尾 --- 之间的 frontmatter 与余下正文。
func splitFrontmatter(content string) (fm, body string, ok bool) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", content, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", content, false
}

var keyRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\s*:\s*(.*)$`)

func (a *Agent) parseFrontmatter(fm string) {
	curKey := ""
	for _, ln := range strings.Split(fm, "\n") {
		t := strings.TrimRight(ln, "\r")
		trimmed := strings.TrimSpace(t)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// 列表项（允许任意缩进）
		if trimmed == "-" || strings.HasPrefix(trimmed, "- ") {
			val := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
			a.appendItem(curKey, val)
			continue
		}
		m := keyRe.FindStringSubmatch(strings.TrimLeft(t, " \t"))
		if m == nil {
			a.UnknownLines = append(a.UnknownLines, t)
			curKey = ""
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		curKey = key
		if val == "" {
			// 可能是列表键，先不落值；首个列表项到达时再归类
			continue
		}
		a.setScalar(key, unquote(val))
	}
}

func (a *Agent) appendItem(key, val string) {
	if val == "" && key != "" {
		// 占位空项忽略
	}
	switch key {
	case "tools":
		a.Tools = append(a.Tools, val)
	case "disallowedTools":
		a.DisallowedTools = append(a.DisallowedTools, val)
	case "skills":
		a.Skills = append(a.Skills, val)
	case "mcpServers":
		a.MCPServers = append(a.MCPServers, val)
	case "":
		// 孤立列表项，原样保留
		a.UnknownLines = append(a.UnknownLines, "- "+val)
	default:
		if a.UnknownLists == nil {
			a.UnknownLists = map[string][]string{}
		}
		a.UnknownLists[key] = append(a.UnknownLists[key], val)
	}
}

func (a *Agent) setScalar(key, val string) {
	switch key {
	case "name":
		a.Name = val
	case "description":
		a.Description = val
	case "color":
		a.Color = ptr(val)
	case "model":
		a.Model = ptr(val)
	case "thoughtLevel":
		a.ThoughtLevel = ptr(val)
	case "permissionMode":
		a.PermissionMode = ptr(val)
	case "maxTurns":
		if n, err := strconv.Atoi(val); err == nil {
			a.MaxTurns = &n
		} else {
			a.UnknownLines = append(a.UnknownLines, "maxTurns: "+quote(val))
		}
	case "background":
		if b, ok := parseBool(val); ok {
			a.Background = &b
		}
	case "injectAgentsMd":
		if b, ok := parseBool(val); ok {
			a.InjectAgentsMd = &b
		}
	default:
		a.UnknownLines = append(a.UnknownLines, key+": "+quote(val))
	}
}

func ptr(s string) *string { return &s }

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "true", "yes", "on":
		return true, true
	case "false", "no", "off":
		return false, true
	}
	return false, false
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		inner := v[1 : len(v)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	return v
}

// ---------- 序列化 ----------

var numRe = regexp.MustCompile(`(?i)^[-+]?(\d+|\d*\.\d+)(e[-+]?\d+)?$`)
var specialRe = regexp.MustCompile(`(?i)^(false|null|true|~)$`)
var safeRe = regexp.MustCompile(`^[A-Za-z0-9_./@*][A-Za-z0-9_./@*\s()-]*$`)

// needsQuote 复刻客户端的引号判定（Bwe 的逆）：裸值不安全时加双引号。
func needsQuote(v string) bool {
	if v == "" {
		return true
	}
	if strings.TrimSpace(v) != v {
		return true
	}
	if specialRe.MatchString(v) {
		return true
	}
	if numRe.MatchString(v) {
		return true
	}
	if strings.ContainsRune(`*?:,[]{}&!|>'"%@`+"`-", rune(v[0])) {
		return true
	}
	if strings.Contains(v, "#") || strings.Contains(v, ": ") {
		return true
	}
	return !safeRe.MatchString(v)
}

func quote(v string) string {
	if !needsQuote(v) {
		return v
	}
	esc := strings.ReplaceAll(v, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return `"` + esc + `"`
}

// Marshal 生成文件内容，字段顺序与客户端 writer 一致。
func (a *Agent) Marshal() []byte {
	var b strings.Builder
	b.WriteString("---\n")
	writeKV(&b, "name", a.Name)
	writeKV(&b, "description", a.Description)
	if a.Color != nil && *a.Color != "" {
		writeKV(&b, "color", *a.Color)
	}
	if a.Model != nil && *a.Model != "" {
		writeKV(&b, "model", *a.Model)
	}
	if a.ThoughtLevel != nil && *a.ThoughtLevel != "" {
		writeKV(&b, "thoughtLevel", *a.ThoughtLevel)
	}
	writeList(&b, "tools", a.Tools)
	writeList(&b, "disallowedTools", a.DisallowedTools)
	writeList(&b, "skills", a.Skills)
	if a.PermissionMode != nil && *a.PermissionMode != "" {
		writeKV(&b, "permissionMode", *a.PermissionMode)
	}
	if a.MaxTurns != nil {
		b.WriteString("maxTurns: " + strconv.Itoa(*a.MaxTurns) + "\n")
	}
	if a.Background != nil {
		b.WriteString("background: " + strconv.FormatBool(*a.Background) + "\n")
	}
	if a.InjectAgentsMd != nil {
		b.WriteString("injectAgentsMd: " + strconv.FormatBool(*a.InjectAgentsMd) + "\n")
	}
	writeList(&b, "mcpServers", a.MCPServers)
	for _, k := range sortedKeys(a.UnknownLists) {
		writeList(&b, k, a.UnknownLists[k])
	}
	for _, ln := range a.UnknownLines {
		b.WriteString(strings.TrimRight(ln, "\r"))
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	body := strings.Trim(a.Body, "\n")
	if body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func sortedKeys(m map[string][]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func writeKV(b *strings.Builder, key, val string) {
	b.WriteString(key + ": " + quote(val) + "\n")
}

func writeList(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(key + ":\n")
	for _, it := range items {
		b.WriteString("  - " + quote(it) + "\n")
	}
}

// ---------- 落盘 ----------

// Save 备份后写回原路径。
func (a *Agent) Save() error {
	if a.FilePath == "" {
		return fmt.Errorf("文件路径为空")
	}
	if _, err := BackupFile(a.FilePath); err != nil {
		return fmt.Errorf("备份失败: %w", err)
	}
	return os.WriteFile(a.FilePath, a.Marshal(), 0o644)
}

// BackupFile 把文件复制到 ~/.zcode/v2/backups/subagent-control/ 下并返回备份路径。
func BackupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(backupRoot())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Base(path)
	// 时间戳到微秒：同一会话内的连续保存（秒级相同）不会互相覆盖
	dst := filepath.Join(dir, fmt.Sprintf("%s.%s.bak", base, time.Now().Format("20060102-150405.000000")))
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	pruneBackups(dir, 200)
	return dst, nil
}

// pruneBackups 只保留最近 keep 个备份文件。
func pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keep {
		return
	}
	type f struct {
		name    string
		modTime time.Time
	}
	var files []f
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, f{e.Name(), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
	for _, old := range files[keep:] {
		_ = os.Remove(filepath.Join(dir, old.name))
	}
}

// SafeFileName 校验角色文件名（新建时用）。
func SafeFileName(name string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`).MatchString(name)
}

// Create 在 dir 下新建角色文件；已存在则报错。
func Create(dir string, a *Agent) error {
	if !SafeFileName(a.FileName) {
		return fmt.Errorf("文件名只能包含字母、数字、- 和 _（且不以 - _ 开头）")
	}
	path := filepath.Join(dir, a.FileName+".md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("角色 %s 已存在", a.FileName)
	}
	a.FilePath = path
	return os.WriteFile(path, a.Marshal(), 0o644)
}

// Delete 备份后删除角色文件。
func Delete(dir, fileName string) error {
	path := filepath.Join(dir, fileName+".md")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("角色 %s 不存在", fileName)
	}
	if _, err := BackupFile(path); err != nil {
		return fmt.Errorf("备份失败: %w", err)
	}
	return os.Remove(path)
}
