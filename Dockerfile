FROM golang:1.23.4-alpine3.21 AS builder

WORKDIR /src

# Dependencies are resolved first so the module layer survives source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary: the runtime stage carries no toolchain and no libc to link to.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/field-service

FROM alpine:latest

# ca-certificates for outbound TLS (GCS, internal services) and tzdata because
# the service pins time.Local to Asia/Jakarta at startup.
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -H -u 10001 appuser

WORKDIR /app

# Only the compiled binary crosses the stage boundary: no sources, no .env,
# no config.json, no build toolchain.
COPY --from=builder /out/field-service /app/field-service

USER appuser

EXPOSE 8002

ENTRYPOINT [ "/app/field-service" ]
