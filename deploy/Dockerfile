# syntax=docker/dockerfile:1.6
#
# Multi-stage build:
# - go-builder: build gpt2api + goose
# - web-builder: build web/dist
# - runtime: alpine runtime image
#
# This Dockerfile does not require host Go/Node or prebuilt artifacts.

FROM golang:1.26-alpine AS go-builder
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.org
ARG TARGETOS=linux
ARG TARGETARCH
ENV GOPROXY=$GOPROXY \
    GOSUMDB=$GOSUMDB

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/gpt2api ./cmd/server
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOBIN=/out CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go install github.com/pressly/goose/v3/cmd/goose@v3.20.0

FROM node:20-alpine AS web-builder
WORKDIR /src/web
ARG NPM_REGISTRY=https://registry.npmmirror.com

COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm config set registry ${NPM_REGISTRY} \
    && \
    npm ci --no-audit --no-fund --loglevel=error

COPY web/ ./
RUN npm run build

FROM alpine:3.20

RUN sed -i 's@https\?://dl-cdn.alpinelinux.org@http://mirrors.ustc.edu.cn@g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata curl bash mariadb-client \
    && update-ca-certificates \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

COPY --from=go-builder /out/gpt2api /app/gpt2api
COPY --from=go-builder /out/goose /usr/local/bin/goose
RUN chmod +x /app/gpt2api /usr/local/bin/goose

COPY --from=web-builder /src/web/dist /app/web/dist
COPY sql /app/sql
COPY configs /app/configs
COPY deploy/entrypoint.sh /app/entrypoint.sh
RUN sed -i 's/\r$//' /app/entrypoint.sh \
    && chmod +x /app/entrypoint.sh \
    && mkdir -p /app/data/backups /app/logs

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/gpt2api", "-c", "/app/configs/config.yaml"]
