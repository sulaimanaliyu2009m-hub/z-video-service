# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o z-video-service main.go

# Production Minimal Stage
FROM alpine:3.19

WORKDIR /app
COPY --from=builder /app/z-video-service .

EXPOSE 8080

CMD ["./z-video-service"]
