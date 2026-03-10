FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.work go.work
COPY backend/shared backend/shared
COPY backend/workers/image-worker backend/workers/image-worker
RUN go build -o /image-worker ./backend/workers/image-worker/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /image-worker /image-worker
EXPOSE 8080
ENTRYPOINT ["/image-worker"]
