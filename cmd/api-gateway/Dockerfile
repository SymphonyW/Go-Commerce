FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download

COPY . .
RUN go build -o api-gateway ./cmd/api-gateway

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/api-gateway .

EXPOSE 8080
CMD ["./api-gateway"]