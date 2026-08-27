# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

# 依赖单独成层，源码变动不触发 go mod download 重跑
WORKDIR /src/backend/risk-engine

COPY backend/risk-engine/go.mod backend/risk-engine/go.sum ./
RUN go mod download

COPY backend/risk-engine/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/risk-engine ./cmd/server

# ---------- Runtime stage ----------
FROM alpine:3.20

RUN apk add --no-cache tzdata ca-certificates \
    && adduser -D -u 10001 appuser

# 复刻仓库结构，使 ../../binfiles、../../frontend、./configs 相对路径原样生效
WORKDIR /app/backend/risk-engine

COPY --from=builder /out/risk-engine ./risk-engine
COPY docker/config.docker.yaml ./configs/config.yaml
COPY frontend/ /app/frontend

RUN mkdir -p /app/binfiles ./logs \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["./risk-engine"]
