FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/pulse ./cmd/pulse

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/pulse /usr/local/bin/pulse
COPY .env.example /app/.env
EXPOSE 8080
CMD ["pulse"]
