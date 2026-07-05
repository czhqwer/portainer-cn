# Portainer.CN

Portainer.CN 是基于 Portainer Community Edition 的中文二开版本，目标是保留 Docker、Swarm、Kubernetes 的核心管理能力，同时提供更适合中文日常使用的界面体验。

当前版本包含：

- 中文/英文界面切换
- 隐藏商业版入口与商业版提示
- Docker 与 Kubernetes 环境管理
- 环境级数据库工作台
- MySQL、MariaDB、PostgreSQL、Redis 连接与查询
- 数据库连接、库表树、SQL 执行、执行历史、结果复制

## 环境要求

本地开发建议准备以下工具：

- Git
- Docker Desktop
- Kubernetes 可选，Docker Desktop 内置 Kubernetes 即可
- Node.js 22
- PNPM 10+
- Go 1.26.1
- GNU Make 可选，Windows 下也可以直接执行 PNPM 和 Go 命令

## 拉取代码

```powershell
git clone https://github.com/czhqwer/portainer-cn.git
cd portainer-cn
pnpm install
```

如果你使用自己的 fork，把 clone 地址替换成自己的仓库地址即可。

## 本地启动

### 方式一：使用项目开发命令

适合 Linux、macOS 或已配置 Make 的 Windows 环境：

```powershell
make dev
```

服务默认端口：

- 前端开发服务：http://localhost:8999
- 后端服务：http://localhost:9000
- HTTPS 服务：https://localhost:9443

也可以拆开启动：

```powershell
make dev-client
make dev-server
```

### 方式二：Windows 本地构建后用 Docker 运行

先构建前端：

```powershell
$env:NODE_ENV = "development"
pnpm run build --config webpack/webpack.development.js
```

再构建 Linux 后端二进制：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist/portainer ./api/cmd/portainer
```

如果你已经创建了本地开发容器，可以直接重启：

```powershell
docker restart portainer-cn-dev
```

### 方式三：Mac 本地构建后用 Docker 运行

先安装依赖：

```bash
pnpm install
```

构建前端：

```bash
NODE_ENV=development pnpm run build --config webpack/webpack.development.js
```

构建 Linux 后端二进制：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/portainer ./api/cmd/portainer
```

如果你已经创建了本地开发容器，可以直接重启：

```bash
docker restart portainer-cn-dev
```

如果还没有本地开发容器，可以使用下面的方式运行一个开发容器：

```bash
docker volume create portainer_data

docker run -d \
  --name portainer-cn-dev \
  --restart=always \
  -p 8000:8000 \
  -p 9000:9000 \
  -p 9443:9443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v portainer_data:/data \
  -v "$(pwd)/dist:/app/dist" \
  portainer/base
```

访问：

```text
http://localhost:9000
```

首次启动需要在页面中创建管理员账号。后续使用你创建的管理员账号登录。

## 启用 Portainer Agent

如果需要使用主机文件浏览、上传、下载等主机管理能力，Docker 环境必须通过 Portainer Agent 接入，并且 Agent 容器需要把宿主机根目录挂载到 `/host`。

### 本地 Docker Agent

启动 Agent：

```powershell
docker run -d `
  --name portainer_agent `
  --restart=always `
  -p 9001:9001 `
  -v /var/run/docker.sock:/var/run/docker.sock `
  -v /var/lib/docker/volumes:/var/lib/docker/volumes `
  -v /:/host `
  portainer/agent:latest
```

建议把 Portainer 和 Agent 放到同一个 Docker 网络，方便 Portainer 通过容器名访问 Agent：

```powershell
docker network create portainer-agent-net
docker network connect portainer-agent-net portainer-cn-dev
docker network connect portainer-agent-net portainer_agent
```

在 Portainer 页面中新增环境：

```text
环境类型：Docker Standalone
连接方式：Agent
名称：local-agent
Agent 地址：portainer_agent:9001
TLS：开启
跳过服务器证书校验：开启
跳过客户端证书校验：开启
```

如果 Portainer 和 Agent 不在同一个 Docker 网络，也可以把 Agent 地址改成：

```text
host.docker.internal:9001
```

### 启用主机管理

进入 Agent 环境后，在功能配置中开启主机管理：

```text
主机 → 功能配置 → 启用主机管理功能
```

开启后可以在“主机”页面浏览 `/host`，并对可访问路径进行文件浏览、上传、下载、删除、重命名等操作。

注意：在 Windows Docker Desktop 中，`/:/host` 指向 Docker Desktop Linux VM 的文件系统视角，不等同于 Windows 的 `C:\` 根目录。若只是管理容器数据，优先使用数据卷浏览通常更稳定。

## 数据库工作台

进入某个 Docker 或 Kubernetes 环境后，可以在左侧环境菜单中打开“数据库”。

数据库连接支持两种目标：

- 容器：选择容器后自动带出容器内部 IP 和常见数据库端口，仍然允许手动修改。
- 自定义地址：连接 Portainer 服务端可访问的任意数据库地址。

注意：数据库连接是由 Portainer 后端发起的，所以 Host 必须能被 Portainer 服务端访问。比如数据库在宿主机上时，容器内访问 `127.0.0.1` 通常指向 Portainer 容器本身，不是宿主机。

## 部署

### 构建镜像

```powershell
make build-image
```

或者先手动构建前端和后端，再按自己的镜像流水线打包。

### Docker 部署

示例：

```powershell
docker volume create portainer_data

docker run -d `
  --name portainer-cn `
  --restart=always `
  -p 9000:9000 `
  -p 9443:9443 `
  -v /var/run/docker.sock:/var/run/docker.sock `
  -v portainer_data:/data `
  czhqwer/portainer-cn:latest
```

如果使用本地构建的镜像，把 `czhqwer/portainer-cn:latest` 替换成你的镜像名。

### Kubernetes 部署

部署思路：

1. 构建并推送镜像到你的镜像仓库。
2. 为 Portainer 配置持久化数据卷，挂载到 `/data`。
3. 使用 `Deployment` 或 `StatefulSet` 运行 Portainer。
4. 使用 `Service` 暴露 `9000` 或 `9443` 端口。
5. 如需管理集群资源，按 Portainer Agent 模式接入 Kubernetes 环境。

最小化部署示例可按你的集群规范编写，关键配置是镜像、持久化数据卷和服务端口。

## 常用验证命令

```powershell
pnpm typecheck
pnpm test
go test ./api/http/handler/endpoints ./api/http/handler/docker/containers
```

构建前端：

```powershell
$env:NODE_ENV = "development"
pnpm run build --config webpack/webpack.development.js
```

构建后端：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist/portainer ./api/cmd/portainer
```

## 许可证

本项目基于 Portainer Community Edition 二次开发，原项目许可证见 [LICENSE](./LICENSE)。
