FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download

COPY . .
RUN go build -o merchant-service ./cmd/merchant-service

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/merchant-service .

EXPOSE 50055
CMD ["./merchant-service"]