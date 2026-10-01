import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  DeleteAgent,
  FloatPanelVisible,
  GetAgent,
  ListAgents,
  Meta,
  SaveAgent,
  SetAllPermissionModes,
  SetPermissionMode,
  ThoughtLevels,
  ToggleFloatPanel,
  Validate,
} from '../bindings/zcode-subagent-control/api'
import type { Agent as AgentModel } from '../bindings/zcode-subagent-control/internal/agentfile/models'
import type { Summary } from '../bindings/zcode-subagent-control/internal/agentfile/models'
import type { Meta as MetaModel } from '../bindings/zcode-subagent-control/internal/zconfig/models'
import type { Issue } from '../bindings/zcode-subagent-control/models'
import { AgentModePill, ModePill } from './components/ModePicker'

// ---------- 本地草案类型（与 Go 的 agentfile.Agent JSON 结构一一对应） ----------

interface Draft {
  fileName: string
  name: string
  description: string
  color: string | null
  model: string | null
  thoughtLevel: string | null
  tools: string[]
  disallowedTools: string[]
  skills: string[]
  mcpServers: string[]
  permissionMode: string | null
  maxTurns: number | null
  background: boolean | null
  injectAgentsMd: boolean | null
  body: string
  // 直接复用绑定生成的类型：重新生成绑定时无需手动同步
  unknownLists?: NonNullable<AgentModel['unknownLists']>
  unknownLines?: AgentModel['unknownLines']
}

const blankDraft = (): Draft => ({
  fileName: '',
  name: '',
  description: '',
  color: null,
  model: null,
  thoughtLevel: null,
  tools: [],
  disallowedTools: [],
  skills: [],
  mcpServers: [],
  permissionMode: 'default',
  maxTurns: null,
  background: null,
  injectAgentsMd: true,
  body: '',
})

const s = (v: string | null | undefined): string => (v ?? '')
const arr = (v: string[] | null | undefined): string[] => v ?? []

function toDraft(a: AgentModel): Draft {
  const d = blankDraft()
  d.fileName = a.fileName ?? ''
  d.name = a.name ?? ''
  d.description = a.description ?? ''
  d.color = a.color ?? null
  d.model = a.model ?? null
  d.thoughtLevel = a.thoughtLevel ?? null
  d.tools = arr(a.tools)
  d.disallowedTools = arr(a.disallowedTools)
  d.skills = arr(a.skills)
  d.mcpServers = arr(a.mcpServers)
  d.permissionMode = a.permissionMode ?? null
  d.maxTurns = a.maxTurns ?? null
  d.background = a.background ?? null
  d.injectAgentsMd = a.injectAgentsMd ?? null
  d.body = a.body ?? ''
  d.unknownLists = a.unknownLists ?? undefined
  d.unknownLines = a.unknownLines ?? undefined
  return d
}

function toAgent(d: Draft): AgentModel {
  const ag: AgentModel = {
    name: d.name,
    description: d.description,
    color: d.color,
    model: d.model,
    thoughtLevel: d.thoughtLevel,
    tools: d.tools,
    disallowedTools: d.disallowedTools,
    skills: d.skills,
    permissionMode: d.permissionMode,
    maxTurns: d.maxTurns,
    background: d.background,
    injectAgentsMd: d.injectAgentsMd,
    mcpServers: d.mcpServers,
    body: d.body,
    unknownLists: d.unknownLists,
    unknownLines: d.unknownLines,
    fileName: d.fileName,
    filePath: '',
  }
  return ag
}

// 权限模式分档的展示色
const tierClass = (mode: string | null): string => {
  switch (mode) {
    case 'yolo':
    case 'bypassPermissions':
    case 'dontAsk':
      return 'tier-yolo'
    case 'edit':
      return 'tier-edit'
    case 'plan':
      return 'tier-plan'
    case 'auto':
      return 'tier-auto'
    default:
      return 'tier-build'
  }
}

