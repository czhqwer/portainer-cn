# 阶段 3 分批实施方案与 Goal 模式提示词

版本：v1.0  
日期：2026-07-12  
关联文档：[阶段2实施进度](./阶段2实施进度.md)、[阶段2验收记录](./阶段2验收记录.md)、[阶段0平台核心模型与后端边界设计](./阶段0平台核心模型与后端边界设计.md)、[私有化微服务部署平台产品需求文档](./私有化微服务部署平台产品需求文档.md)、[功能优先级与工作量排期](./功能优先级与工作量排期.md)

## 1. 总体结论

阶段 3 的版本目标是 V0.5「制品驱动部署」。在阶段 1 的 Docker 单机发布链路与阶段 2 的配置安全、RBAC、审计、人工回滚能力基础上，平台从“登记已有镜像并部署”扩展为“获取受支持制品、校验并包装/导入为镜像、推送到已有私有 registry，再复用既有单机发布链路”。

本阶段只交付以下受支持输入：

- Java 8 Jar；
- 前端 dist 压缩包，支持 SPA 与 MPA/纯静态站点；
- Docker image tar 与 OCI image archive；
- 已配置 MinIO / S3 Compatible 对象存储中的上述制品；
- 已有私有 registry 的标准 tag 推送。

所有阶段 3 输出仍必须是镜像。构建仅指“受控制品包装或镜像导入”，不包含源码编译。最终部署继续使用阶段 1 已验证的 Docker 单机 `ReleaseExecutor`，不得重写其发布、失败恢复、锁、幂等、配置快照或人工回滚边界。

阶段 3 建议拆为 9 批。每批必须先完成批次审计、实现、验证、中文提交，并更新 `docs/阶段3实施进度.md` 后，才能开始下一批。

## 2. 权威范围、前置条件与红线

### 2.1 前置条件

- 阶段 2 的批次 1 至批次 8 已完成；当前证据见 `docs/阶段2实施进度.md`。
- 现有 `PlatformArtifact`、`PlatformArtifactSnapshot`、Release 审计、项目 RBAC、Endpoint 权限交集和 Docker 单机发布链路必须保持兼容。
- 开始批次 2 前，批次 1 必须冻结受支持文件格式、大小上限、存储与 registry 配置模型、受控构建模板、标准 tag 规则、错误 reason、原始制品清理策略和验收矩阵。
- 真实 Docker/registry/MinIO 环境不足时，先完成可本地执行的模型、handler、service、archive 安全和 fake adapter 测试，并在进度文档记录缺少的外部演练；不得以绕过鉴权或模拟生产成功替代真实证据。

### 2.2 阶段 3 必做范围

1. 手动上传与对象存储拉取制品；保存元数据、大小、SHA256、来源和生命周期。
2. 受控 Java 8 Jar 包装镜像、前端 dist 静态镜像、Docker/OCI archive 镜像导入。
3. 推送到用户已配置的 registry，并读取/记录最终 digest 与可追溯性。
4. 原始制品的保留、清理、审计和 Release 快照兼容。
5. 发布向导、制品管理页、发布记录、权限态和中英文文案的 V0.5 闭环。

### 2.3 阶段 3 红线

- 不实现中心 Nginx 网关、域名、证书、网关切流或无损发布承诺；这些属于阶段 4。
- 不实现多 Docker 主机、主机组、跨主机分批发布或中心网关连通性检测；这些属于阶段 5。
- 不实现 Kubernetes、Git、源码仓库拉取、源码编译、任意 Dockerfile 执行、内置 registry、完整 CI/CD、审批流或制品漏洞扫描。
- 不实现 Python 源码包、wheel、Node SSR 或 Node 前端服务构建；Python/SSR 仍只能使用已有镜像或导入镜像 archive。
- 不实现独立阿里云 OSS 适配器；V0.5 仅要求 MinIO / S3 Compatible。阿里云 OSS 只做兼容验证，无法经 S3 endpoint 接入时留给阶段 6 或后续。
- 不把 registry 密码、对象存储 access key / secret key、敏感变量、上传临时路径、签名 URL 或制品内容写入 BoltDB 非加密字段、日志、审计、错误响应、前端 toast 或测试快照。
- 不删除被任一 Release 快照、进行中构建/推送任务或人工回滚仍引用的原始制品。

### 2.4 安全与实现基线

- 上传与对象存储下载必须有项目归属、服务端权限校验、大小上限、超时、SHA256、类型白名单和明确错误 reason。
- archive 处理必须拒绝路径穿越、绝对路径、符号链接/硬链接逃逸、设备文件、异常层级、压缩炸弹和超出上限的解压结果；校验失败时不得产生可部署镜像。
- Java Jar 只允许使用平台维护的 Java 8 基础镜像、固定工作目录和标准启动命令；前端 dist 只允许使用平台维护的静态 Nginx 镜像与受控站点配置。用户不得提交 Dockerfile、构建脚本或任意命令。
- registry 与对象存储凭据复用 Portainer 已有加密能力或安全配置服务；API 响应仅返回脱敏元数据。项目权限仍必须与目标 Endpoint 权限取交集。
- 每次获取、构建、导入、推送、清理和发布都必须写入结构化审计；摘要只记录 ID、文件名、类型、大小、SHA256、来源、镜像引用、digest、状态和 reason，敏感字段只记录字段名。

## 3. 批次总览

| 批次 | 名称 | 核心交付 | 进入条件 | 完成标志 |
| --- | --- | --- | --- | --- |
| 1 | 阶段 3 基线审计与制品契约冻结 | 进度文档、格式/大小/模板/tag/清理契约、API 草案和风险矩阵 | 阶段 2 完成 | docs 与红线可执行且验收用例明确 |
| 2 | 制品与存储配置模型、dataservice | Artifact 扩展、对象存储配置、构建任务/生命周期、迁移和测试 | 批次 1 完成 | 模型、迁移、Export/Import 和权限字段测试通过 |
| 3 | 手动上传与安全校验 | 受限上传、临时文件管理、类型识别、SHA256、元数据和审计 | 批次 2 完成 | Jar/dist/archive 上传可安全保存或拒绝 |
| 4 | MinIO / S3 Compatible 制品拉取 | 存储配置、对象选择/拉取、hash 校验、脱敏和审计 | 批次 3 完成 | 可从 S3 Compatible 读取受支持制品 |
| 5 | Docker tar / OCI archive 导入 | archive 校验、受控导入、镜像元信息与可追溯性 | 批次 4 完成 | archive 可变为本地候选镜像，异常 archive 被拒绝 |
| 6 | Java 8 Jar 受控包装镜像 | Java 模板、运行参数、构建执行器、镜像快照和测试 | 批次 5 完成 | Jar 可包装为可推送镜像，禁止任意构建命令 |
| 7 | 前端 dist 受控包装镜像 | SPA/MPA 模式、入口校验、受控静态镜像与配置 | 批次 6 完成 | dist 可包装为静态镜像，非法包不产生镜像 |
| 8 | registry 推送、发布接入与原始制品清理 | 标准 tag、digest、发布预检、保留/清理和审计 | 批次 7 完成 | 生成镜像推送已有 registry 后可复用单机发布 |
| 9 | 前端整合与 V0.5 验收 | 制品管理/向导、权限态、状态记录、i18n 和验收文档 | 批次 8 完成 | 阶段 3 自动化与手工验收记录完整 |

## 4. 分批实施方案

### 4.1 批次 1：阶段 3 基线审计与制品契约冻结

目标：在编码前冻结 V0.5 制品交付的安全边界和接口契约，避免把阶段 4/5 或源码构建提前纳入。

范围：

- 新增 `docs/阶段3实施进度.md`，记录批次、提交、验证、风险和准入结论。
- 审计阶段 2 的 Artifact、Release、审计、RBAC、加密、Docker 执行器、前端路由和对象存储/registry 现有能力。
- 冻结受支持类型 `.jar`、dist `.zip`、Docker image tar、OCI archive；明确文件/解压大小上限、SHA256 规则、类型判定和拒绝 reason。
- 冻结 Java 8 与静态站点受控模板、标准 tag 命名、registry digest 读取、原始制品保留/清理前提与 Release 快照规则。
- 输出上传、对象存储、导入、构建、推送、清理的 API 草案、状态流和验收矩阵。

