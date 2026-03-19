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
	//prometheus counter vector for jobs routed total
	prometheus.CounterOpts{
		Name: "jpp_jobs_routed_total",
		Help: "Total number of jobs routed to worker queues",
	},
	[]string{"type", "queue"},
)

// registers jobs routed total metric
func init() {
	prometheus.MustRegister(jobsRoutedTotal)
}

// initTracer sets up the OpenTelemetry tracer provider with Jaeger OTLP HTTP exporter.
func initTracer() (func(context.Context) error, error) {
	ctx := context.Background()
	//gets endpoint from environment variable
	endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	//creates new OTLP HTTP exporter
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}
	//creates new tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("job-coordinator"))),
	)
	//sets tracer provider
	otel.SetTracerProvider(tp)
	//returns shutdown function and error
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
	//builds logger
	zerolog.TimeFieldFormat = time.RFC3339
	//creates new logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "job-coordinator").Logger()

	//gets RabbitMQ URL from environment variable
	rabbitmqURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	//opens RabbitMQ connection
	conn, err := amqp.Dial(rabbitmqURL)
	//checks if RabbitMQ connection is successful
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot connect to RabbitMQ")
	}

	defer conn.Close()

	//opens RabbitMQ channel
	ch, err := conn.Channel()
	//checks if RabbitMQ channel is successful
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot open channel")
	}
	defer ch.Close()

	//declares jobs.pending queue
	mustDeclareQueue(logger, ch, "jobs.pending")
	//declares worker queues
	for _, q := range typeToQueue {
		mustDeclareQueue(logger, ch, q)
	}

	// Expose /metrics for Prometheus scraping
	go func() {
		//handles metrics for Prometheus scraping
		http.Handle("/metrics", promhttp.Handler())
		//gets metrics address from environment variable
		metricsAddr := ":" + getEnv("METRICS_PORT", "8080")
		//logs metrics server listening
		logger.Info().Str("addr", metricsAddr).Msg("metrics server listening")
		//starts metrics server
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			logger.Error().Err(err).Msg("metrics server error")
		}
	}()

	//sets QoS for RabbitMQ channel
	if err := ch.Qos(1, 0, false); err != nil {
		logger.Fatal().Err(err).Msg("cannot set QoS")
	}

	//consumes jobs.pending queue
	msgs, err := ch.Consume("jobs.pending", "coordinator", false, false, false, false, nil)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot start consuming jobs.pending")
	}

	//initializes tracer
	if shutdown, err := initTracer(); err != nil {
		logger.Warn().Err(err).Msg("tracing disabled")
	} else if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	//logs job-coordinator running
	logger.Info().Msg("job-coordinator running")

	//iterates over messages from jobs.pending queue
	for msg := range msgs {
		//unmarshals message body into job message struct
		var job shared.JobMessage
		if err := json.Unmarshal(msg.Body, &job); err != nil {
			logger.Warn().Err(err).Msg("malformed message, discarding")
			msg.Nack(false, false)
			continue
		}

		//creates new context
		ctx := context.Background()
		//creates new tracer
		tr := otel.Tracer("job-coordinator")
		ctx, span := tr.Start(ctx, "routeJob")
		span.SetAttributes(attribute.String("job.id", job.JobID), attribute.String("job.type", string(job.Type)))

		//gets destination queue from typeToQueue map
		dest, ok := typeToQueue[job.Type]
		//checks if destination queue is valid
		if !ok {
			span.End()
			logger.Warn().Str("job_id", job.JobID).Str("type", string(job.Type)).Msg("unknown job type, discarding")
			msg.Nack(false, false)
			continue
		}

		//publishes job message to destination queue
		err = ch.Publish("", dest, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         msg.Body,
		})
		//checks if job message is published to destination queue
		if err != nil {
			//ends span
			span.End()
			logger.Error().Err(err).Str("job_id", job.JobID).Str("dest", dest).Msg("failed to route job, requeueing")
			msg.Nack(false, true)
			continue
		}

		//sets span attributes
		span.SetAttributes(attribute.String("queue", dest))
		span.End()
		jobsRoutedTotal.WithLabelValues(string(job.Type), dest).Inc()
		logger.Info().Str("job_id", job.JobID).Str("type", string(job.Type)).Str("queue", dest).Msg("routed job")
		msg.Ack(false)
	}
}

// mustDeclareQueue declares a queue if it does not already exist.
func mustDeclareQueue(logger zerolog.Logger, ch *amqp.Channel, name string) {
	if _, err := ch.QueueDeclare(name, true, false, false, false, nil); err != nil {
		logger.Fatal().Err(err).Str("queue", name).Msg("cannot declare queue")
	}
}

// gets environment variable or returns fallback
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
