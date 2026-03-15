# --- Build stage ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -trimpath -o /bin/cursor2api-go .

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/cursor2api-go /usr/local/bin/cursor2api-go

# Default working directory for config.yaml auto-generation
WORKDIR /app
COPY static/ /app/static/

EXPOSE 8002

ENTRYPOINT ["cursor2api-go"]
