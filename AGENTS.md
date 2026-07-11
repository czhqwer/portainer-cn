# Portainer.CN 二开协作指南

本仓库基于 [Portainer Community Edition](https://github.com/portainer/portainer) 二次开发，目标是提供更适合日常自用和中文环境的容器、Kubernetes、数据库与主机管理体验。

## 项目定位

- 上游项目：Portainer Community Edition。
- 当前方向：中文化、隐藏商业版入口、数据库工作台、Agent 本地开发与部署体验优化。
- 技术栈：前端 React / AngularJS / TypeScript，后端 Go，包管理使用 PNPM。
- 默认开发入口：前端 `http://localhost:8999`，后端 `http://localhost:9000`。

## 工作原则

- 修改前先阅读现有实现，优先沿用项目已有结构、组件、Hook、API 风格和错误处理方式。
- 只改与当前需求直接相关的代码，避免顺手重构无关模块。
- 不要回滚用户或其他协作者已有修改；遇到冲突时先确认差异来源，再基于现状继续。
- 前后端协议、接口字段、持久化模型必须保持兼容，除非需求明确要求破坏性变更。
- 页面体验要优先考虑中文用户的实际操作路径，避免新增入口造成误解。
- 私有化微服务部署平台改造过程中，如果遇到需求、边界、优先级或实现取舍不明确，必须先查阅 `docs/私有化微服务部署平台产品需求文档.md`；如果 PRD 中仍没有明确答案，应暂停相关改造并向用户确认。用户给出明确答复后，先询问是否需要同步更新 PRD，并按用户答复完成必要文档更新后，再继续对应改造。

## 注释规范

- 方法、函数或复杂逻辑块上必须添加中文注释。
- 中文注释应说明“为什么这样做”以及“这个逻辑解决什么场景”，不要只复述代码。
- 适合添加中文注释的场景：
  - 新增或改造后端 handler、service、schema/query 执行方法。
  - 新增或改造前端核心交互函数，例如连接自动填充、SQL 执行保护、主题适配、历史记录处理。
  - 涉及兼容旧数据、跨 Docker/Kubernetes 环境、Agent、本地直连、权限判断的逻辑。
  - 会影响用户数据、数据库写入、容器/节点连接、部署行为的逻辑。
- 不需要给简单的 getter、纯展示组件、直观变量赋值添加注释。
- 注释示例：

```go
// 环境级数据库连接即使来源于容器，也只把 ContainerID 作为自动填充来源；
// 实际查询始终使用 Host/Port 直连，避免误走容器级 WebSocket 代理。
func (handler *Handler) databaseConnectionSchema(...) ...
```

```ts
// 当 Portainer 通过本地 Agent 访问 Docker 时，容器内部 IP 通常不可从本机直连；
// 此时优先使用宿主机发布端口，保证本地开发数据库连接可用。
function suggestedConnectionTarget(...) ...
```

## 前端开发约定

- 优先使用现有组件：`@@/buttons`、`@@/form-components`、`@@/Alert`、`@@/Icon` 等。
- 新增用户可见文案必须接入国际化，至少补齐中文和英文。
- 中文模式下不要遗留硬编码英文菜单、按钮、表单标签、错误提示。
- 数据库页面、SQL 编辑区、库表树和结果表格必须兼容深色与浅色主题。
- 交互控件尽量清晰紧凑：图标按钮需要有明确含义，复杂操作需要提示或二次确认。
- 不引入大型前端依赖，除非需求明确且已有方案无法满足。

## 后端开发约定

- 后端遵循 Portainer 现有 handler / datastore / service 风格。
- 新增 API 必须有清晰的鉴权边界，默认管理员可用的功能不要放宽权限。
- 数据库连接相关逻辑要区分“连接来源”和“实际访问方式”：
  - `ContainerId` 可作为前端自动填充、展示和兼容信息。
  - 环境级数据库连接应通过 Portainer 后端使用 `Host` / `Port` 直连。
  - 容器级旧入口仅做兼容，不应成为新数据库入口的依赖。
- 涉及密码保存时，继续遵守 Portainer 数据库加密限制。
- 执行数据库写操作时，要保留二次确认、影响行数预览和超时保护。

## 数据库工作台约定

- 左侧菜单中的数据库入口属于环境级入口，不挂在某个容器详情页下。
- Docker/Kubernetes 环境都应通过统一环境级 API 管理连接。
- 选择容器时允许自动填充 Host/Port，但用户必须可以手动覆盖。
- 本地 Agent 场景优先使用宿主机发布端口，例如 `127.0.0.1:3307`。
- SELECT 查询默认限制结果行数，避免误拉大表。
- UPDATE/DELETE 需要先预执行并回滚，展示影响行数后再由用户确认正式执行。
- 查询历史先保存在浏览器本地，按用户、环境、连接隔离。

## Apifox CLI 规则

- 使用 Apifox CLI 前，必须读取当前项目根目录下的 `.apifox/settings.json`。
- 使用其中的 `projectId` 作为当前项目的 Apifox 项目 ID。
- 执行项目资源命令时，自动附加 `--project <projectId>`。
- 不允许猜测或使用其他项目的 projectId。
- 如果 `.apifox/settings.json` 不存在，先执行 `apifox project list` 查找项目，不得使用写死的默认项目。
- 访问令牌不得写入仓库、AGENTS.md 或 `.apifox/settings.json`。

## 常用命令

```bash
# 前端开发
pnpm dev
pnpm typecheck
pnpm test
pnpm build

# 后端测试
go test ./api/http/handler/endpoints
go test ./api/http/handler/docker/containers

# 完整构建
make build
make build-client
make build-server
make build-image
```

Windows 本地 Go 如果不在 PATH 中，可使用本机 Go 绝对路径，例如：

```powershell
E:\develop\Go\bin\go.exe test ./api/http/handler/endpoints
E:\develop\Go\bin\gofmt.exe -w <file.go>
```

如果系统 Go build cache 权限异常，可临时把缓存放到项目内：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
```

## 验证要求

- 前端 TypeScript 改动后至少运行 `pnpm typecheck`。
- 后端 handler/service 改动后运行相关 `go test`。
- 改动数据库工作台时，手工验证：
  - 新增连接、测试连接、保存连接、读取库表树。
  - SELECT 查询、结果表格、复制、执行历史。
  - 深色/浅色主题。
  - 中文/英文切换。
  - 本地 Agent + 发布端口连接场景。
- 修改部署、Agent、README 或 Docker 相关内容后，尽量验证本地启动和容器部署路径。

## 提交约定

- 提交信息使用中文。
- 提交前检查 `git status`，不要把无关文件或临时文件放进提交。
- 不要提交本地数据目录、构建缓存、日志或个人环境配置。
- 如果本轮包含用户未要求提交的实验文件，应保留在工作区或删除，不能混入正式提交。
