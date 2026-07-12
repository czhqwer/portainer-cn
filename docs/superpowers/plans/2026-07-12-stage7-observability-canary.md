# 阶段 7 可观测性与最小灰度 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 为现有私有部署平台增加受限观测查询和中心 Nginx 人工 HTTP 灰度，保持凭据、流量及旧配置安全。

**Architecture:** 观测配置和灰度策略各自使用独立 BoltDB 服务。观测 handler 只调用固定模板 adapter；灰度 handler 只由成功 Release 快照生成受控 Nginx 配置，并复用候选配置发布器。

**Tech Stack:** Go、BoltDB dataservices、SecretCipher、net/http、Nginx 受控配置发布器、Go testing、Apifox CLI。

## Global Constraints

- 只改 api/、后端测试、docs/ 和后端路由；不改 app/、app/react/、translations/ 或前端测试。
- 凭据使用 SecretCipher；响应、审计、导出和错误不得泄露凭据、完整 URL、原始日志或内部拓扑。
- 不接受任意 PromQL、LogQL、Grafana Dashboard URL、Nginx 文本或 upstream 地址。
- 上游观测请求最长 30 秒，不能在 BoltDB 事务内执行。
- 灰度仅支持两个成功 Release、中心 Nginx、HTTP 服务和 0/5/25/50/100 权重。
- 阶段接口完成后直接写入 Apifox 项目 8569065 的 main 分支并回读验证。

---

### Task 1: 建立观测连接模型和加密持久化

**Files:**
- Modify: api/platform_models.go
- Modify: api/dataservices/interface.go
- Create: api/dataservices/platformobservabilityconfig/platformobservabilityconfig.go
- Modify: api/datastore/services.go
- Modify: api/datastore/services_tx.go
- Modify: api/datastore/platform_datastore_test.go
- Modify: api/internal/testhelpers/datastore.go

**Produces:** PlatformObservabilityConfig、PlatformObservabilityConfigID、PlatformObservabilityConfigService、NewPlatformObservabilityConfig、ValidatePlatformObservabilityConfig。

- [ ] **Step 1: 写失败测试**

~~~go
func TestPlatformObservabilityConfigExportStripsCredentials(t *testing.T) {
    config := portainer.NewPlatformObservabilityConfig()
    config.PrometheusURL = "https://prometheus.example.test"
    config.BearerTokenCipherText = "cipher"
    config.CredentialEncryptionVersion = portainer.PlatformObservabilityCredentialEncryptionVersion
    config.CredentialHash = "hash"
    config.HasCredentials = true
    require.NoError(t, store.PlatformObservabilityConfig().Create(&config))
    require.NoError(t, store.Export(backupFile))
    require.NoError(t, importedStore.Import(backupFile))
    imported, err := importedStore.PlatformObservabilityConfig().Read(config.ID)
    require.NoError(t, err)
    require.False(t, imported.HasCredentials)
    require.Empty(t, imported.BearerTokenCipherText)
}
~~~

- [ ] **Step 2: 验证失败**

Run: E:\develop\Go\bin\go.exe test ./api/datastore -run TestPlatformObservabilityConfigExportStripsCredentials -count=1  
Expected: FAIL，模型尚不存在。

- [ ] **Step 3: 实现最小模型和服务**

~~~go
type PlatformObservabilityConfig struct {
    ID                          PlatformObservabilityConfigID
    PrometheusURL               string
    LokiURL                     string
    GrafanaURL                  string
    BearerTokenCipherText       string
    CredentialEncryptionVersion string
    CredentialHash              string
    HasCredentials              bool
    Enabled                     bool
    Revision                    int
    PlatformLifecycle
}

type PlatformObservabilityConfigService interface {
    BaseCRUD[portainer.PlatformObservabilityConfig, portainer.PlatformObservabilityConfigID]
    GetNextIdentifier() int
}
~~~

URL 仅允许 HTTPS；凭据状态必须成组存在；Export 清空四个凭据字段；Update 只在配置内容改变时递增 Revision。

- [ ] **Step 4: 验证通过并提交**

Run: E:\develop\Go\bin\go.exe test ./api/datastore -run 'TestPlatform(DataServices|ObservabilityConfig)' -count=1  
Expected: PASS。

