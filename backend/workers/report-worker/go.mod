module github.com/jpp/report-worker

go 1.23

require (
	github.com/jpp/shared v0.0.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.19.1
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/rs/zerolog v1.33.0
	go.opentelemetry.io/otel v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.28.0
	go.opentelemetry.io/otel/sdk v1.28.0
	go.opentelemetry.io/otel/trace v1.28.0
)

replace github.com/jpp/shared => ../../shared
