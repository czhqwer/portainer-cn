# 阶段 4-7 分批实施方案与 Goal 模式提示词（后端优先）

版本：v1.0
日期：2026-07-12
关联文档：[私有化微服务部署平台产品需求文档](./私有化微服务部署平台产品需求文档.md)、[功能优先级与工作量排期](./功能优先级与工作量排期.md)、[阶段3实施进度](./阶段3实施进度.md)、[阶段3验收记录](./阶段3验收记录.md)、[阶段0平台核心模型与后端边界设计](./阶段0平台核心模型与后端边界设计.md)

## 1. 总体结论

本方案用于让 Codex 按顺序实施阶段 4 至阶段 7 的后端能力，并把每个阶段作为一个独立、可审计、可回滚的 Git 交付单元。阶段 4 至阶段 6 的 PRD 范围已经足以拆分后端工作包；阶段 7 在 PRD 中被定义为“按使用反馈滚动”的 V1.x 增强能力，不能在没有明确能力清单和验收口径的情况下自动开始编码。因此阶段 7 的首批是强制范围冻结 Gate，而不是直接实现灰度、ACME、监控或 Kubernetes。

本轮严格采用“后端优先”边界：

- 可以改动 `api/`、后端路由注册、后端测试、必要的数据迁移、`docs/`、Makefile 中与后端验证直接相关的内容。
- 不得改动 `app/react/`、`app/` 中前端页面、`translations/`、前端路由、样式、前端测试或任何用户界面；即使 API 已具备，也只保留后端接口和文档证据。
- 不能为了让前端通过而修改 TypeScript、翻译或界面。后端阶段完成时，前端仍可没有入口；后续由独立前端阶段接入。
- 每个新增或改造的后端 handler、service、schema/query 执行和复杂安全逻辑，必须按仓库约定添加解释“为什么”的中文注释。

阶段目标如下：

| 阶段 | 名称 | 后端交付目标 | 明确不做 |
| --- | --- | --- | --- |
| 4 | Nginx 网关、域名与手动证书 | 受控 Nginx、路由/证书模型、配置校验和原子 reload；网关失败保持旧配置与旧流量 | 多主机分批、HTTP 灰度、ACME、前端页面 |
| 5 | 多 Docker 主机与分批发布 | 环境多目标、主机组/批次策略、中心网关到 `hostIP:publishedPort`、逐主机结果和失败暂停 | 灰度权重、Kubernetes、前端页面 |
| 6 | 数据库绑定与兼容增强 | 服务绑定环境级数据库连接、受控变量注入、数据库工作台深链 API、OSS 兼容证据/必要适配、容量与失败诊断 | 创建数据库实例、容器级旧入口依赖、前端页面 |
| 7 | V1.x 增强能力 | 先冻结本轮后端子范围；获确认后再实施被确认的灰度、ACME、可观测性或 Kubernetes 子集 | 未确认能力、前端页面、Git/源码构建、内置 registry、完整 CI/CD |

## 2. 所有阶段共用的执行、分支与安全规则

### 2.1 前置状态

- 阶段 3 已完成；其本地验证与外部演练待办见 [阶段3验收记录](./阶段3验收记录.md)。开始阶段 4 前必须重新审计这些待办，不能把 fake adapter 的成功误写为真实 Docker、registry、Nginx、Agent 或多角色环境证据。
- 现有 `PlatformEnvironment.Targets`、`PlatformDeploymentTarget`、`PlatformRelease.GatewaySnapshot`、`TargetResults` 和 `PlatformExecutorMode` 是为后续阶段预留的结构，不等于功能已经实现。阶段 4/5 必须先核对其现状、兼容性和测试覆盖，再决定是扩展还是替换。
- 现有数据库工作台使用环境级 `DatabaseConnection`，其中 `ContainerID` 仅可作为自动填充/兼容信息。阶段 6 的真实访问与部署注入必须以 `Host`/`Port` 为准，不能把新的环境级能力重新绑定到容器 WebSocket 代理。

### 2.2 分支生命周期（每阶段必须重复）

本方案中的“基于 develop 拉分支”指从当前本地 `develop` 创建阶段分支。除非用户另行授权同步远端，不执行会改变本地 `develop` 基线的 `pull`、`rebase`、`reset --hard` 或强制推送。

| 时点 | 强制动作 | 通过条件 |
| --- | --- | --- |
| 阶段开始前 | `git status --short --branch`、`git switch develop`、再次检查状态、`git log --oneline -n 30` | 工作区完全干净，存在本地 `develop`，当前基线可审计 |
| 创建分支 | `git switch -c codex/platform-phase<N>` | 分支起点是此刻的 `develop`；阶段 4/5/6/7 分别使用 `codex/platform-phase4` 至 `codex/platform-phase7` |
| 每个批次结束 | 只暂存该批相关文件，中文提交，更新 `docs/阶段<N>实施进度.md` | 提交哈希、测试命令、结果、风险和“是否可进入下一批”均已记录 |
| 阶段完成前 | 完成所有批次、阶段验收记录和后端测试；`git diff --check`；确认 `git diff --name-only develop...HEAD` 不含前端目录 | 阶段分支干净，验收证据完整 |
| 合并回 develop | `git switch develop` 后执行 `git merge --no-ff codex/platform-phase<N> -m "merge: 合并阶段<N>后端实施"`，再在 `develop` 重跑相关测试 | 非快进合并成功，合并后工作区干净，验证通过 |
| 下一阶段 | 仅在上一阶段已实际合并到 `develop` 后，再依本表创建下一阶段分支 | 不从上一阶段特性分支继续开发 |

遇到以下情况必须停止当前 Git 操作并报告，而不是猜测处理：

