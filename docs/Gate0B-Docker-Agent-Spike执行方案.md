# Gate 0B Docker/Agent Spike 执行方案

版本：v0.1
日期：2026-07-11
状态：未通过，仅完成执行方案与本地代码审计
关联文档：[阶段 1 分批实施方案](./阶段1分批实施方案.md)、[阶段 0 平台核心模型与后端边界设计](./阶段0平台核心模型与后端边界设计.md)、[阶段 1 实施进度](./阶段1实施进度.md)

## 1. 结论口径

本文件不是 Gate 0B 通过结论。当前只完成 Spike 执行方案和仓库现有 Docker/Agent 能力审计。

在本文件的“实测记录”全部完成并明确通过前，不得开始 Docker 发布执行器正式编码，不得实现 `RuntimeDriver`、`ContainerAdapter`、`SingleTargetExecutor`，不得创建真实 candidate 容器、停止正式容器或执行端口切换。

## 2. 必验问题

| 编号 | 场景 | 必验问题 | 通过标准 | 当前状态 |
| --- | --- | --- | --- | --- |
| S1 | Portainer 后端直接运行在宿主机 + Docker socket | candidate 随机宿主机端口访问方式 | 后端可通过自动推断或配置访问 candidate 临时端口 | 待实测 |
| S2 | 容器化 Portainer + Docker socket | `127.0.0.1` 不可用时的宿主机地址 | 通过 `HealthCheckHost`、Docker bridge gateway 或明确配置访问 candidate | 待实测 |
| S3 | 本地 Agent | 后端到 Agent 目标宿主机随机端口可达性 | 能完成 candidate 检查，或明确要求用户显式选择 `startup-only` 后重试 | 待实测 |
| S4 | 远程 Agent | 跨主机网络与防火墙 | 健康检查失败 reason 为 `HEALTHCHECK_HOST_UNREACHABLE`，页面提示可配置地址 | 待实测 |
| S5 | 版本化容器命名 | 旧容器停止后新容器创建 | 不再出现固定容器名冲突 | 待实测 |
| S6 | 正式端口切换 | 停旧、启新、恢复旧 | 失败时可重启旧容器并记录中断时间 | 待实测 |
| S7 | 私有 registry | 拉取认证和 digest 解析 | 凭据错误返回 `REGISTRY_AUTH_FAILED`，拉取失败返回 `IMAGE_PULL_FAILED` | 待实测 |

## 3. 环境准备

需要准备四类环境：

| 环境 | 最小要求 | 需要记录 |
| --- | --- | --- |
| 本机后端 + Docker socket | 后端进程可访问 Docker socket；Docker 可运行 `nginx` 或等价 HTTP 镜像 | Docker endpoint URL、后端监听地址、candidate 随机端口访问地址 |
| 容器化 Portainer + Docker socket | Portainer 容器挂载 Docker socket；容器内 `127.0.0.1` 不等于宿主机 | bridge gateway、`host.docker.internal` 可用性、显式 `HealthCheckHost` 配置 |
| 本地 Agent | Portainer 通过本地 Agent 管理 Docker；可指定 `NodeName` | Agent URL、目标节点名、随机端口从 Portainer 后端是否可达 |
| 远程 Agent | Portainer 与 Agent 不在同一主机；防火墙可模拟放行和阻断 | Agent URL、目标主机地址、防火墙策略、失败 reason |

推荐镜像：

- 公开镜像：`nginx:alpine` 或内网可稳定拉取的等价 HTTP 镜像。
- 私有镜像：带认证 registry 中的简单 HTTP 镜像，至少包含一次正确凭据和一次错误凭据验证。

## 4. 操作矩阵

每个环境至少执行以下操作：

