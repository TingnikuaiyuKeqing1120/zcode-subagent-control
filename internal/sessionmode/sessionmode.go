// Package sessionmode 读写 ZCode CLI 数据库里的会话执行状态，实现“会话模式覆盖”。
//
// 背景（源码级结论）：交互式子智能体的模式在派生时由主会话实时模式决定并定格，
// 运行中的实例无法通过产品 UI 改模式；而每个会话的持久化执行状态
// （session_entry 表 runtime/execution_state 行）会在该会话的 runtime
// 下次创建/恢复时被读取。改写这行即可覆盖该会话下次恢复时的模式。
//
// 只改 mode/planEnabled 两个字段，其余内容原样保留；改写前自动备份原值。
package sessionmode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，无 CGO
)

// EnvDBPath 覆盖 CLI 数据库路径（测试用）。
const EnvDBPath = "ZCODE_CLI_DB"

// ValidModes 是客户端 CollaborationMode 的可写值（auto 刻意排除：保留未实现）。
var ValidModes = []string{"yolo", "build", "edit", "plan"}

// SessionModeInfo 是一个会话的持久化模式状态。
type SessionModeInfo struct {
	SessionID   string `json:"sessionId"`
	Title       string `json:"title"`
	IsSubagent  bool   `json:"isSubagent"`
	ParentID    string `json:"parentId"`
	Mode        string `json:"mode"`
	PlanEnabled bool   `json:"planEnabled"`
	UpdatedAt   string `json:"updatedAt"`
	HasState    bool   `json:"hasState"`
}

// DBPath 返回 ZCode CLI 数据库路径。
func DBPath() string {
	if p := os.Getenv(EnvDBPath); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".zcode", "cli", "db", "db.sqlite")
	}
	return filepath.Join(home, ".zcode", "cli", "db", "db.sqlite")
}

func openRO() (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", DBPath())
	return sql.Open("sqlite", dsn)
}

func openRW() (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)", DBPath())
	return sql.Open("sqlite", dsn)
}

// List 列出全部有执行状态记录的会话（主会话 + 子智能体会话）。
func List() ([]SessionModeInfo, error) {
	db, err := openRO()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id,
		       COALESCE(s.title, ''),
		       COALESCE(s.parent_id, ''),
		       e.data,
		       e.time_updated
		FROM session_entry e
		JOIN session s ON s.id = e.session_id
		WHERE e.type = 'runtime/execution_state'
		ORDER BY e.time_updated DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionModeInfo
	for rows.Next() {
		var id, title, parent, data string
		var updated int64
		if err := rows.Scan(&id, &title, &parent, &data, &updated); err != nil {
			return nil, err
		}
		info := SessionModeInfo{
			SessionID:  id,
			Title:      title,
			IsSubagent: strings.HasPrefix(id, "sess_subagent_"),
			ParentID:   parent,
			Mode:       "unknown",
			UpdatedAt:  time.UnixMilli(updated).Format("01-02 15:04"),
			HasState:   true,
		}
		var st struct {
			Mode        string `json:"mode"`
			PlanEnabled bool   `json:"planEnabled"`
		}
		if json.Unmarshal([]byte(data), &st) == nil {
			if st.Mode != "" {
				info.Mode = st.Mode
			}
			info.PlanEnabled = st.PlanEnabled
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// BackupDir 返回会话模式覆盖的备份目录。
func BackupDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".zcode", "v2", "backups", "subagent-control", "sessionmode")
	}
	return filepath.Join(home, ".zcode", "v2", "backups", "subagent-control", "sessionmode")
}

// Set 覆盖目标会话的持久化模式；先备份原值，只改 mode/planEnabled。
func Set(sessionID, mode string) error {
	if !validMode(mode) {
		return fmt.Errorf("模式 %q 不可写（可用: yolo/build/edit/plan；auto 为保留未实现，刻意排除）", mode)
	}
	db, err := openRW()
	if err != nil {
		return err
	}
	defer db.Close()

	var old string
	err = db.QueryRow(
		`SELECT data FROM session_entry WHERE session_id = ? AND type = 'runtime/execution_state'`,
		sessionID,
	).Scan(&old)
	if err != nil {
		return fmt.Errorf("该会话没有持久化执行状态记录（仅支持改写已存在的记录）: %w", err)
	}

	// 备份原值
	if err := os.MkdirAll(BackupDir(), 0o755); err != nil {
		return err
	}
	bakName := fmt.Sprintf("%s.%s.json", sessionID, time.Now().Format("20060102-150405.000000"))
	if err := os.WriteFile(filepath.Join(BackupDir(), bakName), []byte(old), 0o644); err != nil {
		return err
	}

	// 只改 mode/planEnabled，保留其余字段
	var raw map[string]any
	if err := json.Unmarshal([]byte(old), &raw); err != nil {
		return fmt.Errorf("执行状态不是合法 JSON: %w", err)
	}
	raw["mode"] = mode
	raw["planEnabled"] = mode == "plan"
	updated, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	res, err := db.Exec(
		`UPDATE session_entry SET data = ?, time_updated = ? WHERE session_id = ? AND type = 'runtime/execution_state'`,
		string(updated), time.Now().UnixMilli(), sessionID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("未更新任何行（会话可能在同一瞬间被客户端重写）")
	}
	return nil
}

func validMode(mode string) bool {
	for _, m := range ValidModes {
		if m == mode {
			return true
		}
	}
	return false
}