- `develop` 或阶段分支开始时存在非本阶段的未提交改动；先识别所有权，只在用户确认或可完全避开时继续。
- 阶段分支已存在但其合并基点不是当前 `develop`；先展示 `git merge-base`、提交图和差异，不能 `reset` 或重写历史。
- 合并时发生冲突；可使用 `git merge --abort` 恢复到合并前状态，然后报告冲突文件和基线差异。不得强制覆盖其他协作者的变更。
- 阶段验收、关键单测或合并后回归失败；不得把该阶段合并到 `develop`。

### 2.3 通用后端基线

- 复用 Portainer 的 handler、dataservice、事务、鉴权、Endpoint 访问控制、registry、加密和 Docker/Agent 适配器；不引入大型框架或以 shell 命令替代受控 Docker API。
- 所有写入 API 必须在服务端校验项目角色和目标 Endpoint 权限的交集。前端不存在不构成放宽后端授权的理由。
- handler 事务只保存模型、版本、锁、快照和审计事实；Nginx 校验/reload、网络探测、Docker 操作、对象存储和外部 API 调用不得在 BoltDB 事务内执行。
- 不能在日志、审计、API 响应、错误消息、测试快照或文档中暴露密码、私钥、证书私钥、数据库 URL 中的密码、签名 URL、真实临时路径或内部拓扑地址。审计仅记录资源 ID、脱敏摘要、结果和固定 failure reason。
- 发布、网关更新、批次切换、证书变更、数据库变量注入、OSS 拉取均须写结构化 `PlatformAuditLog`。失败不能篡改历史 Release，也不能破坏当前 `CurrentServingReleaseID`。
- 先使用 fake adapter/单元测试覆盖状态机和失败矩阵；真实 Docker、Nginx、registry、MinIO/OSS、Agent、数据库和 Kubernetes 证据必须单独记录，不能用 mock 成功替代。

## 3. 阶段 4：Nginx 网关、域名与手动证书

### 3.1 范围与红线

阶段 4 的目标是让单 Docker 主机上的 HTTP 服务可以通过受控 Nginx 使用域名和路径统一访问。发布候选运行时验证成功后，只有 Nginx 配置通过 `nginx -t` 且 reload 成功，才允许切换网关快照；配置测试或 reload 失败必须保持活动配置、现有 upstream 和线上旧版本不变。

只支持手动上传/选择已有证书、HTTP/HTTPS、强制 HTTPS、WebSocket、代理超时、最大上传大小和同一域名下的路径路由。阶段 4 不实现跨主机 upstream、主机组、分批发布、Nginx 权重灰度、ACME 自动签发、Keepalived、高可用网关、Kubernetes Ingress 或前端页面。

### 3.2 批次总览

| 批次 | 名称 | 核心交付 | 完成标志 |
| --- | --- | --- | --- |
| 4.1 | 基线审计与网关契约冻结 | 阶段进度文档、路由/证书/快照/API/失败矩阵 | 单机边界、配置所有权和切流顺序明确 |
| 4.2 | 网关领域模型与 dataservice | 网关实例、路由、证书元数据、版本化快照、导入导出和迁移 | 持久化兼容、敏感字段不出库 |
| 4.3 | 受控 Nginx 运行时适配器 | 网关容器发现/创建、受控目录与文件读写、健康状态 | 没有任意命令执行或路径逃逸 |
| 4.4 | 域名、路径和手动证书 API | 校验、冲突检测、RBAC、证书安全存储/引用和审计 | 非法路由/无权请求被服务端拒绝 |
| 4.5 | 配置渲染、预检与原子 reload | 确定性模板、`nginx -t`、候选配置、reload/恢复 | 校验或 reload 失败保持旧配置 |
| 4.6 | 发布与回滚的网关切流接入 | Release `GatewaySnapshot`、切流步骤、失败补偿、回滚接入 | 切流失败不影响当前线上版本 |
| 4.7 | 后端验收与真实演练记录 | 回归、权限/泄漏扫描、单机 Nginx 演练清单和验收记录 | 可合并到 `develop` |

### 3.3 分批实施要求

#### 4.1 基线审计与网关契约冻结

目标：先审计 `PlatformEnvironment.Targets` 的 gateway/workload 角色、`PlatformGatewaySnapshot`、Release 状态机、Docker runtime driver、Endpoint 授权和现有 TLS 文件存储方式，冻结受控网关的权属和失败语义。

范围：新增 `docs/阶段4实施进度.md`；定义网关实例、路由、证书、活动配置版本、候选配置版本和 Release 网关快照的字段契约；定义同域名路径冲突规则、域名规范化、通配符支持与否（默认不支持）、路径优先级、协议限制、证书引用和审计 reason；定义“候选运行时成功 → 渲染候选配置 → `nginx -t` → 原子激活 → reload → 验证 → 提交 Release”的顺序。

不做：不改业务 API/模型，不创建 Nginx 容器，不实施证书上传或实际 reload。

验收：文档明确只有一个单机 gateway target 时的选择规则，明确活动配置绝不被失败候选覆盖，明确 release/手动回滚如何使用不可变 GatewaySnapshot。

建议提交：`docs: 固化阶段4网关实施方案`。

#### 4.2 网关领域模型与 dataservice

目标：建立可追溯的后端模型，而不是把 Nginx 文本直接混入 ServiceDeployment 或 Release。

范围：按阶段 4.1 契约新增/扩展平台模型、dataservice、BoltDB bucket、导入导出和迁移；路由必须引用项目、环境、ServiceDeployment、目标端口和可选证书；证书元数据与私钥材料分离；配置版本记录 hash、生成时间、操作者、活动状态和安全摘要；Release 仅保存当次路由/证书引用、upstream 与配置 hash 的不可变快照。

安全要求：私钥、证书原文和受控目录真实路径不进入普通模型 JSON、审计摘要或错误。若复用文件系统存储，文件名必须由服务端 ID 生成、路径固定在平台受控根目录，并以原子写入和权限收敛处理。

