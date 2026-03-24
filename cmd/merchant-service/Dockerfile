FROM golang:1.24-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /app/cmd/merchant-service

RUN go build -o merchant-service .

EXPOSE 50054

CMD ["./merchant-service"]