不做：不改业务代码；不接入真实构建、推送、对象存储；不改变阶段 2 RBAC 或回滚规则。

依赖：阶段 2 进度、验收记录和现有 Docker/registry 边界。

交付物：`docs/阶段3实施进度.md`、本文件的实施契约补充。

验收标准：文档明确 V0.5 范围、受支持输入、可执行红线、敏感数据边界、阶段 4/5 排除项、每类制品的验收与失败路径。

建议验证：

```powershell
git status --short --branch
git log --oneline -n 30
rg -n -i "PlatformArtifact|ArtifactSnapshot|StorageProvider|StoragePath|RegistryID|ReleaseExecutor|RBAC|Audit" api app docs
git diff --check
```

建议提交信息：`docs: 固化阶段3制品实施方案`

### 4.2 批次 2：制品与存储配置模型、dataservice

目标：建立可追溯、可归档、可安全保存的 V0.5 制品与对象存储配置基础。

范围：

- 扩展或启用 Artifact 的类型、来源、文件名、大小、SHA256、StorageProvider、StoragePath、Retained、Cleanable、构建/推送状态与最终镜像字段。
- 新增受控对象存储配置模型与 dataservice，至少包含 provider、endpoint、region、bucket、path prefix、TLS 配置和加密保存的凭据元数据。
- 建立制品构建/导入/推送任务的最小状态与锁边界；不得把运行时 Docker 操作放进 handler 事务。
- 接入 BoltDB bucket、迁移、Export/Import、逻辑归档、资源版本和项目归属校验。
- 设计 Release ArtifactSnapshot 与原始制品可清理性之间的引用查询，保证历史事实不被删除。

不做：不提供上传接口；不连接真实对象存储；不导入 archive，不构建镜像，不推送 registry。

依赖：批次 1 冻结的 schema、生命周期和清理契约。

交付物：模型、dataservice、迁移、Export/Import、单元测试。

验收标准：制品和对象存储配置可按权限保存、读取、更新、归档；密钥默认脱敏；历史 Release 关联不被归档/清理逻辑破坏。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/platform ./api/datastore ./api/dataservices/... -count=1
E:\develop\Go\bin\go.exe test ./api ./api/internal/testhelpers -count=1
git diff --check
```

建议提交信息：`feat: 新增平台制品存储模型`

### 4.3 批次 3：手动上传与安全校验

目标：让有权限用户安全上传受支持制品，并形成可审计、可重新使用的 Artifact 原始输入。

范围：

- 新增项目范围上传会话或等价 API：服务端强制校验项目角色、文件大小、Content-Type 仅作提示、真实格式与 SHA256。
- 使用受控临时目录与原子移动保存原始制品；中断、超时、hash 不匹配或元数据落库失败必须清理临时文件。
- 支持 Jar、dist zip、Docker tar 与 OCI archive 的基础类型识别；archive 深度校验留给对应批次。
- 保存上传人、时间、大小、SHA256、原始文件名、项目/应用/服务归属、来源和可清理性；写入脱敏审计。
- 前端只提供项目内上传入口和状态，不显示服务器真实路径或敏感错误细节。

不做：不从对象存储拉取；不导入镜像；不解压/执行 Jar 或 dist；不推送 registry。

依赖：批次 2 Artifact 生命周期、临时目录与权限模型。

交付物：上传 handler/service、类型检测、清理逻辑、权限/大小/hash/审计测试、最小前端入口。

验收标准：有效受支持文件得到 Artifact；超限、hash 不匹配、未知类型、路径伪造、无权限上传均被拒绝且不残留临时文件；审计不含文件内容或路径秘密。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/http/handler/platform ./api/platform -count=1
pnpm typecheck
rg -n -i "access.?key|secret.?key|password|token|authorization" api/http/handler/platform api/platform app/react/portainer/platform
git diff --check
```

建议提交信息：`feat: 支持平台制品安全上传`

### 4.4 批次 4：MinIO / S3 Compatible 制品拉取

目标：让项目可从管理员配置的 MinIO / S3 Compatible 存储选择或输入对象路径，并受控拉取为 Artifact。

