# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o bifrost .

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bifrost .

RUN mkdir -p /app/data

VOLUME ["/app/data"]

EXPOSE 9000

ENTRYPOINT ["./bifrost"]
CMD ["-config", "/app/config.yaml"]
