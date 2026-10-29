# ---------- 构建阶段 ----------
FROM golang:1.27-alpine AS builder

WORKDIR /src

# 先复制依赖清单，利用 Docker 层缓存：go.mod / go.sum 未变时不会重复下载依赖
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 产出静态二进制；-trimpath 去除构建机路径；-s -w 去掉符号表与调试信息
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server .

# ---------- 运行阶段 ----------
FROM alpine:3.24

# 仅保留 CA 证书与时区数据，不携带 Go 工具链与源码
RUN apk --no-cache add ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/server ./server
RUN chown -R appuser:appuser /app

# 以非 root 用户运行，降低容器逃逸后的影响面
USER appuser

ENV APP_ENV=production

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/health >/dev/null 2>&1 || exit 1

CMD ["./server"]