范围：

- 实现 MinIO / S3 Compatible adapter：连接测试、对象元数据读取、受限路径浏览或手动路径、下载超时、大小限制、SHA256 校验与错误分类。
- 对象存储配置仅全局管理员可管理；项目用户只能选择已获授权的配置/路径，服务端校验项目边界。
- 将拉取文件复用批次 3 的临时文件、类型识别、元数据、原子保存与审计链路。
- 允许用户提供期望 SHA256；不一致时拒绝落库和构建。
- 阿里云 OSS 只验证能否通过 S3 Compatible endpoint 接入；不实现独立 OSS SDK。

不做：不开放任意外部 URL 下载；不生成公开预签名 URL；不接入独立 OSS adapter；不构建或推送镜像。

依赖：批次 2 存储配置、批次 3 安全输入管线。

交付物：S3 Compatible adapter、handler/API、凭据脱敏、fake adapter 测试与 MinIO 手工演练记录。

验收标准：对象路径、大小、hash、超时、权限和错误 reason 可追踪；存储凭据不泄漏；下载失败不保留半成品 Artifact。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/platform ./api/http/handler/platform -count=1
pnpm typecheck
git diff --check
```

建议提交信息：`feat: 支持平台S3制品拉取`

### 4.5 批次 5：Docker tar / OCI archive 导入

目标：安全导入 Docker image tar 与 OCI archive，读取镜像元信息并生成可推送的本地候选镜像。

范围：

- 实现 archive 类型与安全校验：拒绝路径穿越、链接逃逸、设备文件、超限解压、缺失 manifest/config、无效层或不支持架构。
- 使用受控 Docker adapter 导入 archive，读取原始镜像名、tag、架构、创建时间和本地 image ID；导入失败清理临时标签与临时文件。
- 将导入结果回写 Artifact 构建状态与可追溯性；尚未推送时不得标记为最终可部署镜像。
- 保留原始 archive，以支持审计、重新导入和后续清理；所有导入操作写结构化审计。

不做：不导入任意容器运行时格式；不部署未推送标准 tag 的候选镜像；不创建内置 registry；不支持多架构调度。

依赖：批次 3/4 的 Artifact 输入、批次 1 的 archive 约束。

交付物：archive 校验器、Docker import adapter、状态机接入、异常 archive 测试和 Docker 手工演练记录。

验收标准：有效 Docker tar/OCI archive 能得到候选镜像元数据；恶意或异常 archive 被拒绝且不残留镜像；导入失败不影响当前 Release。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/platform ./api/http/handler/platform -count=1
git diff --check
```

建议提交信息：`feat: 支持平台镜像归档导入`

### 4.6 批次 6：Java 8 Jar 受控包装镜像

目标：把 Java 8 Jar 以平台固定模板包装为镜像，禁止用户输入变成任意构建命令。

范围：

- 实现 Java 服务制品校验：Jar 格式、大小、SHA256 和与项目/服务的归属。
- 固定 Java 8 基础镜像、工作目录、Jar 文件名和标准启动命令；用户仅可提交受控字段：JVM 参数、应用参数、端口、健康检查、工作目录的受限子集。
- 通过 Docker adapter 构造受控 build context 与 Dockerfile，记录模板版本、构建日志摘要、候选 image ID 和失败 reason。
- 构建前后维持 Release 锁与 Artifact 构建互斥；失败清理 context 和候选镜像，不影响当前线上 Release。
- 前端提供 `java-service` Jar 选择与受控参数表单，所有可见文案接入中英文。

不做：不执行用户 Dockerfile、shell 脚本、Maven/Gradle、源码编译或任意 JDK 版本；不在本批推送/发布。

依赖：批次 3/4 输入、批次 2 构建状态、阶段 1 Docker adapter。

交付物：Java 模板服务、构建 adapter、handler/API、前端受控表单、模板/失败/清理测试。

