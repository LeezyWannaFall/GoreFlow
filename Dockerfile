FROM golang:1.26.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -o server ./cmd/app

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .

CMD ["./server"]