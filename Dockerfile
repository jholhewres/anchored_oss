# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.versionFlag=$(cat /dev/null || echo dev)" -o /anchored-oss ./cmd/server

# Runtime stage
FROM alpine:3.21
RUN apk --no-cache add ca-certificates
COPY --from=builder /anchored-oss /usr/local/bin/anchored-oss
EXPOSE 8080
ENTRYPOINT ["anchored-oss"]