验收：模型/dataservice/Export-Import 单测覆盖创建、更新、归档、引用保护、旧数据缺省值和敏感字段不泄漏。

建议提交：`feat: 新增平台网关领域模型`。

#### 4.3 受控 Nginx 运行时适配器

目标：将 Nginx 操作限制为平台创建或接管的受控网关容器，而不是允许用户提供 Dockerfile、容器 ID、配置路径或 shell 命令。

范围：实现网关运行时接口和 Docker/Agent 实现；确定 gateway Endpoint/Node、平台标签、受控 Nginx 镜像版本、只读模板和受控配置挂载；提供读取活动版本、写入候选版本、测试、reload、恢复和状态探测能力。所有操作均使用 Docker API 或已验证的 Agent 通道，且有 context 超时。

验收：fake adapter 覆盖容器不存在、错误 Endpoint、权限不足、候选写入失败、`nginx -t` 失败、reload 超时和恢复失败；运行时错误映射为稳定 reason，不能把 Docker/Agent 原始错误回显给客户端。

建议提交：`feat: 新增受控Nginx网关适配器`。

#### 4.4 域名、路径和手动证书 API

目标：提供可由未来前端调用的后端 API，并在服务端完成所有输入、归属、Endpoint 和项目角色检查。

范围：路由/证书的 CRUD、绑定/解绑、预检和查询 API；校验合法 DNS 域名、路径以 `/` 开头、端口属于服务暴露端口、超时/上传大小边界、HTTPS/强制 HTTPS 组合和 WebSocket 限制；同一环境中拒绝相同 host/path 的冲突，按已冻结的规则处理 `/` 与子路径遮蔽。手动证书必须校验 PEM 格式、证书/私钥匹配和有效期元数据；更新或删除被活动路由/Release 快照引用的对象必须被阻断或归档。

验收：handler 测试覆盖管理员、项目管理员、开发者、观察者、无 Endpoint 权限用户；覆盖冲突路由、错误证书、证书私钥不回显、请求幂等和审计成功/拒绝/失败。

建议提交：`feat: 支持平台域名路由与手动证书`。

#### 4.5 配置渲染、预检与原子 reload

目标：以确定性模板生成 Nginx 配置，并让失败永久停留在候选版本而不触碰活动流量。

范围：从已验证的路由、证书和当前单机运行端口生成 `upstream`、`server`、`location`；支持 `/api` 与 `/` 指向不同服务；渲染时严格转义配置值并拒绝控制字符/注入；候选配置先写入隔离版本目录，再执行 `nginx -t`，通过后原子切换活动配置并 reload。reload 失败必须立即恢复前一活动配置并再次 reload；若恢复也失败，记录人工处置状态和完整脱敏审计，但不能删除旧配置或伪造成功。

验收：单元测试覆盖确定性 hash、路由排序、路径优先级、WebSocket/HTTPS 模板、注入拒绝和每一种预检/reload/恢复失败。真实单机 Nginx 演练应验证错误配置时旧域名持续可访问。

建议提交：`feat: 支持平台网关安全配置发布`。

#### 4.6 发布与回滚的网关切流接入

目标：在不破坏阶段 1 单机 candidate/恢复语义的前提下，将网关更新纳入 Release 状态与补偿矩阵。

范围：为 HTTP 服务增加网关预检与切流步骤；使用 candidate 验证后得到的已发布端口生成新的 upstream，网关切流成功后才更新 `GatewaySnapshot` 与服务的当前运行事实；失败时删除候选/新运行时或恢复旧运行时，并恢复旧网关版本。重新部署、人工回滚和中断恢复均使用历史 Release 的 GatewaySnapshot，不根据当前可变路由猜测旧配置。

不做：不把非 HTTP worker/scheduler/python-job 接入网关；不改变多主机/批次执行器；不承诺灰度权重。

验收：Release executor/handler 测试覆盖新服务首次发布、已上线服务更新、网关 `test` 失败、reload 失败、恢复失败、健康检查失败、人工回滚与并发发布锁。任何失败均验证旧 upstream 和旧 Release 保持服务。

建议提交：`feat: 发布流程接入平台网关切流`。

#### 4.7 后端验收与真实演练记录

目标：在不接入前端的情况下收口阶段 4 的 API、状态机、授权、安全和外部环境证据。

范围：新增 `docs/阶段4验收记录.md`；执行相关 Go 测试、敏感信息/模板注入扫描和 `git diff --check`；记录待在 Docker/Agent + Nginx 环境执行的演练清单，包括域名路由、`/api` 与 `/`、HTTPS、WebSocket、证书替换、非法配置、reload 失败和回滚。

验收：分支没有 `app/react/`、`translations/` 或其他前端改动；所有本地后端测试通过；每个外部未验证项都有明确原因和后续步骤。

建议提交：`test: 完成阶段4网关后端验收`。

## 4. 阶段 5：多 Docker 主机与分批发布

### 4.1 范围与红线

阶段 5 将环境从单一 workload target 扩展到多个 Docker workload target，并由阶段 4 的中心 Nginx gateway 通过 `hostIP:publishedPort` 转发。发布按固定、可快照的批次顺序运行；每台主机都留下独立结果。任一批失败时必须暂停后续批，按策略恢复已切换目标和网关，不得把未执行主机标记为成功。

不把 Portainer 通用 `EndpointGroup` 直接当作平台发布组，除非批次 5.1 的审计证明其生命周期、权限、排序和快照语义全部满足平台要求。默认使用平台自己的不可变目标/组快照，避免用户修改通用组后重写历史 Release。阶段 5 不实现 Nginx 权重灰度、Kubernetes、跨 registry 复制、前端主机管理页面或自动扩缩容。

