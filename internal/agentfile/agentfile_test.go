package agentfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleRealistic = `---
name: "coder"
description: "执行者：在主智能体明确的目标、范围和验收条件下完成编码任务。"
color: blue
model: new-provider-3/step-5-preview
thoughtLevel: max
tools:
  - Read
  - Grep
  - Bash
  - Edit
skills:
  - scalpel
  - tdd
permissionMode: yolo
maxTurns: 60
injectAgentsMd: true
---

在给定范围内执行编码任务。

## 工作方式

- 只改任务范围内的东西。
`

const sampleNoFrontmatter = "只是一个正文，没有 frontmatter。\n"

const sampleOddities = `---
name: odd
description: ""
color:
model:
permissionMode: "yolo"
background: false
tools:
  - "Read"
  - Read
customField: hello
customList:
  - a
  - b
---

body
`

func TestParseRealistic(t *testing.T) {
	a, err := Parse(sampleRealistic, "/tmp/coder.md")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "coder" {
		t.Errorf("name = %q", a.Name)
	}
	if a.Description != "执行者：在主智能体明确的目标、范围和验收条件下完成编码任务。" {
		t.Errorf("description = %q", a.Description)
	}
	if a.Color == nil || *a.Color != "blue" {
		t.Errorf("color = %v", a.Color)
	}
	if a.Model == nil || *a.Model != "new-provider-3/step-5-preview" {
		t.Errorf("model = %v", a.Model)
	}
	if a.PermissionMode == nil || *a.PermissionMode != "yolo" {
		t.Errorf("permissionMode = %v", a.PermissionMode)
	}
	if a.MaxTurns == nil || *a.MaxTurns != 60 {
		t.Errorf("maxTurns = %v", a.MaxTurns)
	}
	if a.InjectAgentsMd == nil || !*a.InjectAgentsMd {
		t.Errorf("injectAgentsMd = %v", a.InjectAgentsMd)
	}
	if a.Background != nil {
		t.Errorf("background 应为未设置, got %v", *a.Background)
	}
	if len(a.Tools) != 4 || a.Tools[3] != "Edit" {
		t.Errorf("tools = %v", a.Tools)
	}
	if len(a.Skills) != 2 {
		t.Errorf("skills = %v", a.Skills)
	}
	if !strings.Contains(a.Body, "在给定范围内执行编码任务。") {
		t.Errorf("body 丢失: %q", a.Body)
	}
	if a.FileName != "coder" {
		t.Errorf("fileName = %q", a.FileName)
	}
}

func TestRoundTripStable(t *testing.T) {
	for _, src := range []string{sampleRealistic, sampleOddities} {
		a, err := Parse(src, "/tmp/x.md")
		if err != nil {
			t.Fatal(err)
		}
		once := string(a.Marshal())
		b, err := Parse(once, "/tmp/x.md")
		if err != nil {
			t.Fatal(err)
		}
		twice := string(b.Marshal())
		if once != twice {
			t.Errorf("二次序列化不稳定:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
		}
	}
}

func TestNoFrontmatter(t *testing.T) {
	a, err := Parse(sampleNoFrontmatter, "/tmp/plain.md")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "plain" {
		t.Errorf("name 应回退为文件名, got %q", a.Name)
	}
	out := string(a.Marshal())
	if !strings.HasPrefix(out, "---\nname: plain\n") {
		t.Errorf("序列化缺少 name 头:\n%s", out)
	}
}

func TestOddities(t *testing.T) {
	a, err := Parse(sampleOddities, "/tmp/odd.md")
	if err != nil {
		t.Fatal(err)
	}
	if a.Color != nil {
		t.Errorf("color 空值应视为未设置, got %q", *a.Color)
	}
	if a.Description != "" {
		t.Errorf("description = %q", a.Description)
	}
	if a.Background == nil || *a.Background {
		t.Errorf("background = %v", a.Background)
	}
	if len(a.Tools) != 2 {
		t.Errorf("tools 去重前应为 2 项, got %v", a.Tools)
	}
	if got := a.UnknownLists["customList"]; len(got) != 2 {
		t.Errorf("customList = %v", got)
	}
	out := string(a.Marshal())
	if !strings.Contains(out, "customList:") || !strings.Contains(out, "customField: hello") {
		t.Errorf("未知键未保留:\n%s", out)
	}
}

func TestNeedsQuote(t *testing.T) {
	cases := map[string]bool{
		"yolo":                      false,
		"new-provider-3/step-5-preview": false,
		"Read":                      false,
		"":                          true,
		" true":                     true,
		"true":                      true,
		"60":                        true,
		"a: b":                      true,
		"x#y":                       true,
		"- lead":                    true,
		"has space":                 false,
		"(paren)":                   true, // 首字符不在安全集，客户端会加引号
	}
	for in, want := range cases {
		if got := needsQuote(in); got != want {
			t.Errorf("needsQuote(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSaveBackupDeleteLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvBackupDir, filepath.Join(dir, "backups"))
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	newAgent := &Agent{
		FileName:    "tester",
		Name:        "tester",
		Description: "测试角色",
		Body:        "你是测试角色。",
	}
	mode := "yolo"
	newAgent.PermissionMode = &mode
	if err := Create(agentsDir, newAgent); err != nil {
		t.Fatal(err)
	}
	// 重复创建应报错
	if err := Create(agentsDir, &Agent{FileName: "tester"}); err == nil {
		t.Error("重复创建应报错")
	}
	// 非法文件名
	if err := Create(agentsDir, &Agent{FileName: "../evil"}); err == nil {
		t.Error("非法文件名应报错")
	}

	got, err := ParseFile(filepath.Join(agentsDir, "tester.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got.PermissionMode == nil || *got.PermissionMode != "yolo" {
		t.Fatalf("permissionMode = %v", got.PermissionMode)
	}

	// 改模式并保存（应产生备份）
	edit := "edit"
	got.PermissionMode = &edit
	got.Tools = []string{"Read", "Bash"}
	if err := got.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := ParseFile(filepath.Join(agentsDir, "tester.md"))
	if err != nil {
		t.Fatal(err)
	}
	if *again.PermissionMode != "edit" || len(again.Tools) != 2 {
		t.Errorf("保存后状态不对: %v %v", *again.PermissionMode, again.Tools)
	}
	// 备份文件应存在且内容是保存前的 yolo 版本
	baks, err := filepath.Glob(filepath.Join(dir, "backups", "*.bak"))
	if err != nil || len(baks) != 1 {
		t.Fatalf("应有 1 个备份, got %v (%v)", baks, err)
	}
	bak, err := os.ReadFile(baks[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bak), "permissionMode: yolo") {
		t.Errorf("备份内容应为保存前版本:\n%s", bak)
	}

	// 删除（再产出一个备份）
	if err := Delete(agentsDir, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "tester.md")); !os.IsNotExist(err) {
		t.Error("删除后文件不应存在")
	}
}

func TestListAndSummary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(sampleRealistic), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("应只列出 1 个角色, got %d", len(list))
	}
	if list[0].PermissionMode != "yolo" || list[0].ToolCount != 4 {
		t.Errorf("summary = %+v", list[0])
	}
}
