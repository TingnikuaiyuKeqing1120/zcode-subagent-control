package sessionmode

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// fixtureDB 按 ZCode CLI 库的真实 schema 建一个最小测试库。
func fixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "db.sqlite")
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, workspace_id TEXT,
		   parent_id TEXT, slug TEXT, directory TEXT, path TEXT, title TEXT, version INTEGER,
		   share_url TEXT, summary_additions TEXT, summary_deletions TEXT)`,
		`CREATE TABLE session_entry (id TEXT PRIMARY KEY, session_id TEXT, type TEXT,
		   time_created INTEGER, time_updated INTEGER, data TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	rows := []struct{ id, sid, typ, data string }{
		{"e1", "sess_main_1", "runtime/execution_state", `{"mode":"build","planEnabled":false}`},
		{"e2", "sess_subagent_agent_a", "runtime/execution_state", `{"mode":"yolo","planEnabled":false}`},
		{"e3", "sess_subagent_agent_b", "runtime/execution_state", `{"mode":"plan","planEnabled":true,"extra":{"keep":1}}`},
		{"e4", "sess_nostate_1", "runtime/model_selection", `{"x":1}`},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO session_entry (id, session_id, type, time_created, time_updated, data)
			VALUES (?,?,?,?,?,?)`, r.id, r.sid, r.typ, 1700000000000, 1700000000000, r.data); err != nil {
			t.Fatal(err)
		}
	}
	sessions := []struct{ id, title, parent string }{
		{"sess_main_1", "主会话标题", ""},
		{"sess_subagent_agent_a", "子智能体 A", "sess_main_1"},
		{"sess_subagent_agent_b", "子智能体 B", "sess_main_1"},
		{"sess_nostate_1", "无状态会话", ""},
	}
	for _, s := range sessions {
		if _, err := db.Exec(`INSERT INTO session (id, project_id, parent_id, title) VALUES (?,?,?,?)`,
			s.id, "proj", s.parent, s.title); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestList(t *testing.T) {
	t.Setenv(EnvDBPath, fixtureDB(t))
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("应有 3 个有执行状态的会话, got %d", len(list))
	}
	byID := map[string]SessionModeInfo{}
	for _, x := range list {
		byID[x.SessionID] = x
	}
	if !byID["sess_main_1"].HasState || byID["sess_main_1"].IsSubagent {
		t.Errorf("主会话解析不对: %+v", byID["sess_main_1"])
	}
	if !byID["sess_subagent_agent_a"].IsSubagent {
		t.Errorf("子会话标记不对: %+v", byID["sess_subagent_agent_a"])
	}
	if byID["sess_subagent_agent_b"].Mode != "plan" || !byID["sess_subagent_agent_b"].PlanEnabled {
		t.Errorf("plan 会话解析不对: %+v", byID["sess_subagent_agent_b"])
	}
}

func TestSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDBPath, fixtureDB(t))
	t.Setenv("HOME", dir) // 备份写到临时 HOME 下，不污染真实目录
	// Windows 上 os.UserHomeDir 用 USERPROFILE
	t.Setenv("USERPROFILE", dir)

	if err := Set("sess_subagent_agent_a", "edit"); err != nil {
		t.Fatal(err)
	}
	list, _ := List()
	for _, x := range list {
		if x.SessionID == "sess_subagent_agent_a" {
			if x.Mode != "edit" || x.PlanEnabled {
				t.Errorf("改写后模式不对: %+v", x)
			}
		}
	}

	// 切到 plan 时 planEnabled 应同步为 true
	if err := Set("sess_subagent_agent_a", "plan"); err != nil {
		t.Fatal(err)
	}
	list, _ = List()
	for _, x := range list {
		if x.SessionID == "sess_subagent_agent_a" && (!x.PlanEnabled || x.Mode != "plan") {
			t.Errorf("plan 切换不对: %+v", x)
		}
	}

	// 备份文件应存在且内容是旧值
	baks, _ := filepath.Glob(filepath.Join(BackupDir(), "*.json"))
	if len(baks) != 2 {
		t.Fatalf("应有 2 个备份, got %d", len(baks))
	}
	old, _ := os.ReadFile(baks[0])
	if string(old) != `{"mode":"yolo","planEnabled":false}` && string(old) != `{"mode":"edit","planEnabled":false}` {
		t.Errorf("备份内容不对: %s", old)
	}

	// 非法模式 / 不存在的会话
	if err := Set("sess_subagent_agent_a", "auto"); err == nil {
		t.Error("auto 应被拒绝")
	}
	if err := Set("sess_nostate_1", "yolo"); err == nil {
		t.Error("无执行状态的会话应拒绝（无旧值可备份）")
	}
}
