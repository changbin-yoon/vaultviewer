## Frontend build
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

## Backend build
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -o /out/vaultviewer ./cmd/server

## Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates git && \
    addgroup -g 65532 vaultviewer && \
    adduser -D -u 65532 -G vaultviewer vaultviewer
WORKDIR /app
COPY --from=backend /out/vaultviewer ./vaultviewer
COPY --from=frontend /app/web/dist ./web/dist
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["./vaultviewer"]