| 步骤 | 操作 | 需要采集的证据 | 预期 |
| --- | --- | --- | --- |
| 1 | 拉取公开镜像 | Docker pull 输出、镜像 digest 或 inspect 摘要 | 成功；失败时 reason 可归类为 `IMAGE_PULL_FAILED` |
| 2 | 拉取私有镜像，使用正确凭据 | registry 认证来源、pull 输出、digest 解析结果 | 成功；digest 可记录到 Artifact/Release 快照 |
| 3 | 拉取私有镜像，使用错误凭据 | 错误摘要、HTTP/Docker error | reason=`REGISTRY_AUTH_FAILED` 或可稳定映射到该 reason |
| 4 | 创建 candidate 容器，使用随机宿主机端口 | candidate 名称、labels、端口映射、inspect 摘要 | candidate 不占用正式端口 |
| 5 | 从 Portainer 后端访问 candidate 健康地址 | URL、状态码、响应时间、失败错误 | `verified` 成功；不可达时 reason=`HEALTHCHECK_HOST_UNREACHABLE` |
| 6 | candidate 验证后停止并删除 | stop/remove 输出、inspect 不存在证据 | candidate 不与正式容器并行提供服务 |
| 7 | 创建新正式容器，使用版本化名称和正式端口 | 新容器名、labels、端口映射、启动结果 | 不依赖固定容器名，不发生名称冲突 |
| 8 | 旧容器停止失败模拟 | Docker 错误摘要、旧容器状态 | 中止切换，旧容器继续运行，reason=`RUNTIME_STOP_FAILED` |
| 9 | 新正式容器启动失败模拟 | Docker 错误摘要、恢复动作、旧容器状态 | 尝试恢复旧容器，reason=`RUNTIME_START_FAILED` 或 `RECOVERY_FAILED` |
| 10 | 端口冲突模拟 | Docker 错误摘要、旧容器状态 | reason=`PORT_CONFLICT`，旧容器不受影响 |

### 4.1 本地 Docker socket 辅助脚本

本地 Docker socket 的公开镜像子集可以使用 `docs/spike/gate0b-local-docker-spike.ps1` 辅助执行。脚本默认 dry-run，不会运行 Docker；必须显式传入 `-Apply` 才会创建测试容器。

示例：

```powershell
# 预览将要执行的步骤，不触碰 Docker
powershell -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1

# 执行本地 Docker socket 子集：公开镜像、随机端口 candidate、版本化命名、正式端口切换、旧容器恢复
powershell -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Apply

# 指定证据目录，默认是 docs/spike/evidence/gate0b/S1-local-docker
powershell -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Apply -EvidenceRoot docs/spike/evidence/gate0b/S1-local-docker

# 默认只记录脚本创建的 helper 容器；需要完整现场容器列表时显式开启
powershell -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Apply -RecordAllContainers

# 清理脚本创建的测试容器
powershell -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Cleanup -Apply
```

`-Apply` 执行时，脚本会在证据目录写入 `metadata.txt`、`commands.txt`、`transcript.txt`、`docker-version.txt`、`docker-context.txt`、`containers-before.txt`、`containers-after.txt`、candidate / official inspect 摘要和健康检查结果。默认容器快照只包含带 `com.portainer-cn.platform.spike=gate0b` label 的 helper 容器；传入 `-RecordAllContainers` 才会记录完整容器列表。提交证据前必须检查并移除 registry 密码、token、私钥、完整认证头或生产环境敏感地址。

该脚本只覆盖 S1、S5、S6 的本地公开镜像子集，不覆盖容器化 Portainer、Agent、远程 Agent、私有 registry 和 digest 策略。完整 Gate 0B 通过仍必须补齐 S1 到 S7 的真实记录。

### 4.2 容器化后端网络辅助脚本

容器化 Portainer + Docker socket 的本地网络子集可以使用 `docs/spike/gate0b-containerized-docker-spike.ps1` 辅助执行。脚本默认 dry-run，不会运行 Docker；必须显式传入 `-Apply` 才会创建测试容器。

示例：

```powershell
# 预演，不执行 Docker
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-containerized-docker-spike.ps1

# 执行 S2 本地子集：容器内 Docker socket、candidate 随机端口、容器内健康检查地址探测
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-containerized-docker-spike.ps1 -Apply

# 清理脚本创建的 helper 容器
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-containerized-docker-spike.ps1 -Cleanup -Apply
```

