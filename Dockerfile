FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o subnet-sentinel ./cmd/subnet-sentinel

FROM gcr.io/distroless/base-debian12
COPY --from=builder /app/subnet-sentinel /usr/local/bin/subnet-sentinel
WORKDIR /
RUN mkdir -p /config
ENTRYPOINT ["/usr/local/bin/subnet-sentinel"]
CMD ["run", "--config", "/config/config.yaml"]

