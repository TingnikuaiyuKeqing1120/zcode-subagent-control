package main

import (
	"os"
	"path/filepath"
	"testing"

	"zcode-subagent-control/internal/agentfile"
)

func newTestAPI(t *testing.T) (*API, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(agentfile.EnvBackupDir, filepath.Join(dir, "backups"))
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &API{agentsDir: agentsDir}, agentsDir
}

func strp(s string) *string { return &s }

func TestSaveAgentNewAndRenameAndOverwrite(t *testing.T) {
	api, agentsDir := newTestAPI(t)

	// 新建
	sum, err := api.SaveAgent(SaveRequest{
		Agent: agentfile.Agent{
			FileName:       "t1",
			Name:           "t1",
			Description:    "测试一",
			PermissionMode: strp("yolo"),
			Tools:          []string{"Read"},
		},
		OriginalFileName: "",
	})
	if err != nil {
		t.Fatalf("新建失败: %v", err)
	}
	if sum.PermissionMode != "yolo" {
		t.Errorf("permissionMode = %q", sum.PermissionMode)
	}

	// 原地保存（改模式 + 加工具），应产生备份
	sum, err = api.SaveAgent(SaveRequest{
		Agent: agentfile.Agent{
			FileName:       "t1",
			Name:           "t1",
			Description:    "测试一",
			PermissionMode: strp("edit"),
			Tools:          []string{"Read", "Bash"},
		},
		OriginalFileName: "t1",
	})
	if err != nil {
		t.Fatalf("原地保存失败: %v", err)
	}
	if sum.PermissionMode != "edit" {
		t.Errorf("permissionMode = %q", sum.PermissionMode)
	}

	// 改名 t1 → t2，旧文件应消失、新文件应存在
	if _, err := api.SaveAgent(SaveRequest{
		Agent: agentfile.Agent{
			FileName:       "t2",
			Name:           "t2",
			Description:    "测试一",
			PermissionMode: strp("edit"),
		},
		OriginalFileName: "t1",
	}); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "t1.md")); !os.IsNotExist(err) {
		t.Error("改名后旧文件应不存在")
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "t2.md")); err != nil {
		t.Errorf("改名后新文件应存在: %v", err)
	}

	// 备份目录应有 2 个（改名 1 + 原地保存 1）
	baks, _ := filepath.Glob(filepath.Join(os.Getenv(agentfile.EnvBackupDir), "*.bak"))
	if len(baks) != 2 {
		t.Errorf("应有 2 个备份, got %d", len(baks))
	}
}

func TestSaveAgentRejectsCollision(t *testing.T) {
	api, agentsDir := newTestAPI(t)
	if err := os.WriteFile(filepath.Join(agentsDir, "a.md"), []byte("---\nname: a\n---\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "b.md"), []byte("---\nname: b\n---\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := api.SaveAgent(SaveRequest{
		Agent:            agentfile.Agent{FileName: "b", Name: "b"},
		OriginalFileName: "a",
	})
	if err == nil {
		t.Error("改名到已存在的角色应报错")
	}
}

func TestSetPermissionModes(t *testing.T) {
	api, agentsDir := newTestAPI(t)
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(agentsDir, n+".md"),
			[]byte("---\nname: "+n+"\ndescription: d\npermissionMode: default\n---\n\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sum, err := api.SetPermissionMode("a", "yolo")
	if err != nil {
		t.Fatal(err)
	}
	if sum.PermissionMode != "yolo" {
		t.Errorf("a 的权限模式 = %q", sum.PermissionMode)
	}
	if _, err := api.SetPermissionMode("a", "banana"); err == nil {
		t.Error("未知模式应报错")
	}
	n, err := api.SetAllPermissionModes("edit")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("应切换 2 个, got %d", n)
	}
	list, err := api.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range list {
		if x.PermissionMode != "edit" {
			t.Errorf("%s 的权限模式 = %q", x.FileName, x.PermissionMode)
		}
	}
}

func TestValidate(t *testing.T) {
	api, _ := newTestAPI(t)
	// 错误级：空 name / 坏模式 / 坏文件名
	got := api.Validate(agentfile.Agent{Name: "", FileName: "ok", PermissionMode: strp("banana")})
	hasErr := false
	for _, i := range got {
		if i.Level == "error" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Errorf("应有 error 级问题, got %+v", got)
	}
	// 干净的角色不应有 error
	got = api.Validate(agentfile.Agent{
		Name:           "x",
		FileName:       "x",
		PermissionMode: strp("yolo"),
		Tools:          []string{"Read", "Bash"},
		MaxTurns:       intp(30),
	})
	for _, i := range got {
		if i.Level == "error" {
			t.Errorf("干净角色不应有 error: %s", i.Message)
		}
	}
}

func intp(n int) *int { return &n }

func TestValidateRejectsAutoMode(t *testing.T) {
	api, _ := newTestAPI(t)
	got := api.Validate(agentfile.Agent{
		Name:           "x",
		FileName:       "x",
		PermissionMode: strp("auto"),
		Tools:          []string{"Read"},
	})
	if len(got) == 0 || got[0].Level != "error" {
		t.Fatalf("auto 应触发 error 级校验, got %+v", got)
	}
}