该脚本使用 Docker CLI 容器验证挂载 Docker socket 后的访问能力，并使用临时 probe 容器分别探测 `127.0.0.1`、Docker bridge gateway 与 `host.docker.internal` 对 candidate 随机宿主机端口的可达性。它只覆盖 S2 的本地容器网络子集，不覆盖真实 Portainer 镜像启动、Agent、远程 Agent、私有 registry 和 digest 策略。若没有任何地址可达，应记录 `NO_CONTAINERIZED_HEALTHCHECK_HOST_REACHABLE`，并要求用户显式配置 `HealthCheckHost` 或选择允许的降级验证策略。

### 4.3 私有 Registry 辅助脚本

私有 registry 认证、错误凭据、镜像缺失和 digest 解析子集可以使用 `docs/spike/gate0b-private-registry-spike.ps1` 辅助执行。脚本默认 dry-run，不会运行 Docker；必须显式传入 `-Apply` 才会登录 registry、push/pull 测试镜像。

示例：

```powershell
# 预演，不执行 Docker
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-private-registry-spike.ps1

# 执行 S7 子集；凭据通过父 shell 环境变量传入，避免写入 transcript 或 commands.txt
$env:GATE0B_REGISTRY_USERNAME = '<registry-username>'
$env:GATE0B_REGISTRY_PASSWORD = '<registry-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-private-registry-spike.ps1 -Apply
Remove-Item Env:\GATE0B_REGISTRY_USERNAME
Remove-Item Env:\GATE0B_REGISTRY_PASSWORD
```

该脚本使用临时 Docker config 登录 registry，执行完会删除临时认证文件；证据中不得提交密码、token、认证头或 Docker config。实测至少要证明：正确凭据可 push/pull 私有镜像，错误凭据可稳定映射为 `REGISTRY_AUTH_FAILED`，镜像不存在可稳定映射为 `IMAGE_PULL_FAILED`，并能从拉取后的镜像记录 digest。

### 4.4 本地 Agent 辅助脚本

本地 Agent 子集可以使用 `docs/spike/gate0b-local-agent-spike.ps1` 辅助执行。脚本默认 dry-run，不会运行 Docker；必须显式传入 `-Apply` 才会启动本地 Agent helper 容器和 candidate。

示例：

```powershell
# 预演，不执行 Docker
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-agent-spike.ps1

# 执行 S3 本地子集：Agent /ping、Docker Agent platform、candidate 随机端口从 Portainer 主机可达
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-agent-spike.ps1 -Apply

# 清理脚本创建的 helper 容器
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-agent-spike.ps1 -Cleanup -Apply
```

该脚本启动本地 Portainer Agent，并通过 `/ping` 验证 Agent version 和 Docker platform header；随后创建 candidate 随机宿主机端口，并从当前 Portainer 主机视角验证健康检查可达性。它只覆盖 S3 的单机本地 Agent 子集；Agent Docker API 的签名请求、`X-PortainerAgent-Target` 多节点选择、远程 Agent 防火墙和跨主机端口可达性仍必须在真实 Portainer 后端或远程环境中补齐。

### 4.5 版本化命名辅助脚本

正式版本化容器命名子集可以使用 `docs/spike/gate0b-versioned-naming-spike.ps1` 辅助执行。脚本默认 dry-run，不会运行 Docker；必须显式传入 `-Apply` 才会创建旧正式容器、新 candidate 和新正式容器。

示例：

```powershell
# 预演，不执行 Docker
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-versioned-naming-spike.ps1

# 执行 S5 命名子集：正式模板、candidate 后缀、旧版本保留时新版本不发生名称冲突
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-versioned-naming-spike.ps1 -Apply

# 清理脚本创建的 helper 容器
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-versioned-naming-spike.ps1 -Cleanup -Apply
```

该脚本验证 `pcn-{projectSlug}-{envSlug}-{serviceSlug}-r{releaseId}` 和 `pcn-{projectSlug}-{envSlug}-{serviceSlug}-r{releaseId}-candidate` 命名模板，证明旧 release 容器保留时，新 release 使用不同 releaseId 不发生 Docker name 冲突。正式执行器仍应复用平台模型中已校验的 slug 字段，并在运行前拒绝空 slug、非法字符或超长名称。

## 5. 默认策略待决项

