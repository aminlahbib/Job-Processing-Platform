// job-coordinator consumes the jobs.pending queue and routes each job to its
// type-specific queue (jobs.image, jobs.data, jobs.report) so that workers only
// need to know about their own queue.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/jpp/shared"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Prometheus metrics
var jobsRoutedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "jpp_jobs_routed_total",
		Help: "Total number of jobs routed to worker queues",
	},
	[]string{"type", "queue"},
)

func init() {
	prometheus.MustRegister(jobsRoutedTotal)
}

func initTracer() (func(context.Context) error, error) {
	ctx := context.Background()
	endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("job-coordinator"))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// typeToQueue maps each job type to its dedicated worker queue.
// Add a new entry here when adding a new worker type.
var typeToQueue = map[shared.JobType]string{
	shared.JobTypeImage:  "jobs.image",
	shared.JobTypeData:   "jobs.data",
	shared.JobTypeReport: "jobs.report",
}

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "job-coordinator").Logger()

	rabbitmqURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot connect to RabbitMQ")
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot open channel")
	}
	defer ch.Close()

	mustDeclareQueue(logger, ch, "jobs.pending")
	for _, q := range typeToQueue {
		mustDeclareQueue(logger, ch, q)
	}

	// Expose /metrics for Prometheus scraping
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		metricsAddr := ":" + getEnv("METRICS_PORT", "8080")
		logger.Info().Str("addr", metricsAddr).Msg("metrics server listening")
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			logger.Error().Err(err).Msg("metrics server error")
		}
	}()

	if err := ch.Qos(1, 0, false); err != nil {
		logger.Fatal().Err(err).Msg("cannot set QoS")
	}

	msgs, err := ch.Consume("jobs.pending", "coordinator", false, false, false, false, nil)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot start consuming jobs.pending")
	}

	if shutdown, err := initTracer(); err != nil {
		logger.Warn().Err(err).Msg("tracing disabled")
	} else if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	logger.Info().Msg("job-coordinator running")

	for msg := range msgs {
		var job shared.JobMessage
		if err := json.Unmarshal(msg.Body, &job); err != nil {
			logger.Warn().Err(err).Msg("malformed message, discarding")
			msg.Nack(false, false)
			continue
		}

		ctx := context.Background()
		tr := otel.Tracer("job-coordinator")
		ctx, span := tr.Start(ctx, "routeJob")
		span.SetAttributes(attribute.String("job.id", job.JobID), attribute.String("job.type", string(job.Type)))

		dest, ok := typeToQueue[job.Type]
		if !ok {
			span.End()
			logger.Warn().Str("job_id", job.JobID).Str("type", string(job.Type)).Msg("unknown job type, discarding")
			msg.Nack(false, false)
			continue
		}

		err = ch.Publish("", dest, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         msg.Body,
		})
		if err != nil {
			span.End()
			logger.Error().Err(err).Str("job_id", job.JobID).Str("dest", dest).Msg("failed to route job, requeueing")
			msg.Nack(false, true)
			continue
		}

		span.SetAttributes(attribute.String("queue", dest))
		span.End()
		jobsRoutedTotal.WithLabelValues(string(job.Type), dest).Inc()
		logger.Info().Str("job_id", job.JobID).Str("type", string(job.Type)).Str("queue", dest).Msg("routed job")
		msg.Ack(false)
	}
}

func mustDeclareQueue(logger zerolog.Logger, ch *amqp.Channel, name string) {
	if _, err := ch.QueueDeclare(name, true, false, false, false, nil); err != nil {
		logger.Fatal().Err(err).Str("queue", name).Msg("cannot declare queue")
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
