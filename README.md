# zcode-subagent-control

ZCode 自定义子智能体的独立 GUI 管理工具。管理对象是 `~/.zcode/agents/*.md`
（ZCode 的官方子智能体定义格式：YAML frontmatter + 正文 system prompt）。

ZCode 客户端对此只有设置页里逐个编辑、没有快捷切换权限模式的手段；
本工具把「查看 / 编辑 / 新建 / 删除 / 权限模式一键切换」做成了一个独立小程序，
带系统托盘和悬浮速切面板。

## 功能

- **角色列表**：名称、权限模式分档徽章、模型、工具数；支持按名称/文件名/描述搜索
- **全字段表单**（与客户端 parser/serializer 的字段一一对应）：
  - `name`、`description`、`color`、`fileName`（文件名，可改名）
  - `model`（providerId/modelId，下拉候选来自 `provider_config.json` 的 providerRules +
    `config.json` 的 provider 字典）、`thoughtLevel`（按模型解析推理档位候选）
  - `tools` / `disallowedTools`（按分组的多选 chips + 自由输入）
  - `skills`（来自已安装技能目录的 chips + 自由输入）、`mcpServers`
  - `permissionMode`（十种模式卡片，标注内部归一档位）、`maxTurns`、`background`、`injectAgentsMd`
  - 正文 system prompt 编辑
- **权限模式快捷切换**（与客户端模式选择器同款的弹层交互：图标 + 标题 + 描述 + 勾选）：
  - 侧边栏 / 悬浮面板每个角色行内一个模式 pill，点开即切、即落盘
  - 顶栏「全部 → 完全访问 / 全部 → 变更前确认」批量切
  - 弹层含四个主档（计划模式 / 变更前确认 / 自动编辑 / 完全访问）+ 六个别名档
    （bypassPermissions / dontAsk / acceptEdits / autoEdit / build / auto）
- **防误操作**：单实例锁（重复双击只唤出已有窗口，不开第二进程）；关闭主窗口收回托盘，
  退出只能走托盘菜单；图标与 zcode-dashboard-go 同族不同形（深色圆角方块 +
  三色滑杆 vs 上升柱），托盘/任务栏可区分
- **校验**：保存前分级校验（error 阻止保存，warn 提示），覆盖模式白名单、颜色白名单、
  maxTurns 范围、文件名合法性、model 形式、未知工具名
- **安全**：每次保存/删除前自动备份到 `~/.zcode/v2/backups/subagent-control/`（保留最近 200 份）；
  改名走「备份旧文件 → 写新文件 → 删旧文件」
- **内置智能体**（general-purpose / Explore）只读展示，含 agents-state.json 里的模型覆盖

## 托盘与悬浮速切面板

- 关闭主窗口 = 收回托盘（不退出）；托盘菜单：悬浮面板 / 打开主窗口 /
  全部 → 完全访问 / 全部 → 变更前确认 / 退出
- 单击托盘图标 = 唤出/收起悬浮速切面板（无边框置顶 400×600 小窗，
  `float.html` 独立入口，标题栏可拖拽）
- 悬浮面板内容：两个全局切换按钮 + 搜索框 + 每个角色的模式 pill（即点即切，
  自动备份）+ 「打开主窗口」（切换语义：先收起悬浮面板）
- 主窗口顶栏「◈ 悬浮速切」按钮同样可打开/收起悬浮面板，并显示当前开关状态
- 模式弹层限高并内部滚动（十档长列表可滚轮滚动），按实际高度二次夹取底边
- 主窗口侧边栏同样内嵌模式 pill，与悬浮面板共用同一个 ModePicker 组件

## 构建与运行

依赖：Go 1.25+、Node 20+、[Wails v3 CLI](https://v3.wails.io)。

```bash
# 开发模式（前端热重载，http://localhost:9245）
wails3 dev

# 生产构建 → bin/zcode-subagent-control.exe（单文件，双击即用）
wails3 build
```

```bash
go test ./...        # 后端单测（解析/序列化/备份/保存/校验）
```

## 架构

```
main.go                      Wails 应用装配（主窗口 + 悬浮窗 + 托盘；关闭收回托盘）
api.go                       前端唯一服务：List/Get/Meta/Save/Create/Delete/SetMode/Validate
                             + 悬浮面板窗口控制（FloatPanelVisible/Toggle/Hide/ShowMain）
internal/agentfile/          角色文件的解析、序列化、备份、增删
  agentfile.go               写格式严格复刻客户端 generateSubagentMarkdown（字段顺序、
                             两空格缩进列表、与客户端一致的引号判定）
internal/zconfig/            读客户端配置：provider/模型、已装技能、内置智能体覆盖
frontend/
  index.html / App.tsx       主窗口（列表 + 全字段表单）
  float.html / FloatPanel.tsx 悬浮速切面板（vite 多入口）
  components/ModePicker.tsx  模式 pill + 弹层（主侧边栏与悬浮面板共用）

**写格式保真**：`Marshal()` 的输出与 ZCode 自己的 writer 逐字段对齐——
客户端读回本工具写的文件不会丢字段、不会改语义。frontmatter 里无法识别的
未知键会原样保留（`unknownLists` / `unknownLines`）。

**不碰凭证**：`zconfig` 只读 provider 的 id/名称/模型清单，
不读取、不存储、不外传任何 API Key。

## 已知限制 / 踩坑记录

- **项目根目录可能出现残留 exe，不要双击它**：`wails3 dev` 和 `go build .`（不带 -o）都会把
  开发版二进制输出到项目根目录——它是控制台程序（PE subsystem=3，双击跳命令行）、
  且没有 exe 图标资源（显示 Windows 默认图标）。正式包只在 `bin/`（GUI、图标完整）。
  根目录 exe 已加入 .gitignore；日常只从 `bin/` 启动。
- **`wails3 dev` 会用开发版二进制覆盖 `bin/`**：开发版不带 `-H windowsgui`，
  表现为双击 exe 弹出控制台窗口、体积变大。交付/日常使用前务必重跑一次
  `wails3 build` 并确认 PE subsystem=2（`python` 读 PE 头 pe+0x5C 处为 2）。
- **`application.Options.Name` 必须是 ASCII**。用中文名（如「ZCode 子智能体控制台」）
  会导致 WebView2 用户数据目录创建失败，症状是启动即退、日志
  `error creating controller with 800700aa`。中文只放窗口 Title。已修复。
- **重新生成绑定会改变 interface 形态**（unknownLists 一时带 null 一时不带）。
  Draft 类型直接 `NonNullable<AgentModel['unknownLists']>` 复用绑定类型，免疫重新生成。
- 服务方法不要暴露接口类型参数（如 `SetWindows(main, float application.Window)`），
  绑定生成器会告警且生成的 TS 不可用；窗口引用由 main 直接赋字段。
- 内置智能体（general-purpose / Explore）没有文件载体，本工具只读不停改；
  `agents-state.json` 的启停切换未做（其 agent id 生成规则未逆向确认）。
- 备份名到微秒：早先用秒级时间戳，同一会话内的连续保存会互相覆盖。
- dev 模式下若从别的浏览器打开 dev server 页面，Wails 运行时桥不存在，
  数据调用必然失败——这是预期，不代表应用问题。

## 与官方能力对照

客户端「设置 → Subagents」表单的字段（name/description/color/model/thoughtLevel/
tools/disallowedTools/skills/permissionMode/maxTurns/background/injectAgentsMd/
mcpServers）在本工具中全部可编辑；官方特有的「插件贡献的 agents」「内置模型覆盖」
只做只读展示。