Spike 必须给出以下默认策略，不能只记录“可配置”：

| 决策点 | 需要得出的结论 |
| --- | --- |
| `HealthCheckHost` 默认值 | 各环境是否能自动推断；不能推断时 UI/API 必填规则 |
| Agent `NodeName` | 单节点、Swarm Agent、多节点 Agent 的默认目标节点选择方式 |
| candidate 随机端口 | 使用 Docker 随机端口后的 host/port 解析位置和失败 reason |
| 私有镜像凭据 | 优先使用 Artifact `RegistryID`、环境默认 registry，还是按镜像名匹配现有 registry |
| digest 解析 | 在 pull 前、pull 后或两者都执行；失败是否阻断发布 |
| 旧容器恢复 | 停旧后新容器失败、恢复失败、恢复后健康失败分别进入哪个状态 |

## 6. 当前代码审计

本地审计只用于制定 Spike，不代表 Gate 0B 通过。

| 能力 | 现有位置 | 审计结论 |
| --- | --- | --- |
| Docker client 创建 | `api/docker/client/client.go` | `CreateClient(endpoint, nodeName, timeout)` 已支持 Docker socket、TCP、Agent、Edge Agent，并能通过 `nodeName` 设置 Agent target header。 |
| Agent proxy | `api/http/proxy/factory/agent.go`、`api/http/proxy/factory/agent/transport.go` | 代理层会签名访问 Agent；但 Spike 仍需验证 candidate 随机端口从 Portainer 后端到目标主机是否可达。 |
| Docker proxy | `api/http/proxy/factory/docker/transport.go` | 代理层有权限和 registry header 装饰逻辑，但平台发布执行器不应直接走前端 proxy 假设。 |
| 镜像拉取 | `api/docker/images/puller.go` | 已有 `Puller` 可用 registry auth 拉取镜像；需要 Spike 验证错误到 `REGISTRY_AUTH_FAILED` / `IMAGE_PULL_FAILED` 的稳定映射。 |
| digest 解析 | `api/docker/images/digest.go` | 已有远程 digest 解析能力；需要验证私有 registry 凭据、超时和失败是否适合发布链路。 |
| 容器重建 | `api/docker/container.go` | 现有 `ContainerService.Recreate` 是停旧、重命名、创建新容器，再失败恢复；不符合“candidate 先验证并清理，再切正式”的阶段 1 发布流程，不能直接复用为执行器。 |

## 7. 实测记录模板

每次实测必须追加到 [Gate 0B Docker/Agent Spike 实测记录](./spike/gate0b-spike-record.md)。

| 日期 | 执行人 | 场景编号 | 环境描述 | 结果 | 证据位置 | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
| - | - | - | - | 待执行 | - | - |

单场景记录格式：

```text
场景：
环境：
Portainer 运行方式：
Docker endpoint：
Agent endpoint / NodeName：
HealthCheckHost：
镜像：
registry：
执行步骤：
命令或 API：
成功证据：
失败证据：
reason 映射：
结论：
```

## 8. Gate 0B 通过条件

只有同时满足以下条件，才能把 Gate 0B 标记为通过：

- S1 到 S7 全部有实测记录。
- `HealthCheckHost` 默认策略已确定；不可自动推断的场景有明确 UI/API 配置路径。
- 私有镜像认证和 digest 解析的成功、认证失败、拉取失败都有稳定 reason。
- candidate 使用随机端口，不占用正式端口。
- candidate 验证后会停止并删除，不与正式容器并行提供服务。
- 版本化正式容器命名不会因旧容器存在而冲突。
- 停旧失败、新容器启动失败、健康失败、端口冲突和恢复失败都有明确状态与 reason。
- 结论已回填到阶段 0 设计或实现 ADR，并更新阶段 1 实施进度。

## 9. 未通过时的处理

任一场景未通过时：

- 不得实现 Docker 发布执行器正式代码。
- Release create 继续保持 `GATE_0B_REQUIRED` 阻断。
- 前端真实发布入口继续禁用。
- 可以继续补充 Spike 文档、测试脚本、手工验证记录和设计口径，但不能创建真实执行链路。
