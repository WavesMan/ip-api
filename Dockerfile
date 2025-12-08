## 多阶段构建：先构建前端静态资源，再构建 Go 后端，最后打包为精简运行镜像

# ---------- 前端构建阶段 ----------
FROM node:22-alpine AS ui-builder
WORKDIR /app/ui

# 仅复制依赖清单，提升缓存复用效率
COPY ui/package.json ./

# 优先使用 ci，如无锁文件则回退 install
RUN npm cache clean --force || true && rm -rf ~/.npm/_cacache || true && \
    if [ -f package-lock.json ]; then npm ci --no-audit --no-fund; else npm install --no-audit --no-fund; fi

# 复制前端源码并构建产物
COPY ui/ .
RUN npm run build


# ---------- 后端构建阶段 ----------
FROM golang:1.22-alpine AS go-builder
WORKDIR /src

# 复制 go.mod/go.sum 先拉依赖，提升缓存复用
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# 复制后端源码并构建静态二进制
COPY . .
ARG GIT_SHA=unknown
ARG BUILD_TIME=unknown
RUN go clean -cache -modcache -testcache || true && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X ip-api/internal/version.Commit=$GIT_SHA -X ip-api/internal/version.BuiltAt=$BUILD_TIME" -o /out/ip-api ./cmd/main.go


# ---------- 运行时镜像 ----------
FROM alpine:3.20

# 创建非特权用户
RUN adduser -D -H -u 10001 appuser
RUN apk add --no-cache su-exec

WORKDIR /app

# 复制后端可执行文件与前端静态资源
COPY --from=go-builder /out/ip-api /app/ip-api
COPY --from=ui-builder /app/ui/dist /app/ui/dist

# 预创建数据目录（本地文件缓存与 IPIP 数据位置），建议挂载为持久卷
RUN mkdir -p /app/data/localdb /app/data/ipip
RUN chown -R appuser:appuser /app/data
COPY data/ipip/ipipfree.ipdb /app/data/ipip/ipipfree.ipdb
RUN chown -R appuser:appuser /app

# 默认环境变量（可在运行时覆盖）
ENV ADDR=:8080 \
    API_BASE=/api \
    UI_DIST=/app/ui/dist

# 对外暴露端口
EXPOSE 8080

# 切换非特权用户并启动
USER root
ENTRYPOINT ["/bin/sh","-c","chown -R 10001:10001 /app/data/localdb /app/data/ipip 2>/dev/null || true; exec su-exec appuser /app/ip-api"]
