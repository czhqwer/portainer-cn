# Portainer.CN

Portainer.CN 是基于 [Portainer Community Edition](https://github.com/portainer/portainer) 的中文二次开发版本。它保留 Portainer CE 对 Docker、Docker Swarm、Kubernetes 的核心管理能力，同时移除或隐藏商业版入口，并补充更适合中文日常使用的界面与数据库管理能力。

本项目不是 Portainer 官方版本。原项目版权与许可证请参考 [LICENSE](./LICENSE)。

## 主要改造

- 中文化与中英文切换。
- 隐藏商业版入口与商业版功能提示。
- 品牌与界面体验调整。
- 新增环境级数据库工作台。
- 优化 Docker Agent、本地开发和主机管理相关体验。

## 环境要求

- Git
- Docker Desktop
- Node.js 22，建议使用项目要求的 `^22.22.1`
- PNPM 10+
- Go 1.26.1
- macOS/Linux 建议安装 `make`
- Windows 可使用 PowerShell 直接执行下面的命令

## 拉取代码

```bash
git clone https://github.com/czhqwer/portainer-cn.git
cd portainer-cn
pnpm install
```

如果你使用自己的 fork，把 clone 地址替换为你的仓库地址即可。

## 本地开发启动

本地开发建议分两个服务运行：

- 后端 API：http://localhost:9000
- 前端开发服务：http://localhost:8999

开发时优先访问 `http://localhost:8999`，前端改动会由 webpack dev server 热更新，API 请求会代理到 `http://localhost:9000`。

### Windows 启动

PowerShell 中执行：

```powershell
pnpm install

# 先生成一次前端静态资源，供后端 assets 参数使用
$env:NODE_ENV = "development"
pnpm run build --config webpack/webpack.development.js

# 准备本地数据目录。已有容器数据时，建议先 docker cp 出来再使用
New-Item -ItemType Directory -Force .tmp\portainer-data-local | Out-Null

# 终端 1：启动后端
& "E:\develop\Go\bin\go.exe" run .\api\cmd\portainer `
  --data .\.tmp\portainer-data-local `
  --assets .\dist `
  --bind :9000 `
  --bind-https :9443 `
  --tunnel-port 8000 `
  --http-enabled

# 终端 2：启动前端
pnpm dev
```

如果你的 `go.exe` 已经在 PATH 中，可以把 `& "E:\develop\Go\bin\go.exe"` 改成 `go`。

如果你之前已经用容器跑过 Portainer，想复用已有账号和环境配置，可以先复制容器中的 `/data`：

```powershell
docker cp portainer-cn-dev:/data .tmp\portainer-data-local
docker stop portainer-cn-dev
```

### Mac 启动

```bash
pnpm install

# 先生成一次前端静态资源，供后端 assets 参数使用
NODE_ENV=development pnpm run build --config webpack/webpack.development.js

# 准备本地数据目录
mkdir -p .tmp/portainer-data-local

# 终端 1：启动后端
go run ./api/cmd/portainer \
  --data ./.tmp/portainer-data-local \
  --assets ./dist \
  --bind :9000 \
  --bind-https :9443 \
  --tunnel-port 8000 \
  --http-enabled

# 终端 2：启动前端
pnpm dev
```

如果你之前已经用容器跑过 Portainer，想复用已有账号和环境配置，可以先复制容器中的 `/data`：

```bash
docker cp portainer-cn-dev:/data .tmp/portainer-data-local
docker stop portainer-cn-dev
```

## 构建 Docker 镜像

镜像构建依赖 `dist` 目录中的三类内容：

- `dist/public`：前端静态资源
- `dist/portainer`：Linux 后端二进制
- `dist/mustache-templates`：模板文件

### Windows 构建容器

PowerShell 中执行：

```powershell
pnpm install

$env:NODE_ENV = "production"
pnpm run build --config webpack/webpack.production.js

New-Item -ItemType Directory -Force dist | Out-Null
Copy-Item -Recurse -Force mustache-templates dist\mustache-templates
New-Item -ItemType Directory -Force dist\storybook | Out-Null

$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist\portainer .\api\cmd\portainer

docker buildx build --load -t portainer-cn:local -f build/linux/Dockerfile .
```

构建完成后可直接运行：

```powershell
docker volume create portainer_data

docker run -d `
  --name portainer-cn `
  --restart=always `
  -p 8000:8000 `
  -p 9000:9000 `
  -p 9443:9443 `
  -v /var/run/docker.sock:/var/run/docker.sock `
  -v portainer_data:/data `
  portainer-cn:local
```

### Mac 构建容器

```bash
pnpm install

NODE_ENV=production pnpm run build --config webpack/webpack.production.js

mkdir -p dist
cp -R mustache-templates dist/
mkdir -p dist/storybook

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o dist/portainer ./api/cmd/portainer

docker buildx build --load -t portainer-cn:local -f build/linux/Dockerfile .
```

构建完成后可直接运行：

```bash
docker volume create portainer_data

docker run -d \
  --name portainer-cn \
  --restart=always \
  -p 8000:8000 \
  -p 9000:9000 \
  -p 9443:9443 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v portainer_data:/data \
  portainer-cn:local
```

## Docker Compose 部署

下面示例会同时部署 Portainer.CN 和 Portainer Agent。Agent 用于主机管理、文件浏览、上传下载等能力。

创建 `docker-compose.yml`：

```yaml
services:
  portainer-cn:
    image: portainer-cn:local
    container_name: portainer-cn
    restart: always
    ports:
      - '8000:8000'
      - '9000:9000'
      - '9443:9443'
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - portainer_data:/data
    networks:
      - portainer

  portainer-agent:
    image: portainer/agent:latest
    container_name: portainer-agent
    restart: always
    ports:
      - '9001:9001'
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /var/lib/docker/volumes:/var/lib/docker/volumes
      - /:/host
    networks:
      - portainer

volumes:
  portainer_data:

networks:
  portainer:
    name: portainer-agent-net
```

启动：

```bash
docker compose up -d
```

停止：

```bash
docker compose down
```

查看日志：

```bash
docker compose logs -f portainer-cn
```

访问：

```text
http://localhost:9000
```

首次启动需要在页面中创建管理员账号。

### Agent 环境配置

如果 Portainer.CN 和 Agent 使用上面的 compose 在同一网络中，添加 Docker Agent 环境时可以填写：

```text
环境类型：Docker Standalone
连接方式：Agent
名称：local-agent
Agent 地址：portainer-agent:9001
TLS：开启
跳过服务器证书校验：开启
跳过客户端证书校验：开启
```

如果 Portainer.CN 是本地进程运行，而 Agent 是 Docker 容器运行，Agent 地址建议使用：

```text
localhost:9001
```

注意：Windows Docker Desktop 中的 `/:/host` 通常是 Docker Desktop Linux VM 的根目录视角，不等同于 Windows 的 `C:\` 根目录。

## 数据库工作台说明

进入 Docker 或 Kubernetes 环境后，可以从左侧菜单打开“数据库”。

连接目标支持两种模式：

- 容器：选择容器后自动建议容器内部 IP 和数据库端口，字段仍允许手动修改。
- 自定义地址：连接 Portainer 后端可以访问到的任意数据库地址。

数据库连接由 Portainer 后端发起，因此 Host 必须能被 Portainer 服务端访问：

- Portainer 跑在容器里时，`127.0.0.1` 指向 Portainer 容器自身。
- Portainer 跑在 Windows/Mac 本地时，`127.0.0.1` 指向本机。
- 数据库跑在 Docker 容器里且暴露了端口时，本地开发可使用 `127.0.0.1:映射端口`。
- Portainer 容器和数据库容器在同一 Docker 网络时，可使用容器名和内部端口。

## 常用验证命令

```bash
pnpm typecheck
pnpm test
go test ./api/http/handler/endpoints ./api/http/handler/docker/containers
```

## 常见问题

### 为什么本地开发不需要每次重新构建容器？

前端使用 `pnpm dev` 后会由 webpack dev server 进行热更新；后端使用 `go run ./api/cmd/portainer` 直接在本机运行。只有需要验证最终镜像、部署脚本、容器网络或 Linux 容器内行为时，才需要重新构建镜像并重启容器。

### 为什么切换到本地后 Agent 地址要变？

容器内可以通过 Docker 网络访问 `portainer-agent:9001`，但本地 Windows/Mac 进程不能解析 Docker 网络里的容器名。本地后端连接 Agent 时，应使用 Docker 暴露到宿主机的地址，例如 `localhost:9001`。

### 如何开启主机文件管理？

需要通过 Portainer Agent 接入环境，并确保 Agent 挂载了 `/host`。在环境中进入：

```text
主机 -> 功能配置 -> 启用主机管理功能
```

启用后可以在主机页面浏览文件，并执行上传、下载、删除、重命名等操作。
