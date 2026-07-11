# S7 私有 Registry Spike 证据说明

执行时间：2026-07-11 18:17

执行命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File docs/spike/gate0b-private-registry-spike.ps1 -Apply
```

说明：

- Registry 地址：`127.0.0.1:5000`。
- 凭据通过父 shell 环境变量传入，脚本只记录 `<redacted>` 命令；证据中未提交密码、认证头或 Docker config。
- 脚本使用临时 Docker config 登录 registry，执行完成后已删除临时认证目录。
- 正确凭据登录成功，证据见 `login-success.txt`。
- 错误凭据登录返回 401，可映射为 `REGISTRY_AUTH_FAILED`，证据见 `login-wrong-password.txt`。
- `nginx:alpine` 被 tag 为 `127.0.0.1:5000/portainer-cn/gate0b-spike:v0.1` 后 push 成功，证据见 `private-push.txt`。
- 私有镜像 pull 成功，`RepoDigests` 中解析到私有 registry digest，证据见 `private-pull.txt`、`private-image-repodigests.txt` 和 `registry-summary.txt`。
- 缺失镜像 pull 返回 `manifest unknown`，可映射为 `IMAGE_PULL_FAILED`，证据见 `missing-image-pull.txt`。
- `docker manifest inspect` 对本地 HTTP registry 返回 `no such manifest`，本次 digest 解析以 pull 后的 `RepoDigests` 为准。
- 脚本写完摘要后已移除本地私有镜像 tag，证据见 `private-local-rm-after-summary.txt`。