~~~text
git add api/platform_models.go api/dataservices api/datastore api/internal/testhelpers
git commit -m "feat: 新增平台观测连接模型"
~~~

### Task 2: 实现固定模板观测 adapter

**Files:**
- Create: api/platform/observability.go
- Create: api/platform/observability_test.go

**Produces:** ObservabilityAdapter、ObservabilityQuery、ObservabilityResult、稳定 reason 常量。

- [ ] **Step 1: 写失败测试**

~~~go
func TestObservabilityAdapterUsesFixedTemplateAndCapsSeries(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Contains(t, r.URL.Query().Get("query"), "platform_service_deployment_id")
        require.NotContains(t, r.URL.Query().Get("query"), "user supplied query")
        _, _ = w.Write([]byte("{\"status\":\"success\",\"data\":{\"result\":[{}, {}, {}]}}"))
    }))
    adapter := NewHTTPObservabilityAdapter(server.Client())
    result, err := adapter.Query(context.Background(), ObservabilityQuery{
        Template: ObservabilityTemplateServiceAvailability,
        PrometheusURL: server.URL, ServiceDeploymentID: 1, Start: 100, End: 160,
    })
    require.NoError(t, err)
    require.LessOrEqual(t, len(result.Series), MaxObservabilitySeries)
}
~~~

- [ ] **Step 2: 验证失败**

Run: E:\develop\Go\bin\go.exe test ./api/platform -run TestObservabilityAdapterUsesFixedTemplateAndCapsSeries -count=1  
Expected: FAIL，adapter 尚不存在。

- [ ] **Step 3: 实现 adapter**

~~~go
type ObservabilityAdapter interface {
    Query(context.Context, ObservabilityQuery) (ObservabilityResult, error)
}

const (
    ObservabilityTemplateServiceAvailability = "service-availability"
    ObservabilityTemplateServiceLatency = "service-latency"
    ObservabilityTemplateServiceLogs = "service-logs"
    MaxObservabilitySeries = 100
    MaxObservabilityLogLines = 200
)
~~~

Prometheus availability/latency 和 Loki logs 分别由模板生成请求；拒绝未知模板、超过 24 小时范围或非 HTTPS URL；使用 30 秒 context；将超时、网络失败、非成功响应和响应超限转换为稳定 reason。

- [ ] **Step 4: 验证通过并提交**

Run: E:\develop\Go\bin\go.exe test ./api/platform -run TestObservability -count=1  
Expected: PASS。

~~~text
git add api/platform/observability.go api/platform/observability_test.go
git commit -m "feat: 增加受限观测查询适配器"
~~~

### Task 3: 接入观测配置和查询 API

**Files:**
- Modify: api/http/handler/platform/handler.go
- Create: api/http/handler/platform/observability.go
- Create: api/http/handler/platform/observability_test.go
- Modify: api/http/handler/platform/config_secrets.go

**Consumes:** Handler.ObservabilityAdapter、PlatformObservabilityConfigService。  
**Produces:** 观测配置 CRUD/测试和项目观测摘要路由。

- [ ] **Step 1: 写失败 handler 测试**

~~~go
func TestPlatformObservabilityQueryRequiresProjectAndEndpointPermission(t *testing.T) {
    ctx := newPlatformTestContext(t)
    ctx.handler.ObservabilityAdapter = fakeObservabilityAdapter{}
    doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet,
        "/platform/projects/1/observability?environmentId=1&serviceDeploymentId=1&template=service-availability",
        nil, http.StatusForbidden)
}
~~~

- [ ] **Step 2: 验证失败**

Run: E:\develop\Go\bin\go.exe test ./api/http/handler/platform -run TestPlatformObservabilityQueryRequiresProjectAndEndpointPermission -count=1  
Expected: FAIL，路由尚不存在。

- [ ] **Step 3: 实现授权、超时和审计**

~~~go
func (handler *Handler) observabilityQuery(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
    project, handlerErr := handler.requireProjectPermission(r, projectID, platformPermissionView)
    if handlerErr != nil { return handlerErr }
    environment, handlerErr := handler.requireEnvironmentPermission(r, environmentID, platformPermissionView)
    if handlerErr != nil || environment.ProjectID != project.ID { return platformAccessDenied() }
    if handlerErr := handler.requireEnvironmentEndpointAccess(r, environment); handlerErr != nil { return handlerErr }
    result, reason := handler.queryObservabilityOutsideTransaction(r.Context(), payload)
    return handler.persistObservabilityAuditAndRespond(w, r, result, reason)
}
~~~

