# S1 本地 Docker socket Spike 证据说明

执行时间：2026-07-11 17:52

执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-local-docker-spike.ps1 -Apply
```

说明：

- 本次执行未传入 `-RecordAllContainers`，`containers-before.txt` 和 `containers-after.txt` 只记录带 `com.portainer-cn.platform.spike=gate0b` label 的 helper 容器。
- `containers-before.txt` 中的 `pcn-spike-gate0b-candidate` 来自前一次脚本修复前的失败尝试；成功执行开始后已先执行 `docker rm -f pcn-spike-gate0b-candidate`，再重新创建 candidate。
- candidate 随机宿主机端口健康检查返回 HTTP 200，证据见 `candidate-healthcheck.txt`。
- 正式端口 `18080` 上的 `r1` 和 `r2` 健康检查均返回 HTTP 200，证据见 `official-r1-healthcheck.txt` 和 `official-r2-healthcheck.txt`。
- 坏镜像启动失败后，旧容器 `r1` 被重新启动并通过 HTTP 200 健康检查，证据见 `official-r1-recovered-healthcheck.txt`。
- 后续已执行清理命令，清理证据位于 `docs/spike/evidence/gate0b/S1-local-docker-cleanup/`，其中 `containers-after-cleanup.txt` 显示 helper 容器列表为空。
