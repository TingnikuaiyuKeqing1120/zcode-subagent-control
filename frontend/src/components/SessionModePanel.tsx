import { useEffect, useMemo, useState } from 'react'
import { ListSessionModes, SetSessionMode } from '../../bindings/zcode-subagent-control/api'
import type { SessionModeInfo } from '../../bindings/zcode-subagent-control/internal/sessionmode/models'

const WRITABLE = ['yolo', 'build', 'edit', 'plan']

const modeBadge = (mode: string) => {
  if (mode === 'yolo' || mode === 'bypassPermissions' || mode === 'dontAsk') return 'tier-yolo'
  if (mode === 'edit') return 'tier-edit'
  if (mode === 'plan') return 'tier-plan'
  if (mode === 'auto') return 'tier-auto'
  return 'tier-build'
}

/**
 * SessionModePanel —— 会话模式覆盖面板。
 *
 * 改的是 ZCode CLI 数据库里的持久化执行状态：对该会话**下次恢复 / 重启 ZCode 后**生效，
 * 运行中的实例不受影响（源码：子会话模式在派生时定格，无 UI 改运行中实例的入口）。
 */
export default function SessionModePanel({ onClose }: { onClose: () => void }) {
  const [list, setList] = useState<SessionModeInfo[]>([])
  const [filter, setFilter] = useState('')
  const [toast, setToast] = useState<string | null>(null)
  const [busyId, setBusyId] = useState('')

  useEffect(() => {
    ;(async () => {
      try {
        setList((await ListSessionModes()) ?? [])
      } catch (e) {
        setToast(`读取失败: ${String(e)}`)
      }
    })()
  }, [])

  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => setToast(null), 3200)
    return () => clearTimeout(t)
  }, [toast])

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q) return list
    return list.filter(
      (x) =>
        x.sessionId.toLowerCase().includes(q) ||
        (x.title ?? '').toLowerCase().includes(q) ||
        (x.mode ?? '').toLowerCase().includes(q),
    )
  }, [list, filter])

  const setMode = async (s: SessionModeInfo, mode: string) => {
    if (!WRITABLE.includes(mode)) return
    setBusyId(s.sessionId)
    try {
      await SetSessionMode(s.sessionId, mode)
      setList((ls) =>
        ls.map((x) =>
          x.sessionId === s.sessionId
            ? { ...x, mode, planEnabled: mode === 'plan' }
            : x,
        ),
      )
      setToast(`${s.title || s.sessionId} → ${mode}（重启或重新恢复该会话后生效）`)
    } catch (e) {
      setToast(`改写失败: ${String(e)}`)
    } finally {
      setBusyId('')
    }
  }

  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head">
          <div>
            <div className="modal-title">会话模式覆盖</div>
            <div className="modal-sub">改写 ZCode 持久化执行状态（runtime/execution_state）</div>
          </div>
          <button className="modal-close" onClick={onClose}>
            ✕
          </button>
        </header>

        <div className="modal-warn">
          ⚠️ 改写的是数据库里的持久化状态：<b>该会话下次恢复或重启 ZCode 后才生效</b>，
          正在运行的实例不受影响。之后你若在客户端选择器里切模式，会覆盖这里的改动。
          每次改写前自动备份原值到 ~/.zcode/v2/backups/subagent-control/sessionmode/。
        </div>

        <input
          className="modal-search"
          placeholder="搜索标题 / 会话 ID / 当前模式…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />

        <div className="modal-list">
          {filtered.map((s) => (
            <div key={s.sessionId} className={`sm-row${busyId === s.sessionId ? ' busy' : ''}`}>
              <div className="sm-info">
                <div className="sm-title">
                  <span className={`sm-kind ${s.isSubagent ? 'sub' : 'main'}`}>
                    {s.isSubagent ? '子' : '主'}
                  </span>
                  <span className="sm-name">{s.title || '(无标题)'}</span>
                </div>
                <div className="sm-sub">
                  {s.sessionId.slice(0, 34)}… · 更新于 {s.updatedAt}
                </div>
              </div>
              <div className="sm-current">
                <span className={`badge ${modeBadge(s.mode)}`}>{s.mode}</span>
                {s.planEnabled && <span className="sm-plan">plan 开</span>}
              </div>
              <div className="sm-actions">
                {WRITABLE.map((m) => (
                  <button
                    key={m}
                    className={`sm-btn ${s.mode === m ? 'on' : ''}`}
                    disabled={busyId === s.sessionId || s.mode === m}
                    title={`下次恢复后以 ${m} 模式启动`}
                    onClick={() => setMode(s, m)}
                  >
                    {m}
                  </button>
                ))}
              </div>
            </div>
          ))}
          {filtered.length === 0 && <div className="modal-empty">没有匹配的会话</div>}
        </div>

        {toast && <div className="modal-toast">{toast}</div>}
      </div>
    </div>
  )
}
