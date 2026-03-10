FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY backend/shared backend/shared
COPY backend/workers/data-worker backend/workers/data-worker
RUN cd backend/workers/data-worker && GOWORK=off go build -o /data-worker .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /data-worker /data-worker
EXPOSE 8080
ENTRYPOINT ["/data-worker"]