### 4.2 批次总览

| 批次 | 名称 | 核心交付 | 完成标志 |
| --- | --- | --- | --- |
| 5.1 | 基线审计与多主机契约冻结 | 多目标、组、批次、连通性、回滚矩阵和进度文档 | 目标排序与快照语义明确 |
| 5.2 | 多目标/主机组/策略模型 | 平台主机组、批次策略、迁移、快照和 dataservice | 历史 Release 不受后续组修改影响 |
| 5.3 | 环境目标 API 与连通性预检 | 多 target CRUD、Endpoint 权限、网关到发布端口探测 | 不能从容器 IP 推断可达性 |
| 5.4 | 多目标执行器与逐主机结果 | multi executor、目标锁、运行态/失败原因持久化 | 每台主机可独立追踪与恢复 |
| 5.5 | 分批调度、暂停与回滚 | 批次大小/间隔/暂停策略、健康失败补偿 | 失败批不会启动后续批 |
| 5.6 | 中心网关多 upstream 接入 | `hostIP:publishedPort` upstream、批次后安全切流 | 网关与目标快照一致 |
| 5.7 | 后端验收与多 Agent 演练记录 | 回归、并发/网络故障测试、验收记录 | 可合并到 `develop` |

### 4.3 分批实施要求

#### 5.1 基线审计与多主机契约冻结

目标：审计阶段 4 网关实现、单目标执行器、环境 targets、Endpoint/Agent 连接方式和 Release.TargetResults 预留字段，冻结多主机的模型与失败补偿。

范围：新增 `docs/阶段5实施进度.md`；冻结 target 的稳定身份（EndpointID、NodeName、HostAddress、角色、启用状态）、目标组快照、批次排序、批大小、批次间隔、失败暂停、健康失败回滚、发布锁范围、取消/重启恢复以及 Release.TargetResults 的状态机。定义 gateway Endpoint 必须可访问 workload `HostAddress:HostPort` 的连通性证据；不允许拿 Docker 容器 IP 作为跨主机入口。

验收：明确“批次成功”与“全部发布成功”的区别，明确已切换目标在后续批失败时的恢复策略，明确未运行/跳过/失败的目标记录方式。

建议提交：`docs: 固化阶段5多主机实施方案`。

#### 5.2 多目标、主机组与发布策略模型

目标：在平台领域中持久化和快照多主机发布所需的配置，保持旧 single 环境完全兼容。

范围：新增或扩展平台主机组、环境多 target、批次策略和 Release 目标/批次快照的模型、dataservice、迁移、导入导出和校验。single 环境保持原行为；multi 环境至少一个 enabled workload target 且恰好一个 enabled gateway target（或按阶段 5.1 固化的明确规则）；每个 target 必须具有安全可校验的 HostAddress。组成员、顺序和策略更新只能影响未来 Release，既有 Release 使用快照。

验收：测试覆盖 single→multi 兼容、重复 target、Endpoint 不存在、gateway/workload 角色错误、禁用 target、ResourceVersion 冲突、归档组引用保护及 Export/Import。

建议提交：`feat: 新增平台多主机发布模型`。

#### 5.3 环境目标 API 与连通性预检

目标：提供服务端受控的主机/组配置和连通性检查，不依赖未来前端，也不泄露内部网络信息。

范围：实现环境多目标/主机组/批次策略 API、项目角色与每个 Endpoint 的权限交集校验、HostAddress/端口校验以及 gateway→workload 发布端口 TCP/HTTP 预检。预检必须有超时、并发上限、固定 reason；响应只提供可操作结论和脱敏目标标识，详细网络错误只写安全服务端日志。

验收：handler 测试覆盖角色、无 Endpoint 访问、不同项目、不可达地址、端口冲突、DNS/超时、重复请求和审计。连通性检查是发布前置条件，不是绕过发布时实际健康检查的替代品。

建议提交：`feat: 支持平台多主机目标预检`。

#### 5.4 多目标执行器与逐主机结果

目标：从单目标执行器演进为可对多个 workload target 执行相同受控 Docker 发布动作的执行器，同时保留每个目标的 RuntimeRef、端口、状态和原因。

范围：抽取共享的单目标动作，新增 multi executor；每个目标拥有可续租的执行状态与 `PlatformReleaseTargetResult`，任何耗时 Docker/Agent 操作在事务外进行；进程重启后 queued/running 目标按明确规则恢复、标记 interrupted 或清理 candidate。禁止把部分成功聚合为总成功。

验收：fake runtime driver 测试覆盖某一目标 pull/candidate/switch/final health 失败、目标恢复失败、重复发布、并发锁、进程重启、取消和幂等；单目标 Release 回归保持通过。

建议提交：`feat: 支持平台多目标发布执行`。

#### 5.5 分批调度、暂停与回滚

目标：为多主机 Release 提供确定性的批次顺序、暂停点和失败补偿，不做 HTTP 流量权重。

范围：实现批次大小/间隔、每批预检、批间等待的可取消上下文、失败暂停策略、健康检查失败回滚策略和人工恢复/取消 API。每批开始/结束、每个 target 和每次暂停均追加 Release step 与结构化审计。暂停后不自行越过失败批；已成功但必须回滚的 target 需按其不可变运行/网关快照恢复。

验收：测试覆盖首批失败、后续批失败、批间取消、回滚成功/失败、仅部分 target 可达、目标顺序稳定、间隔不阻塞 handler 事务和外部状态被删除。发布记录准确显示 pending/running/succeeded/failed/skipped。

建议提交：`feat: 支持平台分批发布与失败暂停`。

#### 5.6 中心网关多 upstream 接入

目标：让阶段 4 网关在多主机环境安全转发到每台已成功发布的 `hostIP:publishedPort`，而不是直接访问容器内部 IP。

