FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY backend/shared backend/shared
COPY backend/workers/report-worker backend/workers/report-worker
RUN cd backend/workers/report-worker && GOWORK=off go build -o /report-worker .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /report-worker /report-worker
EXPOSE 8080
ENTRYPOINT ["/report-worker"]
