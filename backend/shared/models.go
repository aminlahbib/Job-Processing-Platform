// Package shared contains domain types used by all services.
// All services import this package; it must never import any service package.
package shared

import "time"

// JobStatus represents the lifecycle state of a job.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

// JobType determines which worker handles a job.
type JobType string

const (
	JobTypeImage  JobType = "image"
	JobTypeData   JobType = "data"
	JobTypeReport JobType = "report"
)

// Job is the core domain object — persisted in Postgres and routed via RabbitMQ.
// Result and Error are pointers because they are NULL in the database until the job finishes.
type Job struct {
	ID        string    `json:"id"`
	Type      JobType   `json:"type"`
	Status    JobStatus `json:"status"`
	Payload   string    `json:"payload"`
	Result    *string   `json:"result,omitempty"`
	Error     *string   `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JobMessage is the payload published to RabbitMQ when a job is queued.
// It is a subset of Job — workers only need what they need to execute.
type JobMessage struct {
	JobID   string  `json:"job_id"`
	Type    JobType `json:"type"`
	Payload string  `json:"payload"`
}

// WorkerResult is returned by a worker after job execution completes or fails.
type WorkerResult struct {
	JobID      string    `json:"job_id"`
	Status     JobStatus `json:"status"`
	Result     string    `json:"result,omitempty"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}