范围：扩展网关渲染/快照和 Release 协调逻辑，按已成功目标生成多 upstream；确保未健康、失败、已回滚或未执行的 target 不进入活动 upstream。网关切流仍执行候选配置、`nginx -t`、原子激活和 reload/恢复；发生失败时恢复上一配置和上一批次的可用服务集合。

不做：不生成 upstream `weight`，不加入灰度版本，不承诺多网关 HA。

验收：覆盖一台/多台成功、部分失败、网关 reload 失败、后续批失败、回滚、target 发布端口变化和历史 Release 回滚。真实演练验证 gateway 主机到至少两台 Agent/Docker 业务主机的连通性。

建议提交：`feat: 网关支持多主机服务转发`。

#### 5.7 后端验收与多 Agent 演练记录

目标：收口阶段 5 的后端交付，确保阶段 4 单机网关与阶段 1 单机发布没有回归。

范围：新增 `docs/阶段5验收记录.md`；执行多目标 executor、gateway、handler、dataservice 和回归测试；记录真实中心 Nginx + 两个 Docker/Agent workload target 的正常、不可达、端口冲突、健康失败、暂停、取消、恢复和回滚演练。

验收：前端目录无变更；本地测试、权限扫描、泄漏扫描和 `git diff --check` 通过；外部依赖不可用时留下明确待演练项。

建议提交：`test: 完成阶段5多主机后端验收`。

## 5. 阶段 6：数据库绑定与兼容增强

### 5.1 范围与红线

阶段 6 只增强数据库连接管理、服务绑定和部署变量注入，不创建 MySQL/PostgreSQL/Redis 实例、不执行迁移、不替应用管理数据库账号，也不把环境级连接重新变成容器级代理能力。可注入的变量限定为 `DATABASE_HOST`、`DATABASE_PORT`、`DATABASE_USER`、`DATABASE_PASSWORD`、`DATABASE_NAME`、`DATABASE_URL`，并以绑定模型定义覆盖规则。

现有 `DatabaseConnection` 是按 Endpoint 和创建者隔离的数据库工作台数据。阶段 6 必须先解决“项目共享的部署绑定”与“个人工作台连接”之间的授权/密文边界，不能把其他用户的连接或密码直接暴露给项目成员。数据库密码与生成的完整 URL 均属于敏感数据；只能在发布执行期间从受控存储解析后注入，不得持久化到 Release、普通配置、日志或审计中。

本阶段还包含 OSS 兼容与容量/失败诊断增强。独立阿里云 OSS adapter 只有在真实目标环境无法通过已支持的 S3 Compatible 接口接入，且用户确认要支持该场景时才实现；不能因缺少真实 OSS 证据擅自引入厂商 SDK 或猜测协议。

### 5.2 批次总览

| 批次 | 名称 | 核心交付 | 完成标志 |
| --- | --- | --- | --- |
| 6.1 | 基线审计与数据库绑定契约 | 工作台/配置/密文审计、绑定和注入规则、进度文档 | 个人连接与项目绑定边界明确 |
| 6.2 | 项目数据库资源与绑定模型 | 平台连接引用/绑定、密文引用、快照、dataservice/迁移 | 不复制或明文保存密码 |
| 6.3 | 绑定、预检与工作台关联 API | 服务绑定 CRUD、连接测试、角色/Endpoint 校验、审计 | 服务端隔离和引用保护有效 |
| 6.4 | 发布变量解析与安全注入 | 六类变量、覆盖规则、快照 hash、失败 reason | 密文只在运行期出现 |
| 6.5 | OSS 兼容 Gate 与必要适配 | S3 Compatible 实证；必要时独立 adapter | 不以假兼容冒充真实支持 |
| 6.6 | 容量提醒与失败诊断 API | 制品/存储容量统计、阈值、统一可操作 reason | 不泄露物理路径或凭据 |
| 6.7 | 后端验收与外部演练记录 | 数据库/OSS/容量/发布联合验证和验收记录 | 可合并到 `develop` |

### 5.3 分批实施要求

#### 6.1 基线审计与数据库绑定契约

目标：审计 `api/http/handler/endpoints` 的环境级数据库连接与直接查询、容器级旧入口、阶段 2 ConfigSet/Secret、Release ConfigSnapshot、现有加密能力和数据导入导出行为。

范围：新增 `docs/阶段6实施进度.md`；冻结平台数据库资源和 ServiceDeployment 绑定的归属、生命周期、引用权限、默认变量名、显式覆盖、URL 生成、连接测试、删除/归档保护、发布快照、审计和错误 reason。明确数据库工作台仍通过环境级 Host/Port 直连；ContainerID 只允许作为显示/自动填充来源。列出旧明文密码数据的兼容/迁移策略，未满足加密条件时不得新增明文持久化。

验收：明确个人工作台连接、项目可部署数据库资源、服务绑定和 runtime secret 的差异；明确数据库与 Redis 的支持矩阵、变量 URL 格式和失败时不影响当前 Release 的规则。

建议提交：`docs: 固化阶段6数据库绑定方案`。

#### 6.2 项目数据库资源与绑定模型

目标：通过平台专用引用模型让项目中的服务可绑定数据库，而非直接复用某个用户的私有连接记录。

范围：按 6.1 契约新增平台数据库资源/受控凭据引用、ServiceDeployment 数据库绑定、变量映射、版本和 Release 快照；必要时复用阶段 2 的加密 secret 服务保存凭据。资源需关联 Project、PlatformEnvironment 和 Endpoint，并保存数据库类型、Host、Port、库名、用户名、连接超时等非敏感元数据；密文仅保存安全引用/密文，不序列化为 API 响应或 Export 明文。删除、归档、修改须保护活动绑定、进行中发布和历史追溯。

