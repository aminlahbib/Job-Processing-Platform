// api-service is the HTTP gateway for the Job Processing Platform.
// It accepts job submissions, persists them to Postgres, and enqueues them to RabbitMQ.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jpp/shared"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Config holds all service configuration loaded from environment variables.
// Defaults allow the service to run locally without any setup.
type Config struct {
	Port        string
	DatabaseURL string
	RabbitMQURL string
}

func loadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://jpp:jpp@localhost:5432/jpp?sslmode=disable"),
		RabbitMQURL: getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
	}
}

// Prometheus metrics
var jobsSubmittedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "jpp_jobs_submitted_total",
		Help: "Total number of jobs submitted via the API",
	},
	[]string{"type"},
)

func init() {
	prometheus.MustRegister(jobsSubmittedTotal)
}

// initTracer sets up the OpenTelemetry tracer provider with Jaeger OTLP HTTP exporter.
// Set OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger.observability:4318 for in-cluster.
func initTracer() (func(context.Context) error, error) {
	ctx := context.Background()
	endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("api-service"),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// App holds application-level dependencies shared across HTTP handlers.
type App struct {
	db      *sql.DB
	channel *amqp.Channel
	config  Config
	log     zerolog.Logger
}

func main() {
	//builds logger
	zerolog.TimeFieldFormat = time.RFC3339
	zl := zerolog.New(os.Stdout).With().Timestamp().Logger()
	logger := zl.With().Str("service", "api-service").Logger()

	//loads configuration
	cfg := loadConfig()

	//opens database connection
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to open database connection")
	}
	defer db.Close()

	//pings database to check if it is reachable
	if err := db.Ping(); err != nil {
		logger.Warn().Err(err).Msg("cannot reach database at startup")
	}

	//opens RabbitMQ connection
	var ch *amqp.Channel
	conn, err := amqp.Dial(cfg.RabbitMQURL)
	//checks if RabbitMQ connection is successful
	if err != nil {
		logger.Warn().Err(err).Msg("cannot connect to RabbitMQ at startup")
	} else {
		//closes RabbitMQ connection when app is shutdown
		defer conn.Close()
		ch, err = conn.Channel()
		if err != nil {
			logger.Warn().Err(err).Msg("cannot open RabbitMQ channel")
		} else {
			defer ch.Close()
			if _, err = ch.QueueDeclare("jobs.pending", true, false, false, false, nil); err != nil {
				logger.Warn().Err(err).Msg("cannot declare jobs.pending queue")
			}
		}
	}

	//creates app instance
	app := &App{db: db, channel: ch, config: cfg, log: logger}

	//initializes tracer
	if shutdown, err := initTracer(); err != nil {
		logger.Warn().Err(err).Msg("tracing disabled")
	} else if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	//creates Gin router
	r := gin.Default()
	r.GET("/health", app.healthHandler)
	r.GET("/healthz", app.healthHandler) // K8s liveness probe alias
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.POST("/jobs", app.createJobHandler)
	r.GET("/jobs/:id", app.getJobHandler)
	r.GET("/jobs", app.listJobsHandler)

	//starts server
	logger.Info().Str("port", cfg.Port).Msg("api-service starting")
	if err := r.Run(":" + cfg.Port); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}

// healthHandler returns service health and live dependency status.
// Used as both the K8s liveness probe and a human-readable status endpoint.
func (a *App) healthHandler(c *gin.Context) {
	//builds response
	resp := gin.H{
		"service": "api-service",
		"status":  "ok",
		"time":    time.Now().UTC(),
	}
	//pings database to check if it is reachable
	if err := a.db.Ping(); err != nil {
		resp["database"] = "unreachable"	
	} else {
		resp["database"] = "ok"
	}
	if a.channel == nil {
		resp["rabbitmq"] = "disconnected"
	} else {
		resp["rabbitmq"] = "ok"
	}
	c.JSON(http.StatusOK, resp)
}

