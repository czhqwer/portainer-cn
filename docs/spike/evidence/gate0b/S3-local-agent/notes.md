# S3 本地 Agent Spike 证据说明

执行时间：2026-07-11 18:25

执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-agent-spike.ps1 -Apply
```

说明：

- 脚本启动本地 `portainer/agent:2.43.0` helper 容器，映射 `19001:9001`，并挂载 Docker socket。
- Agent `/ping` 返回 HTTP 204，响应头包含 `Portainer-Agent: 2.43.0` 和 `Portainer-Agent-Platform: 1`，证据见 `agent-ping.txt`。
- Agent 日志显示运行在 Docker platform，证据见 `agent-logs.txt`。
- 本地单节点 Agent 场景下，`NodeName` 默认可为空；多节点 Swarm / 远程 Agent 仍需单独实测 `X-PortainerAgent-Target` 或等价配置。
- candidate 使用随机宿主机端口，当前 Portainer 主机通过 `127.0.0.1:<candidatePort>` 健康检查返回 HTTP 200，证据见 `candidate-port.txt` 和 `candidate-healthcheck-from-portainer-host.txt`。
- 脚本完成后已删除 candidate 和 Agent helper 容器，`containers-after.txt` 显示 helper 容器列表为空。
- 本脚本没有覆盖真实 Portainer 后端签名 Docker API 调用；执行器编码前仍需在后端联调中验证 Agent 签名请求链路。