验收：模型/dataservice/迁移测试覆盖创建、更新、绑定、解绑、资源版本冲突、敏感字段脱敏、旧 DatabaseConnection 兼容导入策略和历史 Release 可读性。

建议提交：`feat: 新增平台数据库绑定模型`。

#### 6.3 绑定、预检与工作台关联 API

目标：提供后端 API 管理数据库资源和服务绑定，并让未来前端能安全打开正确的环境级数据库工作台。

范围：实现资源/绑定 CRUD、测试连接、绑定可用性预检、从服务部署解析可跳转的数据库工作台上下文 API（只返回授权所需 ID/元数据，不返回密码）。所有操作校验项目角色、数据库资源所属 Endpoint 的权限交集和环境/项目一致性；测试连接有查询超时和固定失败 reason。被绑定资源禁止无确认删除；绑定变更更新配置漂移，但不自动重启服务。

验收：handler 测试覆盖无项目权限、无 Endpoint 权限、跨项目/跨环境、旧 container scope 连接、连接不可达、密码不回显、删除引用阻断和审计。数据库查询接口本身不因绑定而放宽 SQL 操作权限。

建议提交：`feat: 支持平台服务绑定数据库连接`。

#### 6.4 发布变量解析与安全注入

目标：将经授权的数据库绑定解析为部署环境变量，保持阶段 2 配置优先级、SecretSnapshots 和失败恢复语义。

范围：在 Release 预检/执行器中解析绑定，生成六类变量及按数据库类型冻结的 URL 格式；定义与项目/环境/服务 ConfigSet 的覆盖冲突规则，禁止用户用普通明文配置伪造敏感 `DATABASE_PASSWORD`。发布快照只保存绑定 ID、版本、变量名和不含明文的 hash；实际密码/URL 仅在构造 Docker runtime 配置的短生命周期内使用，并在错误路径清空。连接预检失败、缺失密码、变量冲突或密文解密失败应在切流前失败且保留旧 Release。

验收：测试覆盖 MySQL/MariaDB、PostgreSQL、Redis 的变量生成、覆盖冲突、空值、密文解密失败、预检超时、发布/回滚快照和 Docker runtime 配置中实际注入；敏感词扫描不能发现密码或完整 URL 出现在 Release/审计/响应。

建议提交：`feat: 发布流程支持数据库变量注入`。

#### 6.5 OSS 兼容 Gate 与必要适配

目标：以证据决定是否只保留 S3 Compatible 路径，或为已确认的阿里云 OSS 场景新增独立后端 adapter。

范围：先对已配置的目标 OSS endpoint 执行连接、列举、Head、下载、超时、TLS、路径前缀和 SHA256 验证，并记录在阶段进度/验收文档。若 S3 Compatible 完全通过，仅补兼容证据、错误分类或测试；若目标确认无法兼容且用户明确要求独立支持，才实现最小 OSS adapter、受控凭据存储、脱敏错误和 fake/集成测试。任何 adapter 必须复用 ArtifactStorage 接口，不得影响 MinIO/S3 行为。

验收：没有真实凭据时只完成本地 adapter 契约/fake 测试并明确待演练项；不得把无证据的“可能支持”写成已支持。

建议提交：`feat: 增强平台对象存储兼容`。

#### 6.6 容量提醒与失败诊断 API

目标：为原始制品长期保留、对象存储和发布故障提供后端可用的容量和诊断事实，供未来界面消费。

范围：实现项目/环境维度的制品大小聚合、可清理候选统计、受控本地存储预算/阈值、对象存储可得容量（仅 provider 安全支持时）和发布/拉取/推送/网关/数据库的稳定失败分类查询。统计必须有分页、时间范围、权限隔离和缓存/超时；不能扫描任意宿主机文件系统、暴露真实目录、把 provider 错误原文直接返回。

验收：测试覆盖阈值边界、授权隔离、无可用容量 API、计数与已归档/已引用制品一致、失败 reason 聚合与数据脱敏。

建议提交：`feat: 增加平台容量与失败诊断`。

#### 6.7 后端验收与外部演练记录

目标：收口阶段 6 的数据库绑定、OSS 兼容和容量增强，确认它们没有破坏原有数据库工作台或发布恢复链路。

范围：新增 `docs/阶段6验收记录.md`；运行数据库 handler、platform handler/service/dataservice、Release executor 和相关全量回归；记录 MySQL/MariaDB、PostgreSQL、Redis、带发布端口的本地 Agent、MinIO/OSS、密码脱敏、注入、回滚和容量阈值演练。

验收：前端无改动；测试与扫描通过；真实数据库/OSS 不可用时记录精确的外部待办而非标注为“通过”。

建议提交：`test: 完成阶段6数据库后端验收`。

## 6. 阶段 7：V1.x 增强能力的范围冻结 Gate

### 6.1 为什么必须先暂停编码

PRD 仅列出“HTTP 灰度、ACME、Prometheus/Loki/Grafana、Kubernetes 适配”作为后续增强，没有指定本轮要交付哪一项、目标环境、协议/供应商、权限、数据保留、故障补偿和验收边界。它们是相互独立且风险显著不同的后端项目，不能以“阶段 7”名称推断为一次性全部实现。

因此阶段 7 在用户确认范围前只能完成 7.1。7.1 完成后，Codex 必须向用户报告缺少的选择，并询问是否需要把确认结果同步更新 PRD；在用户答复且按答复完成必要 PRD 更新前，不得创建 7.2 之后的业务代码提交，也不得把阶段 7 合并回 `develop`。

### 6.2 预设的后端子工作包（仅供确认后采用）

