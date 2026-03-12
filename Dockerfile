# Build stage
FROM golang:1.24-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    ca-certificates \
    git \
    libtesseract-dev \
    libleptonica-dev \
    tesseract-ocr \
    tesseract-ocr-eng \
    tesseract-ocr-chi-sim \
  && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o cursor2api-go .

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    wget \
    tesseract-ocr \
    tesseract-ocr-eng \
    tesseract-ocr-chi-sim \
    libtesseract-dev \
    libleptonica-dev \
  && rm -rf /var/lib/apt/lists/* \
  && useradd -r -m -d /app appuser

WORKDIR /app

COPY --from=builder /app/cursor2api-go ./cursor2api-go
COPY --from=builder /app/static ./static

RUN chown -R appuser:appuser /app
USER appuser

EXPOSE 8002

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8002/health || exit 1

CMD ["./cursor2api-go"]
