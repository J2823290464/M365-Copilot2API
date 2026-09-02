FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct
ENV GOPROXY=${GOPROXY}
RUN go mod download
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w \
    -X m365-copilot2api/internal/web.Version=${VERSION} \
    -X m365-copilot2api/internal/web.Commit=${COMMIT} \
    -X m365-copilot2api/internal/web.BuildTime=${BUILD_TIME}" \
    -o /out/m365-copilot2api ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S m365 && adduser -S -G m365 m365 \
    && mkdir -p /data /app
WORKDIR /app
COPY --from=build /out/m365-copilot2api /app/m365-copilot2api
COPY --from=build /src/web /app/web
RUN chown -R m365:m365 /app /data
USER m365
EXPOSE 4141
ENV M365_LISTEN=0.0.0.0:4141 \
    M365_DATA_DIR=/data \
    M365_CONFIG=/data/accounts.json \
    M365_TOKEN_CACHE=/data/token-cache.json \
    M365_SESSION_CACHE=/data/sessions.json \
    M365_CONVERSATION_CACHE=/data/conversations.json \
    M365_API_KEYS=/data/api-keys.json \
    M365_ADMIN_PASSWORD_BOOTSTRAP_FILE=/run/secrets/m365_admin_password
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:4141/api/health || exit 1
ENTRYPOINT ["/app/m365-copilot2api"]