验收标准：有效 Jar 仅按 Java 8 固定模板生成候选镜像；恶意参数与构建失败不能执行任意命令，也不能破坏当前发布。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/platform ./api/http/handler/platform -count=1
pnpm typecheck
git diff --check
```

建议提交信息：`feat: 支持平台Jar制品包装`

### 4.7 批次 7：前端 dist 受控包装镜像

目标：把前端 dist 压缩包安全包装为受控静态镜像，支持 SPA 与 MPA/纯静态站点。

范围：

- 解压前后校验 zip 路径、大小、文件数量、符号链接和入口文件；SPA 必须有 `index.html`，MPA/纯静态站点按真实文件路径提供资源。
- 使用平台维护的静态 Nginx 基础镜像和模板生成容器内站点配置；SPA 仅在容器内部配置 `index.html` fallback，不引入中心网关、域名或证书。
- 用户仅可选择 SPA/MPA 模式、受限静态缓存策略和可选 404 页面；不得提交任意 Nginx 配置。
- 构建结果写入 Artifact 元数据和审计，失败清理临时解压目录/候选镜像。
- 前端提供 dist 来源、模式、入口校验状态和失败提示，并兼容深色/浅色主题。

不做：不支持 SSR、Node 服务、任意静态服务器配置、中心 Nginx、路由切流或域名绑定。

依赖：批次 3/4 输入、批次 2 构建状态、批次 6 受控构建执行模式。

交付物：dist 校验器、静态模板服务、构建 adapter、前端表单、SPA/MPA/恶意 zip 测试。

验收标准：有效 dist 可生成候选静态镜像；SPA/MPA 行为差异可预测；恶意 zip 或缺失入口不会产生镜像或残留文件。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/platform ./api/http/handler/platform -count=1
pnpm typecheck
git diff --check
```

建议提交信息：`feat: 支持平台dist制品包装`

### 4.8 批次 8：registry 推送、发布接入与原始制品清理

目标：把候选镜像重新打平台标准 tag 并推送到已有 registry，随后以最终 image+digest 复用阶段 1 单机发布；提供安全的原始制品清理。

范围：

- 复用 Portainer registry 凭据和 TLS 配置，对候选镜像执行标准 tag、push、digest 解析和可追溯性记录。
- 标准 tag 至少包含 registry namespace、项目、服务和版本；不得覆盖不属于当前 Artifact 的已有 tag。tag 冲突、认证、TLS、push、digest 解析失败必须有明确 reason。
- 仅在成功推送并得到最终镜像引用后，允许生成或更新可部署 Artifact；发布请求仍经项目角色、Endpoint 权限、配置快照、锁、幂等和既有 ReleaseExecutor 校验。
- 新增原始制品清理预检/API：检查 Artifact 是否被进行中构建、发布或历史 Release 追溯所需；删除原始对象不删除 Artifact 与 Release 事实。
- 记录 build/import/push/cleanup 审计，前端展示最终 tag、digest、保留状态和清理阻断原因。

不做：不推送到内置 registry；不做跨 registry 复制；不做多主机、网关切流或批量发布；不通过清理改写历史 Release。

依赖：批次 5/6/7 候选镜像、阶段 1 registry 与发布链路、阶段 2 审计/回滚快照。

交付物：registry push adapter、标准 tag 规则、发布预检接入、清理服务/API、审计、集成测试与 registry 手工演练记录。

验收标准：三类候选镜像推送已有 registry 后可创建单机 Release；push/部署失败不影响当前线上版本；可安全清理不再受引用保护的原始制品。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api ./api/platform ./api/http/handler/platform -count=1
pnpm typecheck
git diff --check
```

建议提交信息：`feat: 支持平台制品推送与发布`

### 4.9 批次 9：前端整合与 V0.5 验收

目标：形成从制品来源选择到镜像部署、追溯、清理和审计的 V0.5 用户闭环，并完成验收记录。

范围：

- 完善制品管理页面、上传/对象存储来源、类型/大小/hash/状态、构建与推送进度、最终 tag/digest、保留/清理入口。
- 完善部署向导：服务类型与制品类型匹配、Jar/dist 受控参数、archive 元信息、推送后镜像选择、配置预览和发布确认。
- 前端按服务端 Permissions 隐藏或禁用上传、对象存储拉取、构建、推送、清理和部署入口；直接 API 访问仍由后端拒绝。
- 展示结构化构建/推送/清理审计与失败 reason；敏感信息、实际存储路径、凭据、签名 URL 和制品内容不得显示。
- 补齐中文/英文、深色/浅色主题、长文件名/hash/tag 的布局验证；新增 `docs/阶段3验收记录.md`。

不做：不新增阶段 4 网关或阶段 5 多主机入口；不做营销页、大屏、CI/CD 流水线或新的实时系统。

依赖：批次 8 的最终制品发布链路与前序所有后端能力。

交付物：阶段 3 前端整合、i18n、验收记录、必要 API 文档更新。

验收标准：管理员/项目管理员/开发者/观察者的行为符合权限矩阵；Jar、dist、Docker tar/OCI 与 MinIO/S3 各至少完成一条可追溯发布路径；失败、清理阻断、回滚兼容和敏感信息脱敏均有自动化或真实环境证据。

建议验证：

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api ./api/platform ./api/http/handler/platform -count=1
pnpm typecheck
git diff --check
```

