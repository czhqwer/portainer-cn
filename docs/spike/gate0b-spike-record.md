# Gate 0B Docker/Agent Spike 实测记录

版本：v0.1
日期：2026-07-11
状态：未通过；S1/S2/S6 本地 Docker socket 子集已实测通过，S7 私有 registry 子集通过，S5 本地命名子集已部分验证，S3/S4 仍等待实测
关联方案：[Gate 0B Docker/Agent Spike 执行方案](../Gate0B-Docker-Agent-Spike执行方案.md)

## 1. 总览

本文件用于沉淀 Gate 0B 的真实执行证据。所有场景完成并明确通过前，Gate 0B 仍视为未通过，不得开始 Docker 发布执行器正式编码。

| 场景编号 | 场景 | 状态 | 证据位置 | 结论 |
| --- | --- | --- | --- | --- |
| S1 | Portainer 后端直接运行在宿主机 + Docker socket | 本地子集通过 | `docs/spike/evidence/gate0b/S1-local-docker/` | Docker socket 可用，candidate 随机端口健康检查 200；清理证据见 `docs/spike/evidence/gate0b/S1-local-docker-cleanup/` |
| S2 | 容器化 Portainer + Docker socket | 本地子集通过 | `docs/spike/evidence/gate0b/S2-containerized-docker/` | 容器内 Docker socket 可用；`127.0.0.1` 不可达，bridge gateway 和 `host.docker.internal` 均可访问 candidate 随机端口 |
| S3 | 本地 Agent | 未执行 | - | 待定 |
| S4 | 远程 Agent | 未执行 | - | 待定 |
| S5 | 版本化容器命名 | 部分通过 | `docs/spike/evidence/gate0b/S1-local-docker/` | 本地辅助容器 r1/r2 命名与固定端口冲突规避流程通过；正式 `project/env/service/release` 命名模板仍待实现前复核 |
| S6 | 正式端口切换 | 本地子集通过 | `docs/spike/evidence/gate0b/S1-local-docker/` | 旧容器停止、新容器占用正式端口、坏镜像失败后旧容器恢复均完成；恢复后健康检查 200 |
| S7 | 私有 registry 认证和 digest 解析 | 通过 | `docs/spike/evidence/gate0b/S7-private-registry/` | 正确凭据 push/pull 成功，错误凭据 401，缺失镜像 manifest unknown，pull 后 RepoDigests 可解析私有 digest |

## 2. 证据目录约定

实测证据建议放在 `docs/spike/evidence/gate0b/<场景编号>/` 下。证据文件可包含：

- `commands.txt`：执行命令或 API 请求。
- `metadata.txt`：场景、镜像、端口、执行时间等脚本元信息。
- `transcript.txt`：PowerShell 执行 transcript。
- `docker-version.txt`：Docker client/server 版本。
- `docker-context.txt`：Docker context 或 endpoint 信息。
- `containers-before.txt`、`containers-after.txt`：测试前后容器列表。
- `candidate-inspect.json`：candidate inspect 摘要。
- `official-inspect.json`：正式容器 inspect 摘要。
- `healthcheck.txt`：健康检查 URL、状态码、响应摘要。
- `error.txt`：失败错误摘要和 reason 映射。
- `notes.md`：人工观察、网络拓扑、端口和防火墙说明。

不得提交含有 registry 密码、token、私钥、完整认证头或生产环境敏感地址的证据。

本地 Docker 辅助脚本默认只记录带 `com.portainer-cn.platform.spike=gate0b` label 的 helper 容器列表；只有显式传入 `-RecordAllContainers` 时才会记录完整容器列表。若记录完整列表，提交前必须检查是否包含无关业务容器名称、端口或敏感拓扑。

## 3. 默认策略记录

| 决策点 | 结论 | 证据 | 状态 |
| --- | --- | --- | --- |
| `HealthCheckHost` 默认值 | 宿主机后端场景可用 `127.0.0.1`；本地容器化 Docker socket 场景不能用容器内 `127.0.0.1`，可用 bridge gateway 或 `host.docker.internal`，但正式实现仍需支持显式配置 | `docs/spike/evidence/gate0b/S1-local-docker/*healthcheck.txt`、`docs/spike/evidence/gate0b/S2-containerized-docker/containerized-health-summary.txt` | 部分通过 |
| Agent `NodeName` 默认选择 | 待定 | - | 未执行 |
| candidate 随机端口解析 | 本地宿主机和容器化 Docker socket 场景均可通过 Docker 端口映射解析随机宿主机端口 | `docs/spike/evidence/gate0b/S1-local-docker/candidate-healthcheck.txt`、`docs/spike/evidence/gate0b/S2-containerized-docker/candidate-port.txt` | 部分通过 |
| 私有镜像凭据优先级 | V0.1 发布执行时优先使用 Artifact `RegistryID` 对应凭据；未显式绑定时再按镜像 registry host 匹配环境/系统 registry 配置，不能把明文凭据写入 Release snapshot 或证据 | `docs/spike/evidence/gate0b/S7-private-registry/login-success.txt`、`docs/spike/evidence/gate0b/S7-private-registry/notes.md` | 通过 |
| digest 解析时机 | 对私有 registry，V0.1 以 pull 成功后的 `RepoDigests` 作为权威 digest；`docker manifest inspect` 在本地 HTTP registry 场景可能失败，不能作为唯一来源 | `docs/spike/evidence/gate0b/S7-private-registry/private-image-repodigests.txt`、`docs/spike/evidence/gate0b/S7-private-registry/manifest-inspect.json` | 通过 |
| 旧容器恢复状态映射 | 本地坏镜像启动失败后可恢复旧容器并重新通过健康检查；正式状态枚举和 reason 映射仍待执行器实现时固化 | `docs/spike/evidence/gate0b/S1-local-docker/official-r1-recovered-healthcheck.txt` | 部分通过 |