const toggleIn = (list: string[], v: string): string[] =>
  list.includes(v) ? list.filter((x) => x !== v) : [...list, v]

function App() {
  const [meta, setMeta] = useState<MetaModel | null>(null)
  const [list, setList] = useState<Summary[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [originalFileName, setOriginalFileName] = useState('')
  const [loadedJSON, setLoadedJSON] = useState('')
  const [issues, setIssues] = useState<Issue[]>([])
  const [thoughtLevels, setThoughtLevels] = useState<string[]>([])
  const [filter, setFilter] = useState('')
  const [floatVisible, setFloatVisible] = useState(false)
  const [toast, setToast] = useState<{ kind: 'ok' | 'err'; msg: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const flash = useCallback((kind: 'ok' | 'err', msg: string) => {
    setToast({ kind, msg })
    if (toastTimer.current) clearTimeout(toastTimer.current)
    toastTimer.current = setTimeout(() => setToast(null), 3200)
  }, [])

  const reloadList = useCallback(async () => {
    try {
      setList((await ListAgents()) ?? [])
    } catch (e) {
      flash('err', `读取角色列表失败: ${String(e)}`)
    }
  }, [flash])

  useEffect(() => {
    ;(async () => {
      try {
        setMeta(await Meta())
      } catch (e) {
        flash('err', `读取配置失败: ${String(e)}`)
      }
      try {
        setFloatVisible(await FloatPanelVisible())
      } catch {
        /* 悬浮面板状态获取失败不阻塞主界面 */
      }
      await reloadList()
    })()
  }, [flash, reloadList])

  const openAgent = useCallback(
    async (fileName: string) => {
      setBusy(true)
      try {
        const a = await GetAgent(fileName)
        if (!a) throw new Error('文件不存在')
        const d = toDraft(a)
        setSelected(fileName)
        setDraft(d)
        setOriginalFileName(fileName)
        setLoadedJSON(JSON.stringify(d))
        setIssues([])
        if (d.model) {
          try {
            setThoughtLevels((await ThoughtLevels(d.model)) ?? [])
          } catch {
            setThoughtLevels([])
          }
        } else {
          setThoughtLevels([])
        }
      } catch (e) {
        flash('err', `打开角色失败: ${String(e)}`)
      } finally {
        setBusy(false)
      }
    },
    [flash],
  )

  const patch = useCallback((p: Partial<Draft>) => {
    setDraft((d) => (d ? { ...d, ...p } : d))
  }, [])

  const dirty = useMemo(
    () => draft !== null && JSON.stringify(draft) !== loadedJSON,
    [draft, loadedJSON],
  )

  // 保存前实时校验（防抖 300ms）
  useEffect(() => {
    if (!draft || !dirty) {
      setIssues([])
      return
    }
    const t = setTimeout(() => {
      Validate(toAgent(draft))
        .then((v) => setIssues(v ?? []))
        .catch(() => setIssues([]))
    }, 300)
    return () => clearTimeout(t)
  }, [draft, dirty])

  const save = async () => {
    if (!draft) return
    try {
      const found = (await Validate(toAgent(draft))) ?? []
      setIssues(found)
      if (found.some((i) => i.level === 'error')) {
        flash('err', '有错误级问题，修复后再保存')
        return
      }
      await SaveAgent({ agent: toAgent(draft), originalFileName })
      await reloadList()
      setSelected(draft.fileName)
      setOriginalFileName(draft.fileName)
      setLoadedJSON(JSON.stringify(draft))
      flash('ok', `已保存 ${draft.fileName}.md（旧版本已自动备份）`)
    } catch (e) {
      flash('err', `保存失败: ${String(e)}`)
    }
  }

  const create = () => {
    const d = blankDraft()
    setSelected(null)
    setDraft(d)
    setOriginalFileName('')
    setLoadedJSON(JSON.stringify(d))
    setIssues([])
    setThoughtLevels([])
  }

  const remove = async () => {
    if (!selected) return
    if (!window.confirm(`删除角色 ${selected}.md？删除前会自动备份到 ~/.zcode/v2/backups/。`)) return
    try {
      await DeleteAgent(selected)
      await reloadList()
      setSelected(null)
      setDraft(null)
      flash('ok', `已删除 ${selected}.md`)
    } catch (e) {
      flash('err', `删除失败: ${String(e)}`)
    }
  }

  const switchMode = async (mode: string, targets: 'one' | 'all') => {
    try {
      if (targets === 'all') {
        const n = await SetAllPermissionModes(mode)
        await reloadList()
        if (draft && selected) await openAgent(selected)
        flash('ok', `已将 ${n} 个角色切换为 ${mode}`)
      } else if (selected) {
        const sum = await SetPermissionMode(selected, mode)
        await reloadList()
        if (draft) patch({ permissionMode: sum.permissionMode || mode })
        flash('ok', `${selected} → ${mode}`)
      }
    } catch (e) {
      flash('err', `切换失败: ${String(e)}`)
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

  const errors = issues.filter((i) => i.level === 'error')
  const warnings = issues.filter((i) => i.level === 'warn')

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <span className="logo">◈</span>
          <div>
            <div className="title">ZCode 子智能体控制台</div>
            <div className="subtitle">{meta?.agentsDir ?? '~/.zcode/agents'}</div>
          </div>
        </div>
        <div className="topbar-actions">
          <button
            className="btn btn-ghost"
            title="打开/收起悬浮速切面板（托盘图标也可唤出）"
            onClick={async () => {
              try {
                setFloatVisible(await ToggleFloatPanel())
              } catch (e) {
                flash('err', `切换悬浮面板失败: ${String(e)}`)
              }
            }}
          >
            ◈ 悬浮速切{floatVisible ? '（已开）' : ''}
          </button>
          <button className="btn btn-danger" onClick={() => switchMode('yolo', 'all')}>
            全部 → 完全访问
          </button>
          <button className="btn btn-warn" onClick={() => switchMode('default', 'all')}>
            全部 → 变更前确认
          </button>
          <button className="btn btn-primary" onClick={create}>
            ＋ 新建角色
          </button>
        </div>
      </header>

      <div className="body">
        <aside className="sidebar">
          <input
            className="search"
            placeholder="搜索角色名 / 文件名 / 描述…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <div className="agent-list">
            {filtered.map((x) => (
              <div
                key={x.fileName}
                className={`agent-item${selected === x.fileName ? ' active' : ''}`}
                role="button"
                tabIndex={0}
                onClick={() => openAgent(x.fileName)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') openAgent(x.fileName)
                }}
              >
                <div className="agent-item-head">
                  <span className="agent-name">{x.name || x.fileName}</span>
                  <AgentModePill
                    fileName={x.fileName}
                    mode={x.permissionMode}
                    onChanged={(fn, mode) => {
                      setList((ls) =>
                        ls.map((y) => (y.fileName === fn ? { ...y, permissionMode: mode } : y)),
                      )
                      if (draft && selected === fn) patch({ permissionMode: mode })
                      flash('ok', `${fn} → ${mode}（对新派生的子智能体生效）`)
                    }}
                  />
                </div>
                <div className="agent-item-sub">
                  {x.fileName}.md · {x.model || '默认模型'} · {x.toolCount} 工具
                </div>
              </div>
            ))}
            {filtered.length === 0 && <div className="empty">没有匹配的角色</div>}
          </div>
          {meta && (meta.builtIns ?? []).length > 0 && (
            <div className="builtins">
              <div className="builtins-title">内置智能体（客户端管理，此处只读）</div>
              {(meta.builtIns ?? []).map((b) => (
                <div key={b.name} className="builtin-item">
                  <span>{b.name}</span>
                  <span className="builtin-model">{b.model || '默认'}</span>
                </div>
              ))}
            </div>
          )}
        </aside>

        <main className="main">
          {!draft && <div className="empty hero">左侧选择一个角色，或点「新建角色」</div>}
          {draft && (
            <div className="form">
              <section className="card">
                <h3>权限模式</h3>
                <div className="mode-grid">
                  {(meta?.modes ?? []).map((m) => (
                    <button
                      key={m.value}
                      className={`mode-card ${tierClass(m.value)}${draft.permissionMode === m.value ? ' on' : ''}`}
                      onClick={() => patch({ permissionMode: m.value })}
                    >
                      <div className="mode-label">{m.label}</div>
                      <div className="mode-value">{m.value}</div>
                      <div className="mode-desc">{m.desc}</div>
                    </button>
                  ))}
                </div>
                {selected && (
                  <div className="quick-row">
                    <span>快捷切换（立即保存）：</span>
                    <ModePill
                      mode={draft.permissionMode}
                      allowExtended
                      onChange={(next) => switchMode(next, 'one')}
                    />
                    <span className="quick-hint">弹层内四档 + 六个别名档，勾选为当前档</span>
                  </div>
                )}
              </section>

              <section className="card">
                <h3>基本信息</h3>
                <div className="grid2">
                  <label>
                    <span>文件名</span>
                    <input
                      value={draft.fileName}
                      onChange={(e) => patch({ fileName: e.target.value.trim() })}
                      placeholder="如 coder"
                    />
                    <em>{draft.fileName ? `${draft.fileName}.md` : '新建时必填'}</em>
                  </label>
                  <label>
                    <span>name</span>
                    <input value={draft.name} onChange={(e) => patch({ name: e.target.value })} />
                  </label>
                </div>
                <label>
                  <span>description（展示给模型的简短说明）</span>
                  <textarea
                    rows={2}
                    value={draft.description}
                    onChange={(e) => patch({ description: e.target.value })}
                  />
                </label>
                <div className="grid2">
                  <label>
                    <span>color</span>
                    <div className="color-row">
                      <select
                        value={draft.color ?? ''}
                        onChange={(e) => patch({ color: e.target.value || null })}
                      >
                        <option value="">未设置</option>
                        {(meta?.colors ?? []).map((c) => (
                          <option key={c} value={c}>
                            {c}
                          </option>
                        ))}
                      </select>
                      {draft.color && <span className={`color-dot c-${draft.color}`} />}
                    </div>
                  </label>
                  <label>
                    <span>maxTurns</span>
                    <input
                      type="number"
                      min={1}
                      max={200}
                      value={draft.maxTurns ?? ''}
                      onChange={(e) =>
                        patch({ maxTurns: e.target.value === '' ? null : Number(e.target.value) })
                      }
                      placeholder="未设置"
                    />
                  </label>
                </div>
              </section>

              <section className="card">
                <h3>模型</h3>
                <div className="grid2">
                  <label>
                    <span>model（providerId/modelId）</span>
                    <input
                      list="model-options"
                      value={s(draft.model)}
                      onChange={(e) => {
                        patch({ model: e.target.value || null })
                        if (e.target.value.includes('/')) {
                          ThoughtLevels(e.target.value)
                            .then((v) => setThoughtLevels(v ?? []))
                            .catch(() => setThoughtLevels([]))
                        }
                      }}
                      placeholder="如 new-provider-3/step-5-preview"
                    />
                    <datalist id="model-options">
                      {(meta?.models ?? []).map((m) => (
                        <option key={m.ref} value={m.ref}>
                          {m.providerName} · {m.model}
                        </option>
                      ))}
                    </datalist>
                  </label>
                  <label>
                    <span>thoughtLevel</span>
                    <input
                      list="tl-options"
                      value={s(draft.thoughtLevel)}
                      onChange={(e) => patch({ thoughtLevel: e.target.value || null })}
                      placeholder="未设置"
                    />
                    <datalist id="tl-options">
                      {(thoughtLevels.length ? thoughtLevels : (meta?.thoughtLevels ?? [])).map((t) => (
                        <option key={t} value={t} />
                      ))}
                    </datalist>
                  </label>
                </div>
              </section>

              <section className="card">
                <h3>工具</h3>
                <div className="tools-cols">
                  <div>
                    <div className="col-title">tools（可用）</div>
                    {(meta?.toolGroups ?? []).map((g) => (
                      <div key={g.name} className="tool-group">
                        <div className="tool-group-name">{g.name}</div>
                        <div className="chips">
                          {(g.tools ?? []).map((t) => (
                            <button
                              key={t}
                              className={`chip${draft.tools.includes(t) ? ' on' : ''}`}
                              onClick={() => patch({ tools: toggleIn(draft.tools, t) })}
                            >
                              {t}
                            </button>
                          ))}
                        </div>
                      </div>
                    ))}
                    <ToolInput
                      label="其他工具"
                      values={draft.tools.filter(
                        (t) => !(meta?.toolGroups ?? []).some((g) => (g.tools ?? []).includes(t)),
                      )}
                      suggestions={(meta?.toolGroups ?? []).flatMap((g) => g.tools ?? [])}
                      onChange={(v) =>
                        patch({
                          tools: [
                            ...draft.tools.filter((t) =>
                              (meta?.toolGroups ?? []).some((g) => (g.tools ?? []).includes(t)),
                            ),
                            ...v,
                          ],
                        })
                      }
                    />
                  </div>
                  <div>
                    <div className="col-title">disallowedTools（禁用）</div>
                    {(meta?.toolGroups ?? []).map((g) => (
                      <div key={g.name} className="tool-group">
                        <div className="tool-group-name">{g.name}</div>
                        <div className="chips">
                          {(g.tools ?? []).map((t) => (
                            <button
                              key={t}
                              className={`chip off${draft.disallowedTools.includes(t) ? ' on' : ''}`}
                              onClick={() =>
                                patch({ disallowedTools: toggleIn(draft.disallowedTools, t) })
                              }
                            >
                              {t}
                            </button>
                          ))}
                        </div>
                      </div>
                    ))}
                    <ToolInput
                      label="其他工具"
                      values={draft.disallowedTools.filter(
                        (t) => !(meta?.toolGroups ?? []).some((g) => (g.tools ?? []).includes(t)),
                      )}
                      suggestions={(meta?.toolGroups ?? []).flatMap((g) => g.tools ?? [])}
                      onChange={(v) =>
                        patch({
                          disallowedTools: [
                            ...draft.disallowedTools.filter((t) =>
                              (meta?.toolGroups ?? []).some((g) => (g.tools ?? []).includes(t)),
                            ),
                            ...v,
                          ],
                        })
                      }
                    />
                  </div>
                </div>
              </section>

              <section className="card">
                <h3>技能与 MCP</h3>
                <div className="tool-group">
                  <div className="tool-group-name">skills（回车添加，来自已安装技能）</div>
                  <div className="chips">
                    {(meta?.skills ?? []).map((k) => (
                      <button
                        key={k}
                        className={`chip${draft.skills.includes(k) ? ' on' : ''}`}
                        onClick={() => patch({ skills: toggleIn(draft.skills, k) })}
                      >
                        {k}
                      </button>
                    ))}
                    {draft.skills
                      .filter((k) => !(meta?.skills ?? []).includes(k))
                      .map((k) => (
                        <span key={k} className="chip on custom">
                          {k}
                          <b
                            onClick={() =>
                              patch({ skills: draft.skills.filter((x) => x !== k) })
                            }
                          >
                            ×
                          </b>
                        </span>
                      ))}
                  </div>
                  <input
                    className="mini-input"
                    list="skill-options"
                    placeholder="输入技能名后回车"
                    onKeyDown={(e) => {
                      if (e.key !== 'Enter') return
                      const v = (e.target as HTMLInputElement).value.trim()
                      if (v && !draft.skills.includes(v)) patch({ skills: [...draft.skills, v] })
                      ;(e.target as HTMLInputElement).value = ''
                    }}
                  />
                  <datalist id="skill-options">
                    {(meta?.skills ?? []).map((k) => (
                      <option key={k} value={k} />
                    ))}
                  </datalist>
                </div>
                <ToolInput
                  label="mcpServers（回车添加）"
                  values={draft.mcpServers}
                  suggestions={[]}
                  onChange={(v) => patch({ mcpServers: v })}
                />
              </section>

              <section className="card">
                <h3>高级</h3>
                <div className="grid2">
                  <label>
                    <span>background（默认后台运行）</span>
                    <select
                      value={draft.background === null ? '' : String(draft.background)}
                      onChange={(e) =>
                        patch({
                          background: e.target.value === '' ? null : e.target.value === 'true',
                        })
                      }
                    >
                      <option value="">未设置</option>
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                  </label>
                  <label>
                    <span>injectAgentsMd（注入 AGENTS.md）</span>
                    <select
                      value={draft.injectAgentsMd === null ? '' : String(draft.injectAgentsMd)}
                      onChange={(e) =>
                        patch({
                          injectAgentsMd:
                            e.target.value === '' ? null : e.target.value === 'true',
                        })
                      }
                    >
                      <option value="">未设置</option>
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                  </label>
                </div>
              </section>

              <section className="card">
                <h3>System prompt（正文）</h3>
                <textarea
                  className="body-input"
                  rows={14}
                  value={draft.body}
                  onChange={(e) => patch({ body: e.target.value })}
                  placeholder="角色指令正文，原样写入 .md 的 frontmatter 之后"
                />
              </section>

              <footer className="actionbar">
                <div className="issues">
                  {errors.map((m) => (
                    <div key={m.message} className="issue error">
                      ✕ {m.message}
                    </div>
                  ))}
                  {warnings.map((m) => (
                    <div key={m.message} className="issue warn">
                      ⚠ {m.message}
                    </div>
                  ))}
                </div>
                <div className="actions">
                  {selected && (
                    <button className="btn btn-danger-ghost" onClick={remove}>
                      删除
                    </button>
                  )}
                  <button className="btn btn-ghost" onClick={() => (selected ? openAgent(selected) : create())}>
                    放弃更改
                  </button>
                  <button className="btn btn-primary" disabled={!dirty || busy} onClick={save}>
                    {dirty ? '保存' : '已保存'}
                  </button>
                </div>
              </footer>
            </div>
          )}
        </main>
      </div>

      {toast && <div className={`toast ${toast.kind}`}>{toast.msg}</div>}
    </div>
  )
}

// 自由输入的工具/服务器列表：chips + 回车添加
function ToolInput({
  label,
  values,
  suggestions,
  onChange,
}: {
  label: string
  values: string[]
  suggestions: string[]
  onChange: (v: string[]) => void
}) {
  const [text, setText] = useState('')
  return (
    <div className="tool-group">
      <div className="tool-group-name">{label}</div>
      <div className="chips">
        {values.map((v) => (
          <span key={v} className="chip on custom">
            {v}
            <b onClick={() => onChange(values.filter((x) => x !== v))}>×</b>
          </span>
        ))}
      </div>
      <input
        className="mini-input"
        list={`sugg-${label}`}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key !== 'Enter') return
          const v = text.trim()
          if (v && !values.includes(v)) onChange([...values, v])
          setText('')
        }}
        placeholder="输入后回车"
      />
      <datalist id={`sugg-${label}`}>
        {suggestions.map((x) => (
          <option key={x} value={x} />
        ))}
      </datalist>
    </div>
  )
}

export default App
