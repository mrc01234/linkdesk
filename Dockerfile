FROM golang:1.27.3-alpine3.24 AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go build -trimpath -o /out/linkdesk ./cmd

FROM alpine:3.24
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app && mkdir -p /data && chown app:app /data
WORKDIR /app
COPY --from=builder /out/linkdesk /app/linkdesk
COPY templates ./templates
COPY static ./static
COPY LICENSE UPSTREAM.md ./
USER app
ENV HOST=0.0.0.0 PORT=8080 DB_PATH=/data/linkdesk.db BASE_URL=http://localhost:8080
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- "http://127.0.0.1:${PORT}/healthz" >/dev/null || exit 1
ENTRYPOINT ["/app/linkdesk"]
