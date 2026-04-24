FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-builder
WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.org
ARG TARGETOS=linux
ARG TARGETARCH
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG BUILD_JOBS=2

ENV GOPROXY=$GOPROXY \
    GOSUMDB=$GOSUMDB \
    GOMAXPROCS=$BUILD_JOBS

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN set -eux; \
    os="${TARGETOS}"; \
    arch="${TARGETARCH}"; \
    if [ -z "${os}" ]; then os="$(go env GOOS)"; fi; \
    if [ -z "${arch}" ]; then arch="$(go env GOARCH)"; fi; \
    echo "go-builder: BUILDPLATFORM=${BUILDPLATFORM} TARGETPLATFORM=${TARGETPLATFORM} GOOS=${os} GOARCH=${arch}"; \
    CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" \
      go build -p=${BUILD_JOBS} -o /out/gpt2api ./cmd/server

RUN mkdir -p /tmp/goose-build \
    && cd /tmp/goose-build \
    && go mod init goose-build \
    && go get github.com/pressly/goose/v3/cmd/goose@v3.20.0 \
    && os="${TARGETOS}" \
    && arch="${TARGETARCH}" \
    && if [ -z "${os}" ]; then os="$(go env GOOS)"; fi \
    && if [ -z "${arch}" ]; then arch="$(go env GOARCH)"; fi \
    && echo "goose-builder: BUILDPLATFORM=${BUILDPLATFORM} TARGETPLATFORM=${TARGETPLATFORM} GOOS=${os} GOARCH=${arch}" \
    && CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" \
       go build -p=${BUILD_JOBS} -o /out/goose github.com/pressly/goose/v3/cmd/goose

FROM --platform=$BUILDPLATFORM node:20-alpine AS web-builder
WORKDIR /src/web

ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG BUILD_JOBS=2

ENV npm_config_jobs=$BUILD_JOBS

COPY web/package.json web/package-lock.json ./
RUN npm config set registry ${NPM_REGISTRY} \
    && npm ci --no-audit --no-fund --loglevel=error

COPY web/ ./
RUN npm run build

FROM --platform=$TARGETPLATFORM alpine:3.20

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
