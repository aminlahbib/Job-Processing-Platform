// data-worker consumes jobs from the jobs.data queue and performs data transformation.
// Currently a stub — replace processDataJob() with real transformation logic.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jpp/shared"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const queueName = "jobs.data"
const workerType = "data"

var (
	jobsCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "jpp_jobs_completed_total", Help: "Total jobs completed by worker"},
		[]string{"worker"},
	)
	jobsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "jpp_jobs_failed_total", Help: "Total jobs failed by worker"},
		[]string{"worker"},
	)
	jobProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "jpp_job_processing_duration_seconds",
			Help:    "Job processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"worker"},
	)
)

func init() {
	prometheus.MustRegister(jobsCompletedTotal, jobsFailedTotal, jobProcessingDuration)
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
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("data-worker"))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "data-worker").Logger()

	rabbitmqURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	dbURL := getEnv("DATABASE_URL", "postgres://jpp:jpp@localhost:5432/jpp?sslmode=disable")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot open database")
	}
	defer db.Close()

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

	if _, err = ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		logger.Fatal().Err(err).Msg("cannot declare queue")
	}

	if err = ch.Qos(1, 0, false); err != nil {
		logger.Fatal().Err(err).Msg("cannot set QoS")
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		addr := ":" + getEnv("METRICS_PORT", "8080")
		logger.Info().Str("addr", addr).Msg("metrics server listening")
		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Error().Err(err).Msg("metrics server error")
		}
	}()

	msgs, err := ch.Consume(queueName, "data-worker", false, false, false, false, nil)
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot start consuming")
	}

	if shutdown, err := initTracer(); err != nil {
		logger.Warn().Err(err).Msg("tracing disabled")
	} else if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	logger.Info().Str("queue", queueName).Msg("data-worker running")

	for msg := range msgs {
		var job shared.JobMessage
		if err := json.Unmarshal(msg.Body, &job); err != nil {
			logger.Warn().Err(err).Msg("malformed message, discarding")
			msg.Nack(false, false)
			continue
		}

		ctx := context.Background()
		tr := otel.Tracer("data-worker")
		ctx, span := tr.Start(ctx, "processDataJob")
		span.SetAttributes(attribute.String("job.id", job.JobID))

		start := time.Now()
		logger.Info().Str("job_id", job.JobID).Msg("starting data job")
		updateStatus(logger, db, job.JobID, shared.StatusProcessing, nil, nil)

		result, err := processDataJob(job)
		jobProcessingDuration.WithLabelValues(workerType).Observe(time.Since(start).Seconds())

		if err != nil {
			span.End()
			jobsFailedTotal.WithLabelValues(workerType).Inc()
			logger.Error().Err(err).Str("job_id", job.JobID).Msg("data job failed")
			errStr := err.Error()
			updateStatus(logger, db, job.JobID, shared.StatusFailed, nil, &errStr)
			msg.Ack(false)
			continue
		}

		span.End()
		jobsCompletedTotal.WithLabelValues(workerType).Inc()
		updateStatus(logger, db, job.JobID, shared.StatusCompleted, &result, nil)
		logger.Info().Str("job_id", job.JobID).Msg("data job completed")
		msg.Ack(false)
	}
}

// processDataJob contains the data transformation logic.
// TODO (Week 3): implement real data processing — CSV parsing, JSON transformation, aggregation, etc.
func processDataJob(job shared.JobMessage) (string, error) {
	time.Sleep(200 * time.Millisecond)
	return fmt.Sprintf(`{"processed":true,"job_id":%q,"worker":"data"}`, job.JobID), nil
}

func updateStatus(logger zerolog.Logger, db *sql.DB, jobID string, status shared.JobStatus, result, errMsg *string) {
	_, err := db.Exec(
		`UPDATE jobs SET status=$1, result=$2, error=$3, updated_at=$4 WHERE id=$5`,
		status, result, errMsg, time.Now().UTC(), jobID,
	)
	if err != nil {
		logger.Error().Err(err).Str("job_id", jobID).Msg("failed to update status")
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
