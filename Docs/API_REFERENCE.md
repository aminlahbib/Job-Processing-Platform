# API Reference

Base URL: `http://localhost:8080` (local) or `https://api.jpp.example.com` (production)

## Health Check

### `GET /health`

Returns service health and dependency status.

**Response 200**
```json
{
  "service": "api-service",
  "status": "ok",
  "database": "ok",
  "time": "2026-03-03T10:00:00Z"
}
```

---

## Jobs

### `POST /jobs`

Submit a new job for async processing.

**Request Body**
```json
{
  "type": "image",
  "payload": "{\"url\": \"https://example.com/photo.jpg\", \"width\": 800}"
}
```

| Field | Type | Required | Values |
|-------|------|----------|--------|
| `type` | string | yes | `image`, `data`, `report` |
| `payload` | string (JSON) | no | Any JSON string |

**Response 201**
```json
{
  "id": "3f7d1a2b-...",
  "type": "image",
  "status": "pending",
  "payload": "{...}",
  "created_at": "2026-03-03T10:00:00Z",
  "updated_at": "2026-03-03T10:00:00Z"
}
```

---

### `GET /jobs/:id`

Get the current status and result of a job.

**Response 200**
```json
{
  "id": "3f7d1a2b-...",
  "type": "image",
  "status": "completed",
  "payload": "{...}",
  "result": "{\"processed\":true}",
  "created_at": "2026-03-03T10:00:00Z",
  "updated_at": "2026-03-03T10:00:05Z"
}
```

**Job Status Values**
| Status | Meaning |
|--------|---------|
| `pending` | Queued, not yet picked up |
| `processing` | Worker is executing |
| `completed` | Finished successfully, see `result` |
| `failed` | Execution failed, see `error` |

**Response 404**
```json
{"error": "job not found"}
```

---

### `GET /jobs`

List recent jobs (most recent first, max 100).

**Response 200**
```json
{
  "jobs": [...],
  "count": 5
}
```

---

## Error Format

All errors return:
```json
{"error": "human readable message"}
```
