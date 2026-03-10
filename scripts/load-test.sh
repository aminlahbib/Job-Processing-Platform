#!/usr/bin/env bash
# load-test.sh — submit a burst of jobs and measure throughput
# TODO (Week 8): implement real load testing with hey or k6
# Usage: ./scripts/load-test.sh [job_count] [concurrency]
# Example: ./scripts/load-test.sh 1000 10

set -euo pipefail

JOB_COUNT=${1:-100}
CONCURRENCY=${2:-5}
API_URL=${API_URL:-"http://localhost:8080"}
JOB_TYPES=("image" "data" "report")

echo "Load test: $JOB_COUNT jobs, concurrency $CONCURRENCY → $API_URL"

submit_job() {
  local type="${JOB_TYPES[$((RANDOM % 3))]}"
  curl -s -X POST "$API_URL/jobs" \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"$type\",\"payload\":\"{\\\"run\\\":$RANDOM}\"}" \
    | grep -o '"id":"[^"]*"' | head -1
}

export -f submit_job
export API_URL JOB_TYPES

START=$(date +%s)

# Use xargs for simple concurrency (replace with hey/k6 for proper load testing)
seq 1 "$JOB_COUNT" | xargs -P "$CONCURRENCY" -I{} bash -c 'submit_job'

END=$(date +%s)
ELAPSED=$((END - START))

echo ""
echo "Submitted $JOB_COUNT jobs in ${ELAPSED}s"
echo "Throughput: $((JOB_COUNT / ELAPSED)) jobs/s"
echo ""
echo "Check queue in RabbitMQ: make rabbitmq-port-forward"
echo "Check job status:        curl $API_URL/jobs"
