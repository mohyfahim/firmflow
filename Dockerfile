FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /firmflow-api ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /firmflow-api /app/firmflow-api
COPY .env.example /app/.env.example

EXPOSE 8080
ENTRYPOINT ["/app/firmflow-api"]
