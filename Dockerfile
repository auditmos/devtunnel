# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git for version info
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o devtunnel ./cmd/devtunnel

# Final stage - alpine for SSL certs
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/devtunnel /usr/local/bin/devtunnel

# Data directory for certs and DB
VOLUME /data

ENV HOME=/data

EXPOSE 80 443 4040

ENTRYPOINT ["devtunnel"]
CMD ["server"]
