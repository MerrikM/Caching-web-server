FROM golang:1.24.4-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o app ./cmd/main.go

FROM alpine:3.18
WORKDIR /app

COPY --from=builder /app/app .
COPY --from=builder /app/config.yaml ./config.yaml

EXPOSE 8080

CMD ["./app"]