| 子工作包 | 可拆分批次 | 需冻结的关键决定 |
| --- | --- | --- |
| 7A HTTP 灰度 | 模型/策略 → Nginx 权重渲染 → Release 状态机/回滚 → 审计与演练 | 服务版本并存数量、权重精度、放量/暂停/回退规则、健康门槛、是否只支持单网关 |
| 7B ACME | 证书账号/订单模型 → DNS/HTTP-01 provider → 签发/续期 worker → 网关证书原子替换 | challenge 类型、DNS provider、账号/私钥存储、续期窗口、失败告警、离线私有环境限制 |
| 7C 可观测性 | 连接配置模型 → Prometheus/Loki/Grafana 查询 adapter → 授权/聚合 API → 超时/脱敏/演练 | 外部地址和认证、查询白名单、标签约定、保留期、日志访问权限、是否只集成不部署组件 |
| 7D Kubernetes | 目标/命名空间模型 → Kubernetes runtime adapter → 发布/回滚/健康 → Gateway/Ingress 兼容 | 支持的工作负载（Deployment/CronJob）、Namespace 策略、凭据/Endpoint、Service/Ingress 策略、ConfigMap/Secret 映射、回滚语义 |

### 6.3 批次 7.1：增强范围审计与契约冻结

目标：新增 `docs/阶段7实施进度.md`，审计阶段 4 至阶段 6 的完成状态、真实演练待办、用户反馈和现有 Portainer 能力，为用户选择的一个或多个子工作包产出可执行后端契约。

范围：记录候选子工作包、依赖阶段、数据模型、权限边界、外部服务、失败不影响线上版本的补偿策略、非目标、验收用例、预计批次和风险；不能修改业务后端、前端、路由或第三方依赖。

停止条件：若 PRD/用户没有明确本轮包含的子工作包和关键决定，提交 `docs: 固化阶段7增强范围待确认项` 后停止并向用户提问。用户确认后先询问是否同步更新 PRD，并按答复更新必要文档，然后再创建或继续阶段 7 后续批次。

## 7. Goal 模式总控提示词

在 Codex 中创建 Goal 后，复制以下提示词。它会按阶段顺序建立/合并分支；阶段 7 会在范围 Gate 处按规则停下等待确认，而不是越权猜测。

```text
目标：根据 docs/阶段4-7分批实施方案与Goal模式提示词.md，顺序完成阶段 4、阶段 5、阶段 6 的后端开发，并在每个阶段完成后将该阶段分支合并回本地 develop。随后启动阶段 7 的范围冻结；只有用户确认阶段 7 的具体增强子范围并完成必要 PRD 同步后，才继续阶段 7 后端编码。全程不改动任何前端文件。

开始前必须读取并遵守：
- AGENTS.md
- docs/阶段4-7分批实施方案与Goal模式提示词.md
- docs/私有化微服务部署平台产品需求文档.md
- docs/功能优先级与工作量排期.md
- docs/阶段0平台核心模型与后端边界设计.md
- docs/阶段3实施进度.md
- docs/阶段3验收记录.md
- 已存在的 docs/阶段4实施进度.md 至 docs/阶段7实施进度.md 和验收记录

总红线：
1. 后端优先：只改 api/、必要后端路由注册、后端测试、必要迁移、docs/ 与直接相关的后端构建文件。不得改 app/react/、app/ 前端代码、translations/、前端路由、样式或前端测试。
2. 不实现 Git/源码构建、任意 Dockerfile/shell、内置 registry、完整 CI/CD、审批流、漏洞扫描、数据库实例创建或未确认的阶段 7 功能。
3. 不得泄漏密码、数据库 URL 密码部分、私钥、证书私钥、对象存储凭据、签名 URL、真实临时路径或内部拓扑。所有外部错误转换为稳定 reason；审计只记录脱敏摘要。
4. handler 事务内不得执行 Docker/Agent、Nginx、网络探测、对象存储、数据库连接测试或 Kubernetes 等耗时外部操作。
5. 不回滚、覆盖、暂存或提交用户/其他协作者的改动；不使用 git reset --hard、强制推送或历史重写。

阶段分支流程（阶段 4、5、6、7 每次都重复）：
1. 运行 git status --short --branch、git branch --show-current、git log --oneline -n 30，并读取对应阶段进度文档。
2. 若当前工作区不干净且改动不是本 Goal 已确认的本阶段改动，停止并报告文件清单；不要猜测归属。
3. 确认本地 develop 存在后执行 git switch develop，再次确认工作区干净；不要自行 git pull、rebase 或 reset。
4. 若 codex/platform-phase<N> 不存在，执行 git switch -c codex/platform-phase<N>；必须从当前 develop 创建。若它已存在，先审计 merge-base、提交历史和进度证据，只有确认它是本阶段未完成分支才可切换继续。
5. 按本文档中阶段 <N> 的批次顺序工作。每批先审计缺口与并行改动，使用 update_plan 跟踪；实现后运行相关 Go 测试、git diff --check、敏感信息/安全边界扫描，更新 docs/阶段<N>实施进度.md，并只提交该批相关文件，提交信息使用中文。
6. 每阶段全部批次完成后，新增或更新 docs/阶段<N>验收记录.md，确认 git diff --name-only develop...HEAD 不含 app/react/、translations/ 或其他前端目录，且所有阶段验收测试通过。
7. 只有达到上一条后才能合并：先确保阶段分支干净，git switch develop，然后执行 git merge --no-ff codex/platform-phase<N> -m "merge: 合并阶段<N>后端实施"。合并后在 develop 重跑相关后端测试并记录结果。合并冲突时执行 git merge --abort，报告冲突并停止；不得强行解决不明确的协作者冲突。
8. 只有合并成功且 develop 验证通过，才进入下一阶段并重新从 develop 创建新分支。不要直接从上一阶段特性分支创建下一阶段分支。

阶段顺序和批次：
- 阶段 4：4.1 基线审计与网关契约冻结；4.2 网关模型/dataservice；4.3 受控 Nginx 适配器；4.4 域名路径手动证书 API；4.5 渲染、nginx -t、原子 reload；4.6 Release 网关切流/回滚；4.7 后端验收。
- 阶段 5：5.1 多主机契约；5.2 多目标/主机组/策略模型；5.3 API 与 gateway-to-workload 连通性预检；5.4 多目标执行器；5.5 分批暂停/回滚；5.6 多 upstream 网关；5.7 后端验收。
- 阶段 6：6.1 数据库绑定契约；6.2 资源/绑定模型；6.3 绑定预检和工作台关联 API；6.4 发布变量安全注入；6.5 OSS 兼容 Gate/必要 adapter；6.6 容量和失败诊断 API；6.7 后端验收。
- 阶段 7：只先实施 7.1 增强范围审计与契约冻结。若用户未确认本轮要做的 7A HTTP 灰度、7B ACME、7C 可观测性、7D Kubernetes 中哪几项以及关键决策，提交范围文档后停下向用户确认，并询问是否同步更新 PRD；不能自动编码、不能合并阶段 7。

核心实现规则：
- 阶段 4 的 Nginx 只能是平台受控容器/受控文件；所有配置先候选写入、nginx -t、原子激活、reload，失败必须保持或恢复旧活动配置和旧流量。路由/证书/Release 快照不可用可变当前值替代。
- 阶段 5 的中心网关只通过 hostIP:publishedPort 访问已成功的 workload target；不得使用容器内部 IP。批次失败必须暂停后续批，逐主机 TargetResults 必须可追溯，禁止把部分成功记为全局成功。
- 阶段 6 的数据库部署绑定必须使用环境级 Host/Port 直连；ContainerID 只能作为兼容/自动填充来源。数据库密码和完整 URL 仅在发布运行期解析，Release/审计/API 中不得出现明文。独立 OSS adapter 仅在真实 S3 Compatible 不可用且用户明确要求时实现。
- 每一项外部环境能力都需要 fake/单测和真实演练清单；没有 Docker、Nginx、Agent、数据库、MinIO/OSS 或 Kubernetes 环境时，记录待演练项，不能编造成功。

完成判定：
- 阶段 4/5/6：所有批次已提交、阶段验收记录完整、阶段分支已经非快进合并回 develop，且合并后的相关后端测试通过。
- 阶段 7：只有用户确认具体子范围、必要 PRD 同步完成、确认范围的所有批次按同一分支流程完成并合并后，才能把整个 Goal 标记 complete。

现在开始：先审计 Git 状态和阶段 4 前置证据，然后按上述分支流程从 develop 创建 codex/platform-phase4，执行批次 4.1。
```

