import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  HideFloatPanel,
  ListAgents,
  Meta,
  SetAllPermissionModes,
  ShowMainWindow,
} from '../bindings/zcode-subagent-control/api'
import type { Summary } from '../bindings/zcode-subagent-control/internal/agentfile/models'
import { AgentModePill } from './components/ModePicker'

/**
 * FloatPanel —— 托盘唤出的悬浮速切面板。
 * 每个角色一行 + 图中样式的模式切换 pill，切换即落盘（自动备份）。
 */
export default function FloatPanel() {
  const [list, setList] = useState<Summary[]>([])
  const [filter, setFilter] = useState('')
  const [toast, setToast] = useState<string | null>(null)
  const [count, setCount] = useState(0)

  const reload = useCallback(async () => {
    try {
      setList((await ListAgents()) ?? [])
    } catch (e) {
      setToast(`读取失败: ${String(e)}`)
    }
  }, [])

  useEffect(() => {
    ;(async () => {
      try {
        await Meta()
      } catch {
        /* 选项拉取失败不影响速切 */
      }
      await reload()
    })()
  }, [reload])

  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => setToast(null), 2200)
    return () => clearTimeout(t)
  }, [toast, count])

  const onChanged = useCallback((fileName: string, mode: string) => {
    setList((ls) => ls.map((x) => (x.fileName === fileName ? { ...x, permissionMode: mode } : x)))
    setCount((c) => c + 1)
    setToast(`${fileName} → ${mode}（对新派生的子智能体生效）`)
  }, [])

  const bulk = async (mode: string) => {
    try {
      const n = await SetAllPermissionModes(mode)
      await reload()
      setCount((c) => c + 1)
      setToast(`已将 ${n} 个角色切换为 ${mode}`)
    } catch (e) {
      setToast(`切换失败: ${String(e)}`)
    }
  }

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q) return list
    return list.filter(
      (x) =>
        x.fileName.toLowerCase().includes(q) ||
        x.name.toLowerCase().includes(q) ||
        (x.description ?? '').toLowerCase().includes(q),
    )
  }, [list, filter])

  return (
    <div className="float-app">
      <header className="float-header drag">
        <span className="float-title">◈ 子智能体速切</span>
        <div className="float-header-actions no-drag">
          <button title="打开主窗口" onClick={() => ShowMainWindow()}>
            ⤢
          </button>
          <button title="收起面板" onClick={() => HideFloatPanel()}>
            ✕
          </button>
        </div>
      </header>

      <div className="float-global no-drag">
        <button className="fbtn danger" onClick={() => bulk('yolo')}>
          全部 → 完全访问
        </button>
        <button className="fbtn warn" onClick={() => bulk('default')}>
          全部 → 变更前确认
        </button>
      </div>

      <input
        className="float-search no-drag"
        placeholder="搜索角色…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />

      <div className="float-list">
        {filtered.map((x) => (
          <div key={x.fileName} className="float-row">
            <div className="fr-info">
              <span className="fr-name">{x.name || x.fileName}</span>
              <span className="fr-sub">
                {x.fileName}.md · {x.model || '默认模型'} · {x.toolCount} 工具
              </span>
            </div>
            <AgentModePill fileName={x.fileName} mode={x.permissionMode} onChanged={onChanged} />
          </div>
        ))}
        {filtered.length === 0 && <div className="float-empty">没有匹配的角色</div>}
      </div>

      <footer className="float-foot">托盘图标可随时唤出本面板</footer>

      {toast && <div className="float-toast">{toast}</div>}
    </div>
  )
}
