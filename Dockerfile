FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o sub-subscribe -ldflags="-s -w" .

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/sub-subscribe .

EXPOSE 8080

CMD ["./sub-subscribe"]
