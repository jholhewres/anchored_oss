# Stage 1: build the admin dashboard
FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY web/ .
RUN npm run build

# Stage 2: compile the Go binary with the SPA bundle embedded
FROM golang:1.25-alpine AS builder
ARG VERSION=dev
ENV VERSION_PKG=github.com/jholhewres/anchored_oss/internal/version
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X ${VERSION_PKG}.Version=${VERSION}" \
    -o /anchored-oss ./cmd/server

# Stage 3: minimal runtime
FROM alpine:3.21
RUN apk --no-cache add ca-certificates
COPY --from=builder /anchored-oss /usr/local/bin/anchored-oss
EXPOSE 8080
ENTRYPOINT ["anchored-oss"]
