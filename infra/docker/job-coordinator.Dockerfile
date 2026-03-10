FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.work go.work
COPY backend/shared backend/shared
COPY backend/job-coordinator backend/job-coordinator
RUN go build -o /job-coordinator ./backend/job-coordinator/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /job-coordinator /job-coordinator
EXPOSE 8080
ENTRYPOINT ["/job-coordinator"]
