# S5 版本化命名 Spike 证据说明

执行时间：2026-07-11 18:30

执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-versioned-naming-spike.ps1 -Apply
```

说明：

- 旧正式容器命名：`pcn-demo-shop-prod-cn-web-api-r2026071101`。
- 新 candidate 命名：`pcn-demo-shop-prod-cn-web-api-r2026071102-candidate`。
- 新正式容器命名：`pcn-demo-shop-prod-cn-web-api-r2026071102`。
- 旧正式容器在 `18081:80` 上健康检查返回 HTTP 200。
- 新 candidate 使用随机宿主机端口，健康检查返回 HTTP 200，验证后被删除。
- 停止旧正式容器但保留其容器对象后，新正式容器使用新 releaseId 名称成功创建并复用 `18081:80`，健康检查返回 HTTP 200。
- `containers-during-versioned-retention.txt` 证明旧 release 容器保留为 stopped，新 release 容器处于 running，二者名称不冲突。
- 脚本完成后已删除旧正式容器和新正式容器，`containers-after.txt` 显示 helper 容器列表为空。
