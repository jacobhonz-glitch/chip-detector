FROM golang:1.25 AS builder
RUN apt-get update && apt-get install -y libusb-1.0-0-dev pkg-config
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o chip-detector main.go
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y libusb-1.0-0 ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/chip-detector /app/chip-detector
COPY --from=builder /app/web /app/web
WORKDIR /app
EXPOSE 8080
CMD ["./chip-detector"]
