# S4 远程 Agent 环境准备清单

版本：v0.1
日期：2026-07-11
状态：已完成实测
关联台账：[Gate 0B Docker/Agent Spike 实测记录](./gate0b-spike-record.md)

## 1. 目标

S4 用于验证远程 Agent 场景，必须证明 Portainer 后端所在主机与远程 Agent 目标宿主机不在同一网络假设下，candidate 随机端口的可达和不可达行为都能被稳定识别。

S4 已于 2026-07-11 完成远程 Agent 实测，证据位于 `docs/spike/evidence/gate0b/S4-remote-agent/`。Gate 0B 通过结论以 [Gate 0B Docker/Agent Spike 实测记录](./gate0b-spike-record.md) 为准。

## 2. 需要准备的环境信息

| 字段 | 示例 | 是否必需 | 说明 |
| --- | --- | --- | --- |
| `RemoteAgentURL` | `https://agent.example.internal:9001` | 是 | 远程 Agent API 地址，需能从当前 Portainer 主机访问 `/ping` |
| `NodeName` | `docker-node-01` | 条件必需 | 单节点可为空；多节点或 Swarm Agent 必须明确目标节点 |
| `CandidateHealthURL` | `http://agent-host.example.internal:32780/` | 是 | 远程目标宿主机上 candidate 随机宿主机端口的可达地址 |
| `BlockedCandidateHealthURL` | `http://agent-host.example.internal:32781/` | 建议 | 被防火墙阻断或不可达的 candidate 地址，用于固化 `HEALTHCHECK_HOST_UNREACHABLE` |
| 网络拓扑说明 | Portainer 主机、Agent 主机、目标 Docker 主机 | 是 | 记录是否同网段、是否跨防火墙、NAT 或 VPN |
| 防火墙动作 | 放行 / 阻断 candidate 随机端口 | 是 | 至少需要一次放行证据；建议补一次阻断证据 |

不得把生产密码、token、私钥、完整认证头或敏感内网拓扑提交到证据目录。

## 3. 执行命令

预演，不执行 HTTP 探测：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-remote-agent-spike.ps1
```

执行远程 Agent `/ping` 和 candidate 放行探测：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-remote-agent-spike.ps1 `
  -Apply `
  -InsecureTls `
  -RemoteAgentURL https://<remote-agent-host>:9001 `
  -NodeName <target-node-name> `
  -CandidateHealthURL http://<remote-agent-target-host>:<candidate-random-port>/
```

执行放行和阻断对照探测：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-remote-agent-spike.ps1 `
  -Apply `
  -InsecureTls `
  -RemoteAgentURL https://<remote-agent-host>:9001 `
  -NodeName <target-node-name> `
  -CandidateHealthURL http://<allowed-host>:<candidate-random-port>/ `
  -BlockedCandidateHealthURL http://<blocked-host>:<candidate-random-port>/
```

如果远程 Agent 使用可信 TLS 证书，可以去掉 `-InsecureTls`。

## 4. 通过标准

| 项 | 通过条件 | 证据文件 |
| --- | --- | --- |
| Agent 可识别 | `/ping` 返回 HTTP 204 | `remote-agent-ping.txt` |
| Agent 平台 | 响应头 `Portainer-Agent-Platform: 1` | `remote-agent-ping.txt` |
| Agent 版本 | 响应头包含 `Portainer-Agent` | `remote-agent-ping.txt` |
| 目标节点 | 单节点明确为空；多节点明确 `NodeName` | `remote-agent-summary.txt` |
| candidate 放行 | `CandidateHealthURL` 返回 HTTP 200 | `candidate-health-allowed.txt` |
| candidate 阻断 | 不可达或非 2xx，可稳定映射为 `HEALTHCHECK_HOST_UNREACHABLE` | `candidate-health-blocked.txt` |

## 5. 回填要求

S4 完成后需要同步更新：

- `docs/spike/gate0b-spike-record.md`
- `docs/阶段1实施进度.md`
- 如 S4 结论改变默认策略，还需要更新 `docs/Gate0B-Docker-Agent-Spike执行方案.md`

S4 已完成实测并回填；后续若更换远程 Agent 网络拓扑，应按本清单重新采集证据。
