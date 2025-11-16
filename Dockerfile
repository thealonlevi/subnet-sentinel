FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o subnet-sentinel ./cmd/subnet-sentinel

FROM alpine:3.20

RUN adduser -D -H -s /sbin/nologin subnet && \
    mkdir -p /etc/subnet-sentinel

WORKDIR /etc/subnet-sentinel

COPY --from=builder /app/subnet-sentinel /usr/local/bin/subnet-sentinel
COPY config.yaml /etc/subnet-sentinel/config.yaml

USER subnet

ENV SUBNET_SENTINEL_CONFIG=/etc/subnet-sentinel/config.yaml

ENTRYPOINT ["/usr/local/bin/subnet-sentinel"]
