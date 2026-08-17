# ============================================================
# q-iam 多阶段 Docker 构建
#   阶段 1  ui-builder : 构建 Vue 前端（产物 ui/dist，后续经 go:embed 打进二进制）
#   阶段 2  builder    : 编译 Go 后端（SQLite 驱动依赖 CGO，故启用 CGO 编译）
#   阶段 3  runtime    : 精简运行镜像（alpine + 单二进制）
# 构建：
#   docker build -t zhiqiangwang/app:q-iam .
# ============================================================

# ---------- 阶段 1：前端构建 ----------
FROM node:22-alpine AS ui-builder
WORKDIR /app/ui
# 先复制依赖清单，利用 Docker 层缓存，避免每次构建都重装依赖
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
# 复制前端源码并构建（npm run build = vue-tsc -b && vite build）
COPY ui/ ./
RUN npm run build

# ---------- 阶段 2：Go 后端编译 ----------
FROM golang:1.25-alpine AS builder
# SQLite 驱动 (github.com/mattn/go-sqlite3) 依赖 CGO，需要 gcc 与 musl-dev
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
# ENV GOPROXY=https://goproxy.cn,direct
# 先下载依赖，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download
# 复制源码与 Docker 专用配置
COPY . .
# 覆盖为阶段1构建出的前端产物（go:embed all:dist 要求 dist 目录存在）
COPY --from=ui-builder /app/ui/dist ./ui/dist
ENV CGO_ENABLED=1 GOOS=linux
RUN go build -trimpath -ldflags "-s -w" -o /out/q-iam .

# ---------- 阶段 3：运行镜像 ----------
FROM alpine:3.20
# ca-certificates：HTTPS 出站（Redis 等）；tzdata：时区数据；libc6-compat：glibc 兼容
RUN apk add --no-cache ca-certificates tzdata libc6-compat
WORKDIR /app
COPY --from=builder /out/q-iam /app/q-iam
# Docker 专用配置复制为 /app/config.yaml（main.go 固定加载 config.yaml）
COPY --from=builder /app/config.docker.yaml /app/config.yaml
# 非敏感默认值：数据库驱动与连接由环境变量提供默认值（可运行时 -e 覆盖）。
# 注意：JWT_SECRET 为敏感数据，不固化进镜像，运行容器时必须用 -e 注入。
ENV DB_DRIVER=sqlite \
    DB_DATABASE=./data.db
# 运行数据目录（挂载卷：-v $(pwd)/data:/app/data，DB_DATABASE 指向 ./data/data.db 以持久化）
RUN mkdir -p /app/data
EXPOSE 8080
# 健康检查：/health 接口（busybox wget）
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1
# 以非 root 用户运行（更安全）
RUN addgroup -S qiam && adduser -S -G qiam qiam && chown -R qiam:qiam /app
USER qiam
CMD ["/app/q-iam"]
