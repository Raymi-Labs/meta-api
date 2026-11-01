ARG FRONTEND_SOURCE=builder

# Compile on the builder CPU; only the runtime stage uses the target platform.
FROM --platform=$BUILDPLATFORM oven/bun:1.4.0@sha256:5ff609364c049b54eb0ff560ec96319729a972078ef2c755d758f0c6ef89c2d6 AS builder

# 预渲染（SEO 阶段 2）需要在 build 后跑 puppeteer-core 控制真浏览器抓取页面 HTML，
# 这里在 builder 阶段装系统 chromium + 中文字体（避免预渲染 HTML 出现方框乱码）。
# 发票 PDF 在 runtime 中也使用 Chromium；两个阶段各自安装目标架构依赖。
RUN apt-get update \
    && apt-get install -y --no-install-recommends chromium fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*
ENV PUPPETEER_SKIP_DOWNLOAD=true \
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium

WORKDIR /build
COPY web/package.json .
COPY web/bun.lock .
RUN bun install --frozen-lockfile
COPY ./web .
COPY ./docs /docs
COPY ./VERSION .
ARG APP_VERSION=""
RUN BUILD_VERSION="${APP_VERSION:-$(cat VERSION 2>/dev/null)}"; \
    if [ -z "$BUILD_VERSION" ]; then BUILD_VERSION="dev"; fi; \
    DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION="$BUILD_VERSION" bun run build

FROM scratch AS frontend-prebuilt
COPY ./web/dist /build/dist

# BuildKit skips the source frontend stage when CI supplies the verified artifact.
FROM ${FRONTEND_SOURCE} AS frontend

FROM --platform=$BUILDPLATFORM golang:1.26.10-alpine AS builder2
ENV GO111MODULE=on CGO_ENABLED=0
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download
# Keep the entire cross-database-tested driver set paired; mixing old dialect
# drivers with newer GORM can misclassify MySQL unique indexes during migration.
RUN test "$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' github.com/glebarez/sqlite)" = "v1.11.0" \
    && test "$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' gorm.io/gorm)" = "v1.25.12" \
    && test "$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' gorm.io/driver/mysql)" = "v1.5.7" \
    && test "$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' gorm.io/driver/postgres)" = "v1.5.9"

COPY . .
COPY --from=frontend /build/dist ./web/dist
# Release metadata must not invalidate downloaded Go modules.
ARG APP_VERSION=""
ARG BUILD_COMMIT=""
ARG BUILD_REPOSITORY="QuantumNous/new-api"
ARG BUILD_BRANCH="main"
RUN BUILD_VERSION="${APP_VERSION:-$(cat VERSION 2>/dev/null)}"; \
    if [ -z "$BUILD_VERSION" ]; then BUILD_VERSION="dev"; fi; \
    go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=${BUILD_VERSION}' -X 'github.com/QuantumNous/new-api/common.BuildCommit=${BUILD_COMMIT}' -X 'github.com/QuantumNous/new-api/common.BuildRepository=${BUILD_REPOSITORY}' -X 'github.com/QuantumNous/new-api/common.BuildBranch=${BUILD_BRANCH}'" -o new-api

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget chromium fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=builder2 /build/new-api /
ENV INVOICE_PDF_CHROME_PATH=/usr/bin/chromium
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
