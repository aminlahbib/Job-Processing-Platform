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

const queueName = "jobs.data"
const workerType = "data"

// Prometheus metrics
var (
	jobsCompletedTotal = prometheus.NewCounterVec(
		//prometheus counter vector for jobs completed total
		prometheus.CounterOpts{Name: "jpp_jobs_completed_total", Help: "Total jobs completed by worker"},
		[]string{"worker"},
	)
	//prometheus counter vector for jobs failed total
	jobsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "jpp_jobs_failed_total", Help: "Total jobs failed by worker"},
		[]string{"worker"},
	)
	//prometheus histogram vector for job processing duration
	jobProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "jpp_job_processing_duration_seconds",
			Help:    "Job processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"worker"},
	)
)

// registers prometheus metrics
func init() {
	prometheus.MustRegister(jobsCompletedTotal, jobsFailedTotal, jobProcessingDuration)
}

// initTracer sets up the OpenTelemetry tracer provider with Jaeger OTLP HTTP exporter.
func initTracer() (func(context.Context) error, error) {
	//creates new context
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
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("data-worker"))),
	)
	//sets tracer provider
	otel.SetTracerProvider(tp)
	//returns shutdown function and error
	return tp.Shutdown, nil
}

func main() {
	//builds logger
	zerolog.TimeFieldFormat = time.RFC3339
	//creates new logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "data-worker").Logger()

	//gets RabbitMQ URL from environment variable
	rabbitmqURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	//gets database URL from environment variable
	dbURL := getEnv("DATABASE_URL", "postgres://jpp:jpp@localhost:5432/jpp?sslmode=disable")

	//opens database connection
	db, err := sql.Open("postgres", dbURL)
	//checks if database connection is successful
	if err != nil {
		logger.Fatal().Err(err).Msg("cannot open database")
	}
	defer db.Close()

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

	//declares data queue
	if _, err = ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		//logs error if data queue cannot be declared
		logger.Fatal().Err(err).Msg("cannot declare queue")
	}

	//sets QoS for RabbitMQ channel
	if err = ch.Qos(1, 0, false); err != nil {
		logger.Fatal().Err(err).Msg("cannot set QoS")
	}

	//handles metrics for Prometheus scraping
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		//gets metrics address from environment variable
		addr := ":" + getEnv("METRICS_PORT", "8080")
		//logs metrics server listening
		logger.Info().Str("addr", addr).Msg("metrics server listening")
		//starts metrics server
		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Error().Err(err).Msg("metrics server error")
		}
	}()

	//consumes data queue
	msgs, err := ch.Consume(queueName, "data-worker", false, false, false, false, nil)
	if err != nil {
		//logs error if data queue cannot be consumed
		logger.Fatal().Err(err).Msg("cannot start consuming")
	}

	//initializes tracer
	if shutdown, err := initTracer(); err != nil {
		//logs error if tracer cannot be initialized
		logger.Warn().Err(err).Msg("tracing disabled")
	} else if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	//logs data-worker running
	logger.Info().Str("queue", queueName).Msg("data-worker running")

	//iterates over messages from data queue
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
		tr := otel.Tracer("data-worker")
		ctx, span := tr.Start(ctx, "processDataJob")
		//sets span attributes
		span.SetAttributes(attribute.String("job.id", job.JobID))

		//starts timer for job processing duration
		start := time.Now()
		//logs starting data job
		logger.Info().Str("job_id", job.JobID).Msg("starting data job")
		//updates job status to processing
		updateStatus(logger, db, job.JobID, shared.StatusProcessing, nil, nil)

		//processes data job
		result, err := processDataJob(job)
		jobProcessingDuration.WithLabelValues(workerType).Observe(time.Since(start).Seconds())

		//checks if data job failed
		if err != nil {
			//ends span
			span.End()
			//increments jobs failed total metric
			jobsFailedTotal.WithLabelValues(workerType).Inc()
			//logs data job failed
			logger.Error().Err(err).Str("job_id", job.JobID).Msg("data job failed")
			//gets error string
			errStr := err.Error()
			updateStatus(logger, db, job.JobID, shared.StatusFailed, nil, &errStr)
			msg.Ack(false)
			continue
		}

		span.End()
		//increments jobs completed total metric
		jobsCompletedTotal.WithLabelValues(workerType).Inc()
		//updates job status to completed
		updateStatus(logger, db, job.JobID, shared.StatusCompleted, &result, nil)
		//logs data job completed
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

// updateStatus writes the job result and new status back to Postgres.
func updateStatus(logger zerolog.Logger, db *sql.DB, jobID string, status shared.JobStatus, result, errMsg *string) {
	//executes update job status query
	_, err := db.Exec(
		`UPDATE jobs SET status=$1, result=$2, error=$3, updated_at=$4 WHERE id=$5`,
		status, result, errMsg, time.Now().UTC(), jobID,
	)
	//checks if update job status query is successful
	if err != nil {
		//logs error if update job status query is unsuccessful
		logger.Error().Err(err).Str("job_id", jobID).Msg("failed to update status")
	}
}

// gets environment variable or returns fallback
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
