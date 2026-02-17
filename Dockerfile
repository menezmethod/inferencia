# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /inferencia ./cmd/inferencia

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
USER nobody
EXPOSE 8080
ENTRYPOINT ["/inferencia"]
# No config file in image: app uses defaults + INFERENCIA_* env vars