建议提交信息：`feat: 完成阶段3制品驱动验收`

## 5. 建议 API 与状态契约

批次 1 必须在实现前确认最终命名；以下仅作为与既有 `/api/platform` 风格一致的最小草案：

| 资源 | 建议接口 | 约束 |
| --- | --- | --- |
| 对象存储配置 | `GET/POST /api/platform/artifact-storages`、`GET/PUT/DELETE /{id}`、`POST /{id}/test` | 管理员管理，响应脱敏，凭据加密 |
| 手动上传 | `POST /api/platform/artifacts/upload` 或创建会话后分段上传 | 项目写权限、大小/hash/类型校验、临时文件清理 |
| 对象存储拉取 | `POST /api/platform/artifacts/from-storage` | 仅已授权 storage/path，服务端下载并校验 |
| 构建/导入 | `POST /api/platform/artifacts/{id}/prepare-image` | 仅受支持 Artifact 类型，异步状态可查询 |
| 推送 | `POST /api/platform/artifacts/{id}/push` | 已有 registry、标准 tag、digest 回写 |
| 原始制品清理 | `POST /api/platform/artifacts/{id}/cleanup-original` | 先做引用预检，保留 Release 事实 |
| 查询 | `GET /api/platform/artifacts`、`GET /{id}` | 可筛选项目/应用/服务/类型/来源/状态，默认脱敏 |

建议的 Artifact 生命周期：`uploaded` / `fetched` → `validated` → `importing` 或 `building` → `built` → `pushing` → `ready`；任一失败进入 `failed` 并保留 reason、审计与可安全重试边界。已归档 Artifact 不可新建发布，但不得影响已有 Release 的查询、回滚快照和审计。

## 6. Goal 模式总控提示词

在 Codex 中创建阶段 3 Goal 时，复制下列提示词：

