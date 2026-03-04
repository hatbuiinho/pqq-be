# Backend

Go backend cho sync layer cua PQQ.

## Stack

- Gin
- PostgreSQL
- pgx
- sqlc-ready typed query layer
- MinIO object storage
- Gorilla WebSocket

## Endpoints

- `GET /health`
- `POST /api/v1/sync/push`
- `GET /api/v1/sync/pull`
- `GET /api/v1/sync/ws`
- `POST /api/v1/belt-ranks/import`

## Chay local

1. Tao PostgreSQL database `pqq`
2. Tao file `.env` tu `.env.example` va dien gia tri thuc te
3. Cai dependencies:

```bash
go mod tidy
```

4. Chay server dev:

```bash
make dev
```

Neu muon chay tay khong hot reload:

```bash
go run ./cmd/server
```

## SQL access strategy

- Write path cua sync van dung raw SQL trong `internal/postgres/store.go`
  - ly do: logic canonical sync, upsert, change log va conflict handling dang rat custom
- Read path on dinh da duoc dua sang typed query package o `internal/postgres/db`
  - package nay duoc to chuc theo shape `sqlc` generate
  - source of truth cho schema/query nam o:
    - `sqlc.yaml`
    - `internal/postgres/sqlc/schema.sql`
    - `internal/postgres/sqlc/queries.sql`

Neu may da cai `sqlc`, workflow la:

```bash
make sqlc
```

Hien tai repo da commit san package typed query de khong phu thuoc vao viec cai `sqlc` moi build duoc.

## Make targets

```bash
make sqlc   # generate sqlc code + gofmt + compile test
make dev    # run backend with air
make test   # run go test with local GOCACHE
make fmt    # gofmt cmd and internal packages
```

## MinIO storage

Backend da co storage service cho MinIO o:

- [internal/storage/minio.go](/Users/hatbuinho/Documents/TTPQ/PQQ/be/internal/storage/minio.go)

Env can dung:

```bash
MINIO_ENABLED=true
MINIO_ENDPOINT=minio-api.hatbuinho.me
MINIO_ACCESS_KEY=...
MINIO_SECRET_KEY=...
MINIO_BUCKET=pqq-media
MINIO_REGION=us-east-1
MINIO_USE_SSL=true
MINIO_PUBLIC_BASE_URL=
MINIO_PRESIGN_EXPIRY_MINUTES=15
```

Khi `MINIO_ENABLED=true`, server se:
- khoi tao MinIO client
- kiem tra bucket ton tai
- tu tao bucket neu chua co

Storage service da ho tro:
- upload object
- presign upload url
- presign download url
- delete object

## Ghi chu

- Domain tables:
  - `clubs`
  - `club_groups`
  - `club_schedules`
  - `belt_ranks`
  - `students`
  - `student_schedule_profiles`
  - `student_schedules`
  - `attendance_sessions`
  - `attendance_records`
- Sync support tables:
  - `sync_processed_mutations`
  - `sync_counters`
  - `sync_change_log`
- `studentCode` duoc generate o backend theo format `PQQ-000001`
- WebSocket chi dung de thong bao `sync.changed`, client van phai `pull` de lay canonical data
- File config `air` nam o `.air.toml`
- Server tu doc `.env`; neu shell da co env cung key thi env trong shell se duoc uu tien
- Import Excel belt ranks:
  - upload multipart field `file`
  - doc worksheet dau tien
  - header bat buoc: `name`, `order`
  - header tuy chon: `description`, `isActive`