配置 CRUD/测试必须 requireGlobalAdmin；请求内凭据立即加密；响应清空密文/哈希；adapter 完成后写只含模板、对象 ID、计数、状态和稳定 reason 的审计。

- [ ] **Step 4: 验证通过并提交**

Run: E:\develop\Go\bin\go.exe test ./api/http/handler/platform -run TestPlatformObservability -count=1  
Expected: PASS。

~~~text
git add api/http/handler/platform/handler.go api/http/handler/platform/observability.go api/http/handler/platform/observability_test.go api/http/handler/platform/config_secrets.go
git commit -m "feat: 提供平台受限观测接口"
~~~

### Task 4: 建立灰度策略模型与 Nginx 渲染

**Files:**
- Modify: api/platform_models.go
- Modify: api/dataservices/interface.go
- Create: api/dataservices/platformcanarypolicy/platformcanarypolicy.go
- Modify: api/datastore/services.go
- Modify: api/datastore/services_tx.go
- Modify: api/platform/gateway_renderer.go
- Modify: api/platform/gateway_renderer_test.go
- Modify: api/datastore/platform_datastore_test.go

**Produces:** PlatformCanaryPolicy、PlatformCanaryPolicyService、GatewayRouteTarget.CanaryUpstreams、GatewayRouteTarget.CanaryWeight、ValidatePlatformCanaryWeight。

- [ ] **Step 1: 写失败 renderer 测试**

~~~go
func TestRenderGatewayConfigRendersDeterministicCanarySplit(t *testing.T) {
    config, _, err := RenderGatewayConfig([]GatewayRouteTarget{{
        Route: route,
        Upstreams: []GatewayUpstreamTarget{{Host: "10.0.0.1", Port: 18080}},
        CanaryUpstreams: []GatewayUpstreamTarget{{Host: "10.0.0.2", Port: 18081}},
        CanaryWeight: 25,
    }})
    require.NoError(t, err)
    require.Contains(t, string(config), "split_clients")
    require.Contains(t, string(config), "25%")
}
~~~

- [ ] **Step 2: 验证失败**

Run: E:\develop\Go\bin\go.exe test ./api/platform -run TestRenderGatewayConfigRendersDeterministicCanarySplit -count=1  
Expected: FAIL，灰度字段尚不存在。

- [ ] **Step 3: 实现模型和 renderer**

~~~go
type PlatformCanaryPolicy struct {
    ID PlatformCanaryPolicyID
    ProjectID PlatformProjectID
    EnvironmentID PlatformEnvironmentID
    GatewayRouteID PlatformGatewayRouteID
    StableReleaseID PlatformReleaseID
    CanaryReleaseID PlatformReleaseID
    CurrentWeight int
    ResourceVersion int
    LastFailureReason string
    PlatformLifecycle
}

func ValidatePlatformCanaryWeight(weight int) error {
    switch weight { case 0, 5, 25, 50, 100: return nil }
    return fmt.Errorf("canary weight is invalid")
}
~~~

renderer 用 $request_id 的 split_clients 分流，稳定 upstream 为默认分支；canary 权重零时不生成分流；所有 upstream 继续使用已有 host/port 注入校验。

- [ ] **Step 4: 验证通过并提交**

Run: E:\develop\Go\bin\go.exe test ./api/platform ./api/datastore -run 'Test(RenderGatewayConfigRendersDeterministicCanarySplit|PlatformCanary)' -count=1  
Expected: PASS。

~~~text
git add api/platform_models.go api/dataservices api/datastore api/platform/gateway_renderer.go api/platform/gateway_renderer_test.go
git commit -m "feat: 新增平台最小灰度策略模型"
~~~

### Task 5: 实现灰度 API、健康 Gate 与原子回退

**Files:**
- Modify: api/http/handler/platform/handler.go
- Create: api/http/handler/platform/canary_policies.go
- Create: api/http/handler/platform/canary_policies_test.go
- Modify: api/http/handler/platform/gateways.go