// createJobHandler accepts a job submission, persists it, and queues it.
//
// POST /jobs
// Body: {"type":"image|data|report","payload":"<json string>"}
func (a *App) createJobHandler(c *gin.Context) {
	//gets request context
	ctx := c.Request.Context()
	//creates tracer
	tr := otel.Tracer("api-service")
	//starts span
	ctx, span := tr.Start(ctx, "createJob")
	defer span.End()

	//binds request body to struct
	var req struct {
		Type    shared.JobType `json:"type" binding:"required"`
		Payload string         `json:"payload"`
	}
	//checks if request body is valid
	if err := c.ShouldBindJSON(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	//sets span attributes
	span.SetAttributes(attribute.String("job.type", string(req.Type)))

	//checks if job type is valid
	switch req.Type {
	case shared.JobTypeImage, shared.JobTypeData, shared.JobTypeReport:
	default:
		//returns bad request if job type is invalid
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown job type %q", req.Type)})
		return
	}

	//creates job
	now := time.Now().UTC()
	//creates job struct
	job := shared.Job{
		ID:        uuid.New().String(),
		Type:      req.Type,
		Status:    shared.StatusPending,
		Payload:   req.Payload,
		CreatedAt: now,
		UpdatedAt: now,
	}

	//inserts job into database
	_, err := a.db.ExecContext(ctx,
		`INSERT INTO jobs (id, type, status, payload, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		job.ID, job.Type, job.Status, job.Payload, job.CreatedAt, job.UpdatedAt,
	)
	//checks if database insert is successful
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "db insert failed")
		a.log.Error().Err(err).Str("job_id", job.ID).Str("trace_id", span.SpanContext().TraceID().String()).Msg("db insert failed")
		//returns internal server error if database insert is unsuccessful
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist job"})
		return
	}
	//sets span attributes
	span.SetAttributes(attribute.String("job.id", job.ID))

	//checks if RabbitMQ channel is open
	if a.channel != nil {
		//creates job message
		msg := shared.JobMessage{JobID: job.ID, Type: job.Type, Payload: job.Payload}
		body, _ := json.Marshal(msg)
		//publishes job message to RabbitMQ
		if err = a.channel.Publish("", "jobs.pending", false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		}); err != nil {
			a.log.Error().Err(err).Str("job_id", job.ID).Msg("job persisted but not queued")
		}
	}

	//increments jobs submitted total metric
	jobsSubmittedTotal.WithLabelValues(string(job.Type)).Inc()
	//logs job submitted
	a.log.Info().Str("job_id", job.ID).Str("type", string(job.Type)).Str("trace_id", span.SpanContext().TraceID().String()).Msg("job submitted")
	//returns created status with job
	c.JSON(http.StatusCreated, job)
}

// getJobHandler returns a single job by ID.
// GET /jobs/:id
func (a *App) getJobHandler(c *gin.Context) {
	//gets job ID from request
	id := c.Param("id")
	//queries database for job
	var job shared.Job
	err := a.db.QueryRow(
		`SELECT id, type, status, payload, result, error, created_at, updated_at
		 FROM jobs WHERE id = $1`, id,
	).Scan(&job.ID, &job.Type, &job.Status, &job.Payload, &job.Result, &job.Error, &job.CreatedAt, &job.UpdatedAt)

	//checks if job is not found
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	//checks if database query is successful
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	//returns ok status with job
	c.JSON(http.StatusOK, job)
}

// listJobsHandler returns the 100 most recent jobs.
// GET /jobs
func (a *App) listJobsHandler(c *gin.Context) {
	//queries database for jobs
	rows, err := a.db.Query(
		`SELECT id, type, status, payload, result, error, created_at, updated_at
		 FROM jobs ORDER BY created_at DESC LIMIT 100`,
	)
	//checks if database query is successful
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	//closes rows when function returns
	defer rows.Close()

	//creates jobs slice
	jobs := []shared.Job{}
	//iterates over rows
	for rows.Next() {
		//scans row into job struct
		var j shared.Job
		if err := rows.Scan(&j.ID, &j.Type, &j.Status, &j.Payload, &j.Result, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
			a.log.Error().Err(err).Msg("row scan error")
			continue
		}
		jobs = append(jobs, j)
	}
	//returns ok status with jobs and count
	c.JSON(http.StatusOK, gin.H{"jobs": jobs, "count": len(jobs)})
}

// gets environment variable or returns fallback
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
