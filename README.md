# Backend

Go backend cho sync layer cua PQQ.

## Stack

- Gin
- PostgreSQL
- pgx
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

4. Chay server dev voi `air`:

```bash
air
```

Neu muon chay tay khong hot reload:

```bash
go run ./cmd/server
```

## Ghi chu

- Domain tables:
  - `clubs`
  - `belt_ranks`
  - `students`
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
