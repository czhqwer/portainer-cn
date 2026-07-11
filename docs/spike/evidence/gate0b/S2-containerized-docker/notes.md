# S2 容器化后端 + Docker socket Spike 证据说明

执行时间：2026-07-11 18:10

执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-containerized-docker-spike.ps1 -Apply
```

说明：

- 本次执行未传入 `-RecordAllContainers`，`containers-before.txt` 和 `containers-after.txt` 只记录带 `com.portainer-cn.platform.spike=gate0b` label 的 helper 容器。
- Docker CLI 临时容器挂载 `/var/run/docker.sock` 后可以执行 `docker version`，证据见 `container-docker-socket.txt`。
- candidate 使用随机宿主机端口，实测端口记录在 `candidate-port.txt`，inspect 证据见 `candidate-inspect.json`。
- probe 容器内访问 `http://127.0.0.1:<candidatePort>/` 失败，符合“容器内 loopback 不等于宿主机”的预期，证据见 `probe-loopback.txt`。
- probe 容器内访问 Docker bridge gateway 和 `host.docker.internal` 均返回 HTTP 200，证据见 `probe-bridge-gateway.txt`、`probe-host-docker-internal.txt` 和 `containerized-health-summary.txt`。
- 本地 Docker Desktop 环境下，`HealthCheckHost` 可使用 bridge gateway `172.17.0.1` 或 `host.docker.internal` 访问 candidate 随机宿主机端口；正式实现仍应允许用户显式配置 `HealthCheckHost`，不能把该地址跨环境硬编码。
- 脚本完成后已删除 candidate，`containers-after.txt` 显示 helper 容器列表为空。