**Produces:** 灰度策略创建、查询、权重变更及回退 API。

- [ ] **Step 1: 写失败状态机测试**

~~~go
func TestPlatformCanaryWeightChangeRestoresPreviousWeightOnReloadFailure(t *testing.T) {
    ctx := newPlatformTestContext(t)
    policy := createCanaryPolicyFixture(t, ctx, 0)
    ctx.handler.GatewayRuntime = failingReloadGatewayRuntime{}
    doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost,
        fmt.Sprintf("/platform/canary-policies/%d/weights", policy.ID),
        canaryWeightPayload{ResourceVersion: policy.ResourceVersion, Weight: 5},
        http.StatusBadGateway)
    stored, err := ctx.handler.DataStore.PlatformCanaryPolicy().Read(policy.ID)
    require.NoError(t, err)
    require.Equal(t, 0, stored.CurrentWeight)
}
~~~

- [ ] **Step 2: 验证失败**

Run: E:\develop\Go\bin\go.exe test ./api/http/handler/platform -run TestPlatformCanaryWeightChangeRestoresPreviousWeightOnReloadFailure -count=1  
Expected: FAIL，策略 handler 尚不存在。

- [ ] **Step 3: 实现状态机**

~~~go
func allowedCanaryTransition(current, next int) bool {
    allowed := map[int]map[int]bool{
        0: {5:true}, 5: {0:true,25:true}, 25: {0:true,5:true,50:true},
        50: {0:true,25:true,100:true}, 100: {0:true,50:true},
    }
    return allowed[current][next]
}
~~~

创建和放量前验证两个 Release 同属路由服务部署、均成功、canary 运行时健康且端口可解析。调用 GatewayConfigPublisher 失败时不得更新 CurrentWeight；候选健康失败时发布 0% 配置并写 CANARY_HEALTH_FAILED 审计；成功时更新策略版本和审计。

- [ ] **Step 4: 验证通过并提交**

Run: E:\develop\Go\bin\go.exe test ./api/http/handler/platform ./api/platform -run TestPlatformCanary -count=1  
Expected: PASS。

~~~text
git add api/http/handler/platform/handler.go api/http/handler/platform/canary_policies.go api/http/handler/platform/canary_policies_test.go api/http/handler/platform/gateways.go
git commit -m "feat: 支持平台人工灰度放量"
~~~

### Task 6: 阶段验收和 Apifox 文档

**Files:**
- Modify: docs/阶段7实施进度.md
- Create: docs/阶段7验收记录.md

- [ ] **Step 1: 运行回归**

Run: E:\develop\Go\bin\go.exe test ./api/platform ./api/http/handler/platform -count=1  
Expected: PASS。

Run: E:\develop\Go\bin\go.exe test ./api/datastore -run '^TestPlatform(DataServices|Canary|Observability)' -count=1  
Expected: PASS。

Run: E:\develop\Go\bin\go.exe test ./api/http -run '^$' -count=1  
Expected: PASS。

- [ ] **Step 2: 创建并验证 Apifox 文档**

读取原工作区 .apifox/settings.json，使用 project 8569065 与 branch main。对每个 endpoint JSON 先运行 apifox cli-schema get endpoint-create 和 apifox cli-schema validate endpoint-create，再创建观测配置、查询、灰度策略、放量、回退接口并在同一分支 apifox endpoint get 回读。

- [ ] **Step 3: 写验收记录并提交**

记录测试命令、失败 reason、外部 Prometheus/Loki/Grafana/Nginx 演练待办、Apifox ID、无前端差异和不合并 develop 的结论。

~~~text
git add docs/阶段7实施进度.md docs/阶段7验收记录.md
git commit -m "test: 完成阶段7观测灰度后端验收"
~~~

## Self-Review

- 覆盖：Task 1-3 实现观测模型、模板、超时、权限与审计；Task 4-5 实现双 Release、固定权重、健康 Gate、原子恢复与审计；Task 6 实现文档与验收。
- 范围：不引入 ACME、Kubernetes、任意查询或自动放量。
- 一致性：handler 消费 Task 1 的服务与 Task 2 adapter；Task 5 消费 Task 4 灰度模型和 renderer。