## 4. Reason 映射记录

| 失败场景 | 期望 reason | 实测 reason | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| candidate 地址不可达 | `HEALTHCHECK_HOST_UNREACHABLE` | 待定 | - | 未执行 |
| 健康检查失败 | `HEALTHCHECK_FAILED` | 待定 | - | 未执行 |
| 私有 registry 凭据错误 | `REGISTRY_AUTH_FAILED` | 错误密码登录返回 401 Unauthorized | `docs/spike/evidence/gate0b/S7-private-registry/login-wrong-password.txt` | 通过 |
| 镜像拉取失败 | `IMAGE_PULL_FAILED` | 本地坏 tag 镜像启动失败后未破坏旧容器；私有 registry 缺失镜像返回 manifest unknown | `docs/spike/evidence/gate0b/S1-local-docker/commands.txt`、`docs/spike/evidence/gate0b/S7-private-registry/missing-image-pull.txt` | 部分通过 |
| 端口冲突 | `PORT_CONFLICT` | 待定 | - | 未执行 |
| candidate 或正式容器启动失败 | `RUNTIME_START_FAILED` | 待定 | - | 未执行 |
| 旧容器停止失败 | `RUNTIME_STOP_FAILED` | 待定 | - | 未执行 |
| 旧容器恢复失败 | `RECOVERY_FAILED` | 待定 | - | 未执行 |

## 5. 场景记录模板

复制以下模板到对应场景小节，并补齐证据：

```text
执行日期：
执行人：
场景编号：
环境描述：
Portainer 运行方式：
Docker endpoint：
Agent endpoint / NodeName：
HealthCheckHost：
镜像：
registry：
执行命令或 API：
成功证据：
失败证据：
reason 映射：
是否通过：
结论：
```

## 6. S1 本机后端 + Docker socket

状态：本地 Docker socket 子集通过。

执行日期：2026-07-11
执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Apply
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Cleanup -Apply -EvidenceRoot docs/spike/evidence/gate0b/S1-local-docker-cleanup
```

证据：

- `docs/spike/evidence/gate0b/S1-local-docker/docker-version.txt`
- `docs/spike/evidence/gate0b/S1-local-docker/docker-context.txt`
- `docs/spike/evidence/gate0b/S1-local-docker/candidate-inspect.json`
- `docs/spike/evidence/gate0b/S1-local-docker/candidate-healthcheck.txt`
- `docs/spike/evidence/gate0b/S1-local-docker/containers-after.txt`
- `docs/spike/evidence/gate0b/S1-local-docker-cleanup/containers-after-cleanup.txt`

结论：

- Docker socket 可用，`nginx:alpine` 可拉取。
- candidate 使用随机宿主机端口，脚本可解析端口并从 `127.0.0.1` 健康检查得到 HTTP 200。
- candidate 验证后已删除；清理后 helper 容器列表为空。
- 本结论仅覆盖宿主机后端直连 Docker socket，不覆盖容器化 Portainer、Agent 或远程 Agent。

## 7. S2 容器化 Portainer + Docker socket

状态：本地容器化 Docker socket 子集通过。

执行日期：2026-07-11
执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-containerized-docker-spike.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-containerized-docker-spike.ps1 -Apply
```

证据：

- `docs/spike/evidence/gate0b/S2-containerized-docker/container-docker-socket.txt`
- `docs/spike/evidence/gate0b/S2-containerized-docker/candidate-port.txt`
- `docs/spike/evidence/gate0b/S2-containerized-docker/containerized-health-summary.txt`
- `docs/spike/evidence/gate0b/S2-containerized-docker/probe-loopback.txt`
- `docs/spike/evidence/gate0b/S2-containerized-docker/probe-bridge-gateway.txt`
- `docs/spike/evidence/gate0b/S2-containerized-docker/probe-host-docker-internal.txt`
- `docs/spike/evidence/gate0b/S2-containerized-docker/containers-after.txt`

结论：

- Docker CLI 临时容器挂载 `/var/run/docker.sock` 后可以访问 Docker Engine。
- candidate 使用随机宿主机端口，脚本可解析端口。
- 容器内 `127.0.0.1` 访问 candidate 随机宿主机端口失败，符合预期。
- Docker bridge gateway `172.17.0.1` 和 `host.docker.internal` 均能从 probe 容器访问 candidate 随机宿主机端口并返回 HTTP 200。
- 本结论只覆盖本地 Docker Desktop + bridge 网络子集；正式实现不能硬编码该地址，仍需支持显式配置 `HealthCheckHost`，Agent 和远程 Agent 仍待实测。

