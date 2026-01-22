# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o wa-cloud-proxy .

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/wa-cloud-proxy .

RUN mkdir -p /app/data

VOLUME ["/app/data"]

EXPOSE 9000

ENTRYPOINT ["./wa-cloud-proxy"]
CMD ["-config", "/app/config.yaml"]
