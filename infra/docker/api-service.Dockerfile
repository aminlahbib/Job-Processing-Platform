# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.work go.work
COPY backend/shared backend/shared
COPY backend/api-service backend/api-service
RUN go build -o /api-service ./backend/api-service/

# Run stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /api-service /api-service
EXPOSE 8080
ENTRYPOINT ["/api-service"]