## 8. S3 本地 Agent

状态：未执行

待补证据：

- Agent endpoint、目标节点名、`X-PortainerAgent-Target` 或等价后端配置。
- candidate 随机端口从 Portainer 后端到 Agent 目标宿主机的可达性。
- 不可达时是否要求用户选择 `startup-only` 后重试。

## 9. S4 远程 Agent

状态：未执行

待补证据：

- Portainer 主机、Agent 主机和防火墙拓扑。
- 放行和阻断随机端口时的健康检查结果。
- `HEALTHCHECK_HOST_UNREACHABLE` 的稳定 reason 映射。

## 10. S5 版本化容器命名

状态：部分通过。

已验证：

- 本地辅助脚本使用 `pcn-spike-gate0b-candidate`、`pcn-spike-gate0b-r1`、`pcn-spike-gate0b-r2` 验证 candidate、旧正式容器和新正式容器的分离命名。
- 旧容器 `r1` 停止后，新容器 `r2` 可占用同一正式端口，坏镜像失败后可恢复 `r1`。

待补证据：

- `pcn-{projectSlug}-{envSlug}-{serviceSlug}-r{releaseId}-candidate` 正式命名模板验证。
- `pcn-{projectSlug}-{envSlug}-{serviceSlug}-r{releaseId}` 正式容器命名模板验证。
- 多项目、多环境、多服务 slug 截断、冲突和非法字符归一化策略。

## 11. S6 正式端口切换与旧容器恢复

状态：本地 Docker socket 子集通过。

证据：

- `docs/spike/evidence/gate0b/S1-local-docker/official-r1-healthcheck.txt`
- `docs/spike/evidence/gate0b/S1-local-docker/official-r2-healthcheck.txt`
- `docs/spike/evidence/gate0b/S1-local-docker/official-r1-recovered-healthcheck.txt`
- `docs/spike/evidence/gate0b/S1-local-docker/commands.txt`

结论：

- 旧正式容器 `r1` 占用 `18080:80` 时健康检查 HTTP 200。
- 停止 `r1` 后，新正式容器 `r2` 可占用 `18080:80` 并健康检查 HTTP 200。
- 模拟坏镜像启动失败后，脚本删除失败的新容器并重新启动 `r1`，恢复后健康检查 HTTP 200。
- 恢复失败时的状态和 reason 映射仍待正式执行器实现时补齐。

## 12. S7 私有 registry 认证和 digest 解析

状态：通过。

执行日期：2026-07-11
执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-private-registry-spike.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-private-registry-spike.ps1 -Apply
```

证据：

- `docs/spike/evidence/gate0b/S7-private-registry/login-success.txt`
- `docs/spike/evidence/gate0b/S7-private-registry/login-wrong-password.txt`
- `docs/spike/evidence/gate0b/S7-private-registry/private-push.txt`
- `docs/spike/evidence/gate0b/S7-private-registry/private-pull.txt`
- `docs/spike/evidence/gate0b/S7-private-registry/private-image-repodigests.txt`
- `docs/spike/evidence/gate0b/S7-private-registry/missing-image-pull.txt`
- `docs/spike/evidence/gate0b/S7-private-registry/registry-summary.txt`

结论：

- 正确凭据可登录 `127.0.0.1:5000` 并 push/pull 私有镜像。
- 错误凭据返回 401 Unauthorized，映射为 `REGISTRY_AUTH_FAILED`。
- 缺失镜像返回 `manifest unknown`，映射为 `IMAGE_PULL_FAILED`。
- pull 成功后可从 `RepoDigests` 解析私有 registry digest：`127.0.0.1:5000/portainer-cn/gate0b-spike@sha256:2fabf6963cb8eb9f6806beac013d5b4c347dcb254e54aef4bdafd61fa6a06d17`。
- 本地 HTTP registry 下 `docker manifest inspect` 返回 `no such manifest`，正式实现不能只依赖 manifest inspect，应以 pull 后 RepoDigests 为权威 digest 来源。
- 脚本使用临时 Docker config，执行后已删除；证据中不包含密码、认证头或 Docker config。

## 13. Gate 0B 结论

当前结论：未通过。

2026-07-11 已完成本地 Docker socket、容器化 Docker socket 和私有 registry 子集实测，S1/S2/S6/S7 可作为本地公开/私有镜像场景的正向证据，S5 仅完成辅助命名子集验证。S3 本地 Agent、S4 远程 Agent 和正式命名策略仍未完成，Gate 0B 仍未通过，Docker 发布执行器正式编码仍不得启动。

通过前必须同时满足：

- S1 到 S7 均有实测记录和证据路径。
- 默认策略记录全部完成。
- reason 映射记录全部完成。
- 阶段 0 设计或实现 ADR 已回填结论。
- `docs/阶段1实施进度.md` 已更新 Gate 0B 通过状态。
