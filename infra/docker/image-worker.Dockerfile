FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY backend/shared backend/shared
COPY backend/workers/image-worker backend/workers/image-worker
RUN cd backend/workers/image-worker && GOWORK=off go build -o /image-worker .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /image-worker /image-worker
EXPOSE 8080
ENTRYPOINT ["/image-worker"]