```text
目标：根据 docs/阶段3分批实施方案与Goal模式提示词.md，按批次 1 到批次 9 顺序完成阶段 3（V0.5 制品驱动部署）开发。每一批必须先审计、实现、测试通过、中文提交对应代码、更新 docs/阶段3实施进度.md 后，才能进入下一批。

工作方式：

1. 先读取并遵守：
   - AGENTS.md
   - docs/阶段3分批实施方案与Goal模式提示词.md
   - docs/阶段3实施进度.md（如存在）
   - docs/阶段2实施进度.md
   - docs/阶段2验收记录.md
   - docs/阶段0平台核心模型与后端边界设计.md
   - docs/私有化微服务部署平台产品需求文档.md
   - docs/功能优先级与工作量排期.md

2. 先审计当前状态：
   - 执行 git status --short --branch。
   - 执行 git log --oneline -n 30。
   - 搜索阶段 3 相关代码和文档：PlatformArtifact、ArtifactSnapshot、StorageProvider、StoragePath、SHA256、Retained、Cleanable、RegistryID、ImageDigest、ReleaseExecutor、RBAC、PlatformAuditLog。
   - 判断当前应从哪个批次继续；不得重复已有提交与测试证据已经证明完成的批次，证据不足时先补验证或补缺口。

3. 按批次顺序推进：
   - 批次 1：阶段 3 基线审计与制品契约冻结。
   - 批次 2：制品与存储配置模型、dataservice。
   - 批次 3：手动上传与安全校验。
   - 批次 4：MinIO / S3 Compatible 制品拉取。
   - 批次 5：Docker tar / OCI archive 导入。
   - 批次 6：Java 8 Jar 受控包装镜像。
   - 批次 7：前端 dist 受控包装镜像。
   - 批次 8：registry 推送、发布接入与原始制品清理。
   - 批次 9：前端整合与 V0.5 验收。

4. 每批开始前必须做批次审计：
   - 从本方案提取本批目标、范围、不做项、依赖、交付物、验收标准。
   - 搜索当前代码和文档，确认本批缺口与用户并行改动。
   - 制定本批实现计划并用 update_plan 跟踪。

5. 阶段 3 红线：
   - 只支持 Java 8 Jar、前端 dist、Docker image tar、OCI archive 和 MinIO/S3 Compatible 输入；所有输出必须是镜像。
   - 不实现中心 Nginx、域名、证书、网关切流、多 Docker 主机、跨主机分批发布、Kubernetes、Git、源码构建、任意 Dockerfile、内置 registry、完整 CI/CD、审批流、漏洞扫描、Python 源码包/wheel、SSR/Node 构建或独立阿里云 OSS adapter。
   - Jar 与 dist 只能使用平台维护的受控模板、基础镜像和固定命令；不得执行用户脚本、Dockerfile 或任意 shell 命令。
   - archive 必须防止路径穿越、链接逃逸、设备文件、压缩炸弹和异常 manifest/layer；失败不得留下候选镜像或临时文件。
   - 不得把 registry/对象存储凭据、敏感变量、原始制品内容、签名 URL、服务器真实路径写入非加密 BoltDB 字段、日志、审计、错误消息、前端 toast 或测试快照。
   - 原始制品清理不得删除被进行中任务、历史 Release、回滚或审计追溯仍需要的对象。
   - 不回滚用户或其他协作者已有改动；只提交本批相关文件。

6. 每批实现规则：
   - 优先沿用仓库已有 Portainer handler、dataservice、service、Docker adapter、React、i18n、错误处理、审计和测试风格。
   - 后端必须强制校验项目权限；触发 Docker、registry 或部署动作时还必须与 Portainer Endpoint 权限取交集。前端隐藏/禁用只能改善体验。
   - 构建、导入、推送与部署必须分离状态和锁；handler 事务只持久化事实与锁，不在事务内执行耗时 Docker/网络操作。
   - 最终 Artifact 必须记录最终 image、tag、digest、SHA256、来源与可追溯性；发布继续写不可变 ArtifactSnapshot、ConfigSnapshot 和 SecretSnapshots。
   - 推送失败、导入失败、构建失败、清理失败均不得改变当前线上 Release；审计要有明确 reason，敏感字段只写字段名。

7. 每批验证规则：
   - 模型、dataservice、handler、构建/导入/推送服务、权限、审计或状态机改动后，运行相关 go test。
   - 前端 TypeScript 改动后，运行 pnpm typecheck。
   - 上传、对象存储、archive、构建、registry 或凭据相关批次必须额外扫描明文泄漏、路径穿越和临时文件/候选镜像清理风险。
   - 有 Docker/MinIO/registry 环境时执行对应手工演练；没有时先完成 fake adapter 与本地测试，并将阻塞和待演练项写入进度文档。
   - 测试失败必须先修复并重跑；不得在测试失败时提交，除非失败有充分证据证明与本批无关。

8. 每批提交规则：
   - 每批完成并验证后，执行 git status --short，确认只包含本批相关改动。
   - stage 本批相关文件，使用中文 commit message。
   - 建议提交信息：
     - 批次 1：docs: 固化阶段3制品实施方案
     - 批次 2：feat: 新增平台制品存储模型
     - 批次 3：feat: 支持平台制品安全上传
     - 批次 4：feat: 支持平台S3制品拉取
     - 批次 5：feat: 支持平台镜像归档导入
     - 批次 6：feat: 支持平台Jar制品包装
     - 批次 7：feat: 支持平台dist制品包装
     - 批次 8：feat: 支持平台制品推送与发布
     - 批次 9：feat: 完成阶段3制品驱动验收
   - 提交成功后，在回复中说明本批提交哈希、验证命令和结果，再继续下一批。

9. 进度记录：
   - 维护 docs/阶段3实施进度.md。
   - 每批提交后记录：批次、提交哈希、完成内容、验证命令、测试结果、遗留风险、是否允许进入下一批。

10. 完成判定：
   - 只有批次 1 至批次 9 都有提交、验证结果和当前状态证据证明完成后，才能把 Goal 标记为 complete。
   - 如缺少真实 Docker、registry、MinIO/S3、权限用户或浏览器环境，先完成本地可验证部分并记录待演练项；只有同一阻塞条件连续三轮仍无法推进时，才按 Goal 模式 blocked 规则处理。

从现在开始执行：先审计当前仓库状态，判断应该从第几批继续，然后按上述规则推进。
```

