# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o telnyx-validator .

# Run stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget

COPY --from=builder /app/telnyx-validator /usr/local/bin/telnyx-validator

EXPOSE 8080

ENTRYPOINT ["telnyx-validator"]
