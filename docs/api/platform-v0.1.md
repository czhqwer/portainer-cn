# 平台 API（V0.1 技术预览）

版本：基于当前仓库实现（批次 6 后）  
日期：2026-07-11  
权限：全部接口 `AdminAccess`，非管理员返回 403

> 本文档以 `api/http/handler/platform` 实际代码为准，不是阶段 0 设计稿全文。  
> 设计稿中尚未实现的接口（如 `cancel` / `retry-recovery` / `cleanup-runtime` / `cleanup-original`）不列入。

## 1. 结论：可以通过接口访问

Gate 0B 横幅只挡前端交互，不挡后端。

| 能力 | UI | API |
| --- | --- | --- |
| 项目 / 环境 / 应用 / 服务 CRUD | 列表骨架可读，创建表单不完整 | 可用 |
| 制品 image-reference 登记 | 页面可进，创建受限 | 可用 |
| 发布校验 / 真实发布 | 可能显示 Gate 0B 禁用 | 可用（执行器已接入） |
| 运行状态 / 日志 | 批次 7 完善中 | `status` / `logs` 已可用 |
| 人工 resolve | UI 未完整 | `POST .../resolve` 可用 |

## 2. 认证

先登录拿 JWT，再带 `Authorization: Bearer <jwt>`。

```bash
# 登录
curl -s http://localhost:9000/api/auth \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"你的密码\"}"
```

响应：

```json
{ "jwt": "eyJhbGciOi..." }
```

后续请求：

```bash
export TOKEN="eyJhbGciOi..."
export API=http://localhost:9000/api

curl -s "$API/platform/projects" \
  -H "Authorization: Bearer $TOKEN"
```

也支持：

- Cookie 会话（浏览器登录后自动带）
- `X-API-KEY`（管理员 Access Token）

不要同时带 API Key 和 Bearer Token。

## 3. 公共约定

### 3.1 Base Path

所有平台业务接口前缀：`/api/platform`

### 3.2 乐观锁

更新类 `PUT` 必须带当前 `ResourceVersion`。不匹配返回：

- HTTP `409`
- code：`PLATFORM_RESOURCE_VERSION_CONFLICT`

### 3.3 归档

`DELETE` 是归档，不是物理删除。列表默认隐藏已归档对象；加 `?includeArchived=true` 可看到。

### 3.4 Name / Slug

创建时：

- `Name`、`Slug` 必填，最长 64
- `Slug` 仅 `a-z`、`0-9`、`-`，且不能首尾是 `-`
- 同作用域 active slug 重复返回冲突

### 3.5 错误码

| code | 常见场景 |
| --- | --- |
| `PLATFORM_INVALID_REQUEST` | 路由参数非法 |
| `PLATFORM_VALIDATION_FAILED` | payload 校验失败、业务重复 |
| `PLATFORM_NOT_FOUND` | 资源不存在 |
| `PLATFORM_RESOURCE_VERSION_CONFLICT` | 乐观锁冲突 |
| `PLATFORM_RELEASE_CONFLICT` | 同一 ServiceDeployment 已有进行中发布 |
| `PLATFORM_IDEMPOTENCY_PAYLOAD_MISMATCH` | 同一幂等键对应不同 payload |
| `PLATFORM_UNSUPPORTED_OPERATION` | 执行器未配置等 |
| `PLATFORM_INTERNAL_ERROR` | 内部错误 |

部分发布冲突会额外返回结构化 body：

```json
{
  "code": "PLATFORM_RELEASE_CONFLICT",
  "message": "ServiceDeployment already has an active release.",
  "details": { "reason": "RELEASE_LOCKED" },
  "data": {
    "ReleaseId": 12,
    "Status": "switching",
    "PollUrl": "/api/platform/releases/12"
  }
}
```

## 4. 完整发布闭环（推荐调用顺序）

把下面命令里的 `EndpointId` 换成你环境里的 Docker Endpoint ID。

