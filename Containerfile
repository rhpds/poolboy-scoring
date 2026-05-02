# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Build static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /poolboy-scoring ./cmd

# Runtime stage
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /poolboy-scoring /poolboy-scoring

USER 65532:65532

ENTRYPOINT ["/poolboy-scoring"]
