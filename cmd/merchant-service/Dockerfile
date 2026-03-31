FROM golang:1.21-alpine

WORKDIR /app

COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download

COPY . .

WORKDIR /app/cmd/merchant-service

RUN go build -o merchant-service .

EXPOSE 50055

CMD ["./merchant-service"]