```bash
export API=http://localhost:9000/api
export TOKEN="<jwt>"
export AUTH="Authorization: Bearer $TOKEN"

# 0) 查 Docker EndpointId
curl -s "$API/endpoints" -H "$AUTH"
# 记下本地 Docker 的 Id，例如 3
export ENDPOINT_ID=3

# 1) 创建项目
curl -s -X POST "$API/platform/projects" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"Name\":\"演示项目\",\"Slug\":\"demo\",\"Description\":\"V0.1 API 验证\"}"
# => Id=1

# 2) 创建环境（绑定 Endpoint）
curl -s -X POST "$API/platform/projects/1/environments" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"Name\":\"本地开发\",
    \"Slug\":\"dev\",
    \"Type\":\"dev\",
    \"IsProduction\":false,
    \"TargetMode\":\"single\",
    \"HealthCheckHost\":\"127.0.0.1\",
    \"Targets\":[
      {
        \"EndpointId\": $ENDPOINT_ID,
        \"Role\":\"workload\",
        \"HostAddress\":\"127.0.0.1\",
        \"Enabled\":true
      }
    ]
  }"
# => Id=1

# 3) 创建应用
curl -s -X POST "$API/platform/projects/1/applications" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"Name\":\"订单应用\",\"Slug\":\"order\",\"Description\":\"demo app\"}"
# => Id=1

# 4) 创建逻辑服务
curl -s -X POST "$API/platform/applications/1/services" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"Name\":\"订单 API\",\"Slug\":\"order-api\",\"Type\":\"backend\",\"Description\":\"http service\"}"
# => Id=1

# 5) 创建环境部署配置 DesiredSpec
curl -s -X POST "$API/platform/services/1/deployments" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"EnvironmentId\": 1,
    \"DesiredSpec\": {
      \"Image\": {
        \"Image\": \"nginx:alpine\",
        \"PullPolicy\": \"if-not-present\"
      },
      \"Ports\": [
        {
          \"Name\": \"http\",
          \"ContainerPort\": 80,
          \"HostPort\": 18080,
          \"Protocol\": \"tcp\",
          \"ExposeMode\": \"published\"
        }
      ],
      \"HealthCheck\": {
        \"VerificationLevel\": \"verified\",
        \"Type\": \"http\",
        \"Path\": \"/\",
        \"Port\": 80,
        \"IntervalSeconds\": 5,
        \"TimeoutSeconds\": 3,
        \"Retries\": 3,
        \"StartPeriodSeconds\": 10
      },
      \"Runtime\": {
        \"RuntimeDriver\": \"docker-container\",
        \"Replicas\": 1,
        \"RestartPolicy\": \"unless-stopped\"
      },
      \"Strategy\": { \"Type\": \"replace\" }
    }
  }"
# => Id=1, SpecRevision=1

# 6) 登记已有镜像制品
curl -s -X POST "$API/platform/artifacts/image-reference" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"ProjectId\": 1,
    \"ApplicationId\": 1,
    \"ServiceDefinitionId\": 1,
    \"Name\": \"order-api\",
    \"Version\": \"20260711-001\",
    \"ImageRef\": \"nginx:alpine\"
  }"
# => Id=1

# 7) 发布前校验
curl -s -X POST "$API/platform/releases/validate" -H "$AUTH" -H "Content-Type: application/json" \
  -d "{
    \"ProjectId\": 1,
    \"EnvironmentId\": 1,
    \"ApplicationId\": 1,
    \"ServiceDefinitionId\": 1,
    \"ServiceDeploymentId\": 1,
    \"ArtifactId\": 1,
    \"Version\": \"20260711-001\",
    \"ExpectedSpecRevision\": 1,
    \"Strategy\": { \"Type\": \"replace\" }
  }"

# 8) 创建发布（必须带 Idempotency-Key）
curl -s -X POST "$API/platform/releases" -H "$AUTH" -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-release-20260711-001" \
  -d "{
    \"ProjectId\": 1,
    \"EnvironmentId\": 1,
    \"ApplicationId\": 1,
    \"ServiceDefinitionId\": 1,
    \"ServiceDeploymentId\": 1,
    \"ArtifactId\": 1,
    \"Version\": \"20260711-001\",
    \"ExpectedSpecRevision\": 1,
    \"Strategy\": { \"Type\": \"replace\" },
    \"TriggerType\": \"deploy\"
  }"
# => 202
# {
#   "ReleaseId": 1,
#   "Status": "succeeded",
#   "PollUrl": "/api/platform/releases/1"
# }

# 9) 轮询发布详情 / 查看运行状态与日志
curl -s "$API/platform/releases/1" -H "$AUTH"
curl -s "$API/platform/service-deployments/1/status" -H "$AUTH"
curl -s "$API/platform/service-deployments/1/logs?tail=200" -H "$AUTH"
```

说明：

- `ExpectedSpecRevision` 必须等于当前 `ServiceDeployment.SpecRevision`
- 同一 `Idempotency-Key` + 相同 payload 会复用同一 Release
- 同一 `ServiceDeployment` 同时只能有一个进行中发布
- V0.1 策略只支持 `replace`，固定端口切换可能短暂停机；失败会尝试恢复旧容器

## 5. 接口清单

### 5.1 Project

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/projects` | 项目列表，`?includeArchived=true` |
| `POST` | `/api/platform/projects` | 创建项目 |
| `GET` | `/api/platform/projects/{projectId}` | 详情 |
| `PUT` | `/api/platform/projects/{projectId}` | 更新，需 `ResourceVersion` |
| `DELETE` | `/api/platform/projects/{projectId}` | 归档 |

创建 body：

```json
{
  "Name": "演示项目",
  "Slug": "demo",
  "Description": "optional"
}
```

更新 body：

```json
{
  "ResourceVersion": 1,
  "Name": "演示项目-改名",
  "Description": "updated"
}
```

### 5.2 Environment

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/projects/{projectId}/environments` | 环境列表 |
| `POST` | `/api/platform/projects/{projectId}/environments` | 创建环境 |
| `GET` | `/api/platform/environments/{environmentId}` | 详情 |
| `PUT` | `/api/platform/environments/{environmentId}` | 更新 |
| `DELETE` | `/api/platform/environments/{environmentId}` | 归档 |

创建 body：

```json
{
  "Name": "本地开发",
  "Slug": "dev",
  "Type": "dev",
  "IsProduction": false,
  "TargetMode": "single",
  "HealthCheckHost": "127.0.0.1",
  "DefaultRegistryId": 0,
  "Targets": [
    {
      "EndpointId": 3,
      "Role": "workload",
      "HostAddress": "127.0.0.1",
      "Enabled": true
    }
  ],
  "ReleasePolicy": { "Type": "replace" }
}
```

字段说明：

- `Type`：`dev` / `test` / `prod` / `custom`
- `TargetMode`：V0.1 用 `single`
- `Targets[].EndpointId`：Portainer Docker 环境 ID（`GET /api/endpoints`）
- `HealthCheckHost`：candidate / 正式容器健康检查从 Portainer 主机访问的地址，本地常见 `127.0.0.1`

### 5.3 Application

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/projects/{projectId}/applications` | 应用列表 |
| `POST` | `/api/platform/projects/{projectId}/applications` | 创建应用 |
| `GET` | `/api/platform/applications/{applicationId}` | 详情 |
| `PUT` | `/api/platform/applications/{applicationId}` | 更新 |
| `DELETE` | `/api/platform/applications/{applicationId}` | 归档 |

创建 body：

```json
{
  "Name": "订单应用",
  "Slug": "order",
  "Description": "optional",
  "OwnerUserIds": [1]
}
```

### 5.4 ServiceDefinition

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/applications/{applicationId}/services` | 逻辑服务列表 |
| `POST` | `/api/platform/applications/{applicationId}/services` | 创建逻辑服务 |
| `GET` | `/api/platform/services/{serviceDefinitionId}` | 详情 |
| `PUT` | `/api/platform/services/{serviceDefinitionId}` | 更新 |
| `DELETE` | `/api/platform/services/{serviceDefinitionId}` | 归档 |

创建 body：

```json
{
  "Name": "订单 API",
  "Slug": "order-api",
  "Type": "backend",
  "Description": "optional"
}
```

`Type` 常用值：`frontend` / `backend` / `java-service` / `python-service` / `worker` / `scheduler` / `static-site` / `database` / `redis`

### 5.5 ServiceDeployment

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/services/{serviceDefinitionId}/deployments` | 某逻辑服务下的环境部署列表 |
| `POST` | `/api/platform/services/{serviceDefinitionId}/deployments` | 为某环境创建部署配置 |
| `GET` | `/api/platform/service-deployments/{deploymentId}` | 详情 |
| `PUT` | `/api/platform/service-deployments/{deploymentId}` | 更新 DesiredSpec，`SpecRevision++` |
| `DELETE` | `/api/platform/service-deployments/{deploymentId}` | 归档 |
| `GET` | `/api/platform/service-deployments/{deploymentId}/status` | 实时运行态 |
| `GET` | `/api/platform/service-deployments/{deploymentId}/logs` | 容器日志，`?tail=100`（默认 100，最大 1000） |

创建 body：

```json
{
  "EnvironmentId": 1,
  "DesiredSpec": {
    "Image": {
      "Image": "nginx:alpine",
      "PullPolicy": "if-not-present"
    },
    "Ports": [
      {
        "Name": "http",
        "ContainerPort": 80,
        "HostPort": 18080,
        "Protocol": "tcp",
        "ExposeMode": "published"
      }
    ],
    "EnvOverrides": [
      { "Name": "APP_ENV", "Value": "dev", "Source": "literal", "IsSecret": false }
    ],
    "HealthCheck": {
      "VerificationLevel": "verified",
      "Type": "http",
      "Path": "/",
      "Port": 80
    },
    "Runtime": {
      "RuntimeDriver": "docker-container",
      "Replicas": 1,
      "RestartPolicy": "unless-stopped"
    },
    "Strategy": { "Type": "replace" }
  }
}
```

更新 body：

```json
{
  "ResourceVersion": 1,
  "DesiredSpec": { "...": "完整 DesiredSpec" }
}
```

`status` 响应关键字段：

- `RuntimeFound` / `RuntimeRunning`
- `RuntimeState` / `PublishedPorts`
- `Reason`：如 `RUNTIME_NOT_CONFIGURED`、`RUNTIME_MISSING`、`RUNTIME_INSPECTOR_UNAVAILABLE`
- 外部删容器后会标 `DriftStatus=runtime-missing`，不会把 Service 配置标成删除

### 5.6 Artifact

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/artifacts` | 列表，支持筛选 |
| `POST` | `/api/platform/artifacts/image-reference` | 登记已有镜像 |
| `GET` | `/api/platform/artifacts/{artifactId}` | 详情 |
| `POST` | `/api/platform/artifacts/{artifactId}/validate` | 元数据级校验 |
| `DELETE` | `/api/platform/artifacts/{artifactId}` | 归档元数据 |

列表查询参数：

- `projectId`
- `applicationId`
- `serviceDefinitionId`
- `includeArchived=true`

创建 body：

```json
{
  "ProjectId": 1,
  "ApplicationId": 1,
  "ServiceDefinitionId": 1,
  "Name": "order-api",
  "Version": "20260711-001",
  "ImageRef": "nginx:alpine",
  "ImageDigest": "",
  "RegistryId": 0,
  "Traceability": "weak"
}
```

说明：

- V0.1 只支持 `image-reference` 登记，不在创建时 pull / 解析 digest
- 私有镜像发布时，凭据复用 Portainer Registry（环境 `DefaultRegistryId` 或制品 `RegistryId`）

### 5.7 Release

