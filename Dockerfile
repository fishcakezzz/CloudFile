FROM golang:1.23-bookworm AS builder

WORKDIR /src
COPY backend/go.mod ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=1 go build -o /out/cloudfile-api ./cmd/api \
    && CGO_ENABLED=1 go build -o /out/cloudfile-worker ./cmd/worker

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/cloudfile-api /usr/local/bin/cloudfile-api
COPY --from=builder /out/cloudfile-worker /usr/local/bin/cloudfile-worker

EXPOSE 8000
CMD ["cloudfile-api"]