## 8. Goal 模式续跑提示词

```text
继续“阶段 4-7 后端实施”Goal。

先读取 docs/阶段4-7分批实施方案与Goal模式提示词.md、当前阶段的实施进度/验收记录、PRD、排期、AGENTS.md，并运行：
- git status --short --branch
- git branch --show-current
- git log --oneline -n 30
- git log --oneline --decorate --graph -n 40

然后按证据判断状态：
1. 若某阶段分支尚未创建，严格从干净的本地 develop 创建 codex/platform-phase<N>。
2. 若批次已有代码提交但缺少测试或进度证据，先补验证/文档；不要重复实现。
3. 若批次未提交，继续该批，完成实现、Go 测试、中文提交与进度记录后再进入下一批。
4. 若阶段全部完成但尚未合并，先完成验收并在阶段分支确认没有前端目录改动；然后切到 develop 进行一次 --no-ff 合并和合并后回归。
5. 若发生脏工作区、冲突、前端改动、权限/安全边界不明或 PRD 未定义的实现取舍，停止并报告，不要 reset、rebase、强制提交或猜测。
6. 若进入阶段 7，先只完成 7.1。用户没有明确确认 7A/7B/7C/7D 的具体组合与关键决定时，提交范围 Gate 文档后向用户提问，并询问是否同步更新 PRD；不要创建阶段 7 业务代码或合并阶段 7。

仍须遵守：后端优先、每阶段从 develop 建分支且完成后合回 develop、只提交本批相关文件、中文提交、外部系统操作不在事务内、所有敏感值脱敏、失败不能影响当前线上 Release。
```

## 9. 合并前最小验证清单

每个阶段合并前至少执行并记录与改动相匹配的命令。Go 缓存权限异常时可使用仓库约定的 `.tmp\gocache`。

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.tmp\gocache'
E:\develop\Go\bin\go.exe test ./api/platform ./api/http/handler/platform ./api/datastore -count=1
git diff --check
git diff --name-only develop...HEAD
git status --short --branch
```

按阶段增加的最低验证：

| 阶段 | 必须额外覆盖 |
| --- | --- |
| 4 | 网关 renderer/adapter、路由/证书 handler、Release 网关切流/恢复、配置注入与私钥泄漏扫描 |
| 5 | multi executor、批次暂停/取消/恢复、TargetResults、Endpoint 权限、gateway→workload 连通性和多 upstream 回归 |
| 6 | 环境级数据库 handler、数据库绑定/注入、敏感变量/完整 URL 泄漏扫描、ArtifactStorage adapter、容量统计 |
| 7 | 仅执行经用户确认子工作包的模型、handler、adapter、权限、失败补偿和真实外部演练；不得以未确认项目的测试替代 |

真实环境未具备时，应在阶段验收记录中列出命令/场景、所缺系统和预期证据。例如阶段 4 至少需要单机 Nginx；阶段 5 至少需要一个 gateway 和两个 Docker/Agent workload target；阶段 6 至少需要 MySQL/MariaDB、PostgreSQL 或 Redis 之一，以及目标 MinIO/OSS；阶段 7 取决于用户确认的子工作包。