| 方法 | 路径 | 返回 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/platform/releases` | 200 | 发布列表 |
| `POST` | `/api/platform/releases/validate` | 200 | 发布前校验，不落库执行 |
| `POST` | `/api/platform/releases` | 202 | 创建发布并触发执行器 |
| `GET` | `/api/platform/releases/{releaseId}` | 200 | 发布详情与步骤 |
| `POST` | `/api/platform/releases/{releaseId}/resolve` | 200 | 人工解决 `interrupted` / `recovery-failed` |

列表查询参数：

- `projectId`
- `environmentId`
- `applicationId`
- `serviceDefinitionId`
- `serviceDeploymentId`
- `status`

创建发布 **必须** 带请求头：

```http
Idempotency-Key: <任意稳定字符串>
```

创建 / validate body：

```json
{
  "ProjectId": 1,
  "EnvironmentId": 1,
  "ApplicationId": 1,
  "ServiceDefinitionId": 1,
  "ServiceDeploymentId": 1,
  "ArtifactId": 1,
  "Version": "20260711-001",
  "ExpectedSpecRevision": 1,
  "Strategy": { "Type": "replace" },
  "TriggerType": "deploy"
}
```

创建成功响应（202）：

```json
{
  "ReleaseId": 1,
  "Status": "succeeded",
  "PollUrl": "/api/platform/releases/1"
}
```

当前实现会在请求内同步推进执行器，因此返回时 `Status` 可能已是终态（如 `succeeded` / `failed` / `recovery-failed`），不一定停在 `queued`。仍建议用 `PollUrl` 再查详情确认步骤。

validate 响应示例：

```json
{
  "Valid": true,
  "Executable": true,
  "Message": "Gate 0B has passed; release execution can start.",
  "ServiceDeploymentId": 1,
  "ArtifactId": 1,
  "ExpectedSpecRevision": 1
}
```

resolve body（仅 `interrupted` / `recovery-failed`）：

```json
{
  "Action": "accept-current",
  "Comment": "已人工确认当前容器可用"
}
```

`Action`：

- `accept-current`：接纳当前运行版本并更新 ServiceDeployment 当前成功发布
- `mark-handled`：标记已处理
- `release-lock-only`：只释放发布锁

### 5.8 Audit Logs

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/platform/audit-logs` | 最小平台审计列表 |

查询参数：

- `projectId`
- `releaseId`
- `serviceDeploymentId`
- `action`

## 6. 常见失败排查

| 现象 | 处理 |
| --- | --- |
| UI 显示 Gate 0B | 忽略横幅，改打 API；或等批次 7 清前端开关 |
| `403` | 确认管理员 JWT / API Key |
| 环境创建后发布找不到目标 | 检查 `Targets[].EndpointId` 是否存在且可达 |
| `ExpectedSpecRevision` 校验失败 | 先 `GET` deployment，用最新 `SpecRevision` |
| `PLATFORM_RELEASE_CONFLICT` | 等当前发布结束，或对 `recovery-failed`/`interrupted` 先 resolve |
| 健康检查失败 | 检查 `HealthCheckHost`、端口映射、容器内 path/port |
| 私有镜像拉取失败 | 在 Portainer Registries 配好凭据，并写到环境/制品 RegistryId |

## 7. 与设计稿差异（当前实现）

以下设计稿接口**尚未实现**，请勿依赖：

- `POST /api/platform/releases/{id}/cancel`
- `POST /api/platform/releases/{id}/retry-recovery`
- `POST /api/platform/releases/{id}/cleanup-runtime`
- `POST /api/platform/releases/{id}/rollback`（阶段 2）
- `POST /api/platform/artifacts/{id}/cleanup-original`（阶段 3）

设计稿里 `POST /releases` 返回包了一层 `data`；当前实现直接返回：

```json
{ "ReleaseId": 1, "Status": "...", "PollUrl": "..." }
```

## 8. 相关代码

- 路由注册：`api/http/handler/platform/handler.go`
- 请求体：`api/http/handler/platform/payloads.go`、`artifact_release_payloads.go`
- 模型：`api/platform_models.go`
- 发布执行：`api/platform/`
