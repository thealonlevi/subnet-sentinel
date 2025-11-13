FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o subnet-sentinel ./cmd/subnet-sentinel

FROM alpine:3.20
RUN adduser -D appuser
WORKDIR /
COPY --from=builder /app/subnet-sentinel /usr/local/bin/subnet-sentinel
RUN mkdir /config
USER appuser
ENTRYPOINT ["/usr/local/bin/subnet-sentinel"]
CMD ["run","--config","/config/config.yaml"]