## 7. Goal 模式续跑提示词

```text
继续当前阶段 3 Goal。

请先读取：
- docs/阶段3分批实施方案与Goal模式提示词.md
- docs/阶段3实施进度.md（如存在）
- docs/阶段2实施进度.md
- docs/阶段2验收记录.md
- docs/私有化微服务部署平台产品需求文档.md
- docs/功能优先级与工作量排期.md
- git log --oneline -n 30
- git status --short --branch

然后判断下一步：
1. 如果上一批已经提交且测试证据充分，进入下一批。
2. 如果上一批有提交但测试证据不足，先补验证；必要时补修复提交。
3. 如果上一批未提交，继续完成该批，实现、测试、提交并更新 docs/阶段3实施进度.md 后再进入下一批。
4. 如果发现当前改动包含用户并行工作，先识别范围，只暂存和提交本批相关文件，不得回滚用户改动。

仍然必须遵守：
- 每批先审计、实现、测试通过、中文 commit 和进度记录后再进入下一批。
- 不提交无关文件，不泄漏制品内容、存储凭据或敏感变量明文。
- 不实现阶段 4/5/7 能力，不执行用户 Dockerfile、脚本或源码构建。
- handler 事务不执行耗时 Docker、对象存储或 registry 操作。
- 构建/导入/推送失败不得破坏当前线上 Release，原始制品清理不得破坏历史追溯与回滚。
```

## 8. 阶段 3 最小验收用例

阶段 3 完成时至少应具备以下证据：

1. 管理员配置一个 MinIO / S3 Compatible 存储，API 默认脱敏，连接测试不会回显 access key 或 secret key。
2. 项目管理员/开发者按权限上传 Jar、dist zip、Docker tar/OCI archive；超限、未知类型、hash 不匹配和无权限上传均被拒绝且临时文件被清理。
3. 从对象存储拉取制品时，路径、大小、期望 SHA256、超时与失败 reason 可追踪；半成品不会成为可发布 Artifact。
4. 恶意 archive（路径穿越、链接逃逸、压缩炸弹、无效 manifest）被拒绝，且不残留候选镜像。
5. 合法 Docker tar 与 OCI archive 能导入、读取镜像元信息、重新打平台标准 tag，并在推送后记录最终 digest。
6. Java 8 Jar 仅使用受控基础镜像和固定启动模板；提交 Dockerfile、构建脚本或任意命令的尝试被拒绝。
7. SPA dist 缺失 `index.html` 时被拒绝；SPA 与 MPA/纯静态站点生成的容器内路由配置可区分，且未启用中心网关。
8. 三类候选镜像推送到已有 registry 后可复用 Docker 单机发布；push、端口冲突、健康检查失败仍保持当前线上版本可用。
9. 原始制品被历史 Release 或进行中任务引用时无法清理；可清理制品删除原始对象后仍保留 Artifact、Release、SHA256、最终 tag/digest 与审计事实。
10. 管理员、项目管理员、开发者、观察者及无 Endpoint 权限用户的上传、拉取、构建、推送、清理和部署权限符合服务端矩阵；前端仅作体验优化。
11. 构建、导入、推送、清理和发布均有脱敏结构化审计；中英文、深色/浅色主题与长文件名/hash/tag 不破坏页面布局。
