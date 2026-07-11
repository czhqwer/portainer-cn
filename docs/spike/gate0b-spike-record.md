# Gate 0B Docker/Agent Spike 实测记录

版本：v0.1
日期：2026-07-11
状态：未通过，等待实测
关联方案：[Gate 0B Docker/Agent Spike 执行方案](../Gate0B-Docker-Agent-Spike执行方案.md)

## 1. 总览

本文件用于沉淀 Gate 0B 的真实执行证据。所有场景完成并明确通过前，Gate 0B 仍视为未通过，不得开始 Docker 发布执行器正式编码。

| 场景编号 | 场景 | 状态 | 证据位置 | 结论 |
| --- | --- | --- | --- | --- |
| S1 | Portainer 后端直接运行在宿主机 + Docker socket | 未执行 | - | 待定 |
| S2 | 容器化 Portainer + Docker socket | 未执行 | - | 待定 |
| S3 | 本地 Agent | 未执行 | - | 待定 |
| S4 | 远程 Agent | 未执行 | - | 待定 |
| S5 | 版本化容器命名 | 未执行 | - | 待定 |
| S6 | 正式端口切换 | 未执行 | - | 待定 |
| S7 | 私有 registry 认证和 digest 解析 | 未执行 | - | 待定 |

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

## 3. 默认策略记录

| 决策点 | 结论 | 证据 | 状态 |
| --- | --- | --- | --- |
| `HealthCheckHost` 默认值 | 待定 | - | 未执行 |
| Agent `NodeName` 默认选择 | 待定 | - | 未执行 |
| candidate 随机端口解析 | 待定 | - | 未执行 |
| 私有镜像凭据优先级 | 待定 | - | 未执行 |
| digest 解析时机 | 待定 | - | 未执行 |
| 旧容器恢复状态映射 | 待定 | - | 未执行 |

## 4. Reason 映射记录

| 失败场景 | 期望 reason | 实测 reason | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| candidate 地址不可达 | `HEALTHCHECK_HOST_UNREACHABLE` | 待定 | - | 未执行 |
| 健康检查失败 | `HEALTHCHECK_FAILED` | 待定 | - | 未执行 |
| 私有 registry 凭据错误 | `REGISTRY_AUTH_FAILED` | 待定 | - | 未执行 |
| 镜像拉取失败 | `IMAGE_PULL_FAILED` | 待定 | - | 未执行 |
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

状态：未执行

本场景可用辅助脚本预演：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Apply
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Cleanup -Apply
```

待补证据：

- Docker version 和 context。
- candidate 随机端口映射。
- 后端访问 candidate 临时端口的健康检查结果。
- candidate 验证后停止并删除证据。

## 7. S2 容器化 Portainer + Docker socket

状态：未执行

待补证据：

- Portainer 容器网络信息。
- `127.0.0.1`、Docker bridge gateway、`host.docker.internal` 和显式 `HealthCheckHost` 的可达性对比。
- 不可自动推断时的 UI/API 配置路径。

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

状态：未执行

待补证据：

- `pcn-{projectSlug}-{envSlug}-{serviceSlug}-r{releaseId}-candidate` 命名验证。
- `pcn-{projectSlug}-{envSlug}-{serviceSlug}-r{releaseId}` 正式容器命名验证。
- 旧容器存在时，新正式容器不发生固定名称冲突的证据。

## 11. S6 正式端口切换与旧容器恢复

状态：未执行

待补证据：

- 停旧、启新、final-checking 证据。
- 新正式容器启动失败后的旧容器恢复证据。
- 恢复失败时状态和 reason 映射。
- 中断时间记录方式。

## 12. S7 私有 registry 认证和 digest 解析

状态：未执行

待补证据：

- 正确凭据拉取私有镜像成功。
- 错误凭据稳定映射为 `REGISTRY_AUTH_FAILED`。
- 镜像不存在或网络失败稳定映射为 `IMAGE_PULL_FAILED`。
- digest 解析成功、超时和失败策略。

## 13. Gate 0B 结论

当前结论：未通过。

通过前必须同时满足：

- S1 到 S7 均有实测记录和证据路径。
- 默认策略记录全部完成。
- reason 映射记录全部完成。
- 阶段 0 设计或实现 ADR 已回填结论。
- `docs/阶段1实施进度.md` 已更新 Gate 0B 通过状态。
