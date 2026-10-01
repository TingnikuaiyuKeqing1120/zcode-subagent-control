import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { SetPermissionMode } from '../../bindings/zcode-subagent-control/api'

// 与 ZCode 客户端模式选择器对齐的四个主档位（图标/标题/描述/勾选）
export const QUICK_MODES = [
  { value: 'plan', label: '计划模式', desc: '编辑前先出计划。', icon: '💡', tier: 'plan' },
  { value: 'default', label: '变更前确认', desc: '改文件前先问我。', icon: '✋', tier: 'build' },
  { value: 'edit', label: '自动编辑', desc: '自动编辑文件。', icon: '✎', tier: 'edit' },
  { value: 'yolo', label: '完全访问', desc: '减少确认次数。', icon: '⚠️', tier: 'yolo' },
] as const

export function modeInfo(mode: string | null) {
  return QUICK_MODES.find((m) => m.value === mode) ?? QUICK_MODES[1]
}

/**
 * ModePill —— 图中样式的模式切换器：触发器是带图标和当前档位的 pill，
 * 点击弹出四档菜单（图标 + 标题 + 描述 + 右侧勾选）。
 */
export function ModePill({
  mode,
  onChange,
  small = false,
  disabled = false,
  allowExtended = false,
}: {
  mode: string | null
  onChange?: (mode: string) => void
  small?: boolean
  disabled?: boolean
  /** 为 true 时菜单额外列出十种完整模式（含 acceptEdits/bypassPermissions 等别名档） */
  allowExtended?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const btnRef = useRef<HTMLButtonElement>(null)
  const popRef = useRef<HTMLDivElement>(null)
  const info = modeInfo(mode)

  useEffect(() => {
    if (!open) return
    const onDocDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (btnRef.current?.contains(t) || popRef.current?.contains(t)) return
      setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDocDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDocDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  // 打开时按触发器位置定位；空间不足向上翻转，横向夹在视口内
  useLayoutEffect(() => {
    if (!open || !btnRef.current) return
    const r = btnRef.current.getBoundingClientRect()
    const h = popRef.current?.offsetHeight ?? 250
    const w = 264
    const up = r.bottom + h + 8 > window.innerHeight
    setPos({
      x: Math.max(8, Math.min(r.left, window.innerWidth - w - 8)),
      y: up ? Math.max(8, r.top - h - 6) : r.bottom + 6,
    })
  }, [open])

  // 首帧用估算高度定位，弹层真实渲染后再按实际高度夹一次底边
  useLayoutEffect(() => {
    if (!open || !pos || !popRef.current) return
    const bottom = pos.y + popRef.current.offsetHeight
    if (bottom > window.innerHeight - 8) {
      setPos({ ...pos, y: Math.max(8, window.innerHeight - 8 - popRef.current.offsetHeight) })
    }
  }, [open, pos])

  const pick = (value: string) => {
    setOpen(false)
    onChange?.(value)
  }

  const extended = allowExtended
    ? ([
        { value: 'bypassPermissions', label: '绕过权限', desc: '跳过权限检查（与 yolo 同档）。', icon: '🚫', tier: 'yolo' },
        { value: 'dontAsk', label: '无需询问', desc: '跳过常规确认（归 yolo 档）。', icon: '🔇', tier: 'yolo' },
        { value: 'acceptEdits', label: '接受编辑', desc: '自动接受文件编辑——客户端归 build 档，仍会逐次确认。', icon: '📝', tier: 'build' },
        { value: 'autoEdit', label: '自动编辑', desc: '自动编辑（客户端归 build 档）。', icon: '✏️', tier: 'build' },
        { value: 'build', label: '构建模式', desc: '改文件前先问我。', icon: '🔨', tier: 'build' },
        {
          value: 'auto',
          label: '自动（勿选）',
          desc: '保留未实现：客户端会拒绝该角色的全部工具调用（mode.auto.unimplemented）。',
          icon: '⛔',
          tier: 'auto',
          danger: true,
        },
      ] as const)
    : []

  return (
    <>
      <button
        ref={btnRef}
        className={`mode-pill ${small ? 'sm' : ''} tier-${info.tier}${disabled ? ' disabled' : ''}`}
        disabled={disabled}
        onClick={(e) => {
          e.stopPropagation()
          setOpen((v) => !v)
        }}
        title={`当前权限模式：${info.label}（点击切换）`}
      >
        <span className="mp-icon">{info.icon}</span>
        <span className="mp-label">{info.label}</span>
        <span className="mp-caret">▾</span>
      </button>
      {open && pos && (
        <div ref={popRef} className="mode-pop" style={{ left: pos.x, top: pos.y }}>
          {[...QUICK_MODES, ...extended].map((m) => (
            <button
              key={m.value}
              className={`mode-pop-item tier-${m.tier}${mode === m.value ? ' on' : ''}${
                'danger' in m && m.danger ? ' danger-item' : ''
              }`}
              title={'danger' in m && m.danger ? '该档位在客户端为保留未实现，选中会导致角色所有工具调用被拒绝' : undefined}
              onClick={(e) => {
                e.stopPropagation()
                if ('danger' in m && m.danger) {
                  const ok = window.confirm(
                    'auto（自动模式）在 ZCode 客户端是"保留未实现"：\n\n' +
                      '任何角色设为 auto，它派发后的全部工具调用都会被直接拒绝\n' +
                      '（错误码 mode.auto.unimplemented），不是弹窗、是报错。\n\n' +
                      '确定仍要设为 auto 吗？',
                  )
                  if (!ok) return
                }
                pick(m.value)
              }}
            >
              <span className="mpi-icon">{m.icon}</span>
              <span className="mpi-text">
                <span className="mpi-title">{m.label}</span>
                <span className="mpi-desc">{m.desc}</span>
              </span>
              <span className="mpi-check">{mode === m.value ? '✓' : ''}</span>
            </button>
          ))}
        </div>
      )}
    </>
  )
}

/**
 * AgentModePill —— 侧边栏/悬浮面板行内用的模式切换 pill：
 * 本地乐观更新 + 调用 API 落盘，失败时回滚并抛出。
 */
export function AgentModePill({
  fileName,
  mode,
  onChanged,
  small = true,
  allowExtended = true,
}: {
  fileName: string
  mode: string | null
  onChanged: (fileName: string, mode: string) => void
  small?: boolean
  allowExtended?: boolean
}) {
  const [busy, setBusy] = useState(false)
  return (
    <ModePill
      mode={mode}
      small={small}
      allowExtended={allowExtended}
      disabled={busy}
      onChange={async (next) => {
        setBusy(true)
        try {
          await SetPermissionMode(fileName, next)
          onChanged(fileName, next)
        } catch (e) {
          alert(`切换失败: ${String(e)}`)
        } finally {
          setBusy(false)
        }
      }}
    />
  )
}
