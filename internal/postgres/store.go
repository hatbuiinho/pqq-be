package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pqq/be/internal/sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SyncStore struct {
	pool *pgxpool.Pool
}

func NewSyncStore(pool *pgxpool.Pool) *SyncStore {
	return &SyncStore{pool: pool}
}

func (s *SyncStore) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}

func (s *SyncStore) GetRecordForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	entityName sync.EntityName,
	recordID string,
) (*sync.StoredRecord, error) {
	switch entityName {
	case sync.EntityClubs:
		return getClubForUpdate(ctx, tx, recordID)
	case sync.EntityBeltRanks:
		return getBeltRankForUpdate(ctx, tx, recordID)
	case sync.EntityStudents:
		return getStudentForUpdate(ctx, tx, recordID)
	default:
		return nil, fmt.Errorf("unsupported entityName %q", entityName)
	}
}

func (s *SyncStore) UpsertRecord(ctx context.Context, tx pgx.Tx, record sync.StoredRecord) error {
	switch record.EntityName {
	case sync.EntityClubs:
		if err := upsertClub(ctx, tx, record); err != nil {
			return err
		}
	case sync.EntityBeltRanks:
		if err := upsertBeltRank(ctx, tx, record); err != nil {
			return err
		}
	case sync.EntityStudents:
		if err := upsertStudent(ctx, tx, record); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported entityName %q", record.EntityName)
	}

	return insertChangeLog(ctx, tx, record)
}

func (s *SyncStore) ListChangesSince(ctx context.Context, since string, limit int) ([]sync.StoredRecord, error) {
	sinceTime, sinceChangeID, err := parseSyncCursor(since)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT change_id, entity_name, record_id, payload, server_modified_at
		FROM sync_change_log
		WHERE (
			$1::timestamptz IS NULL
			OR server_modified_at > $1
			OR (server_modified_at = $1 AND change_id > $2)
		)
		ORDER BY server_modified_at ASC, change_id ASC
		LIMIT $3
	`

	rows, err := s.pool.Query(ctx, query, sinceTime, sinceChangeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.StoredRecord, 0, limit)
	for rows.Next() {
		var record sync.StoredRecord
		var serverModifiedAt time.Time
		if err := rows.Scan(&record.ChangeID, &record.EntityName, &record.RecordID, &record.Payload, &serverModifiedAt); err != nil {
			return nil, err
		}
		record.ServerModifiedAt = serverModifiedAt.UTC().Format(time.RFC3339Nano)
		records = append(records, record)
	}

	return records, rows.Err()
}

func (s *SyncStore) IsMutationProcessed(ctx context.Context, tx pgx.Tx, deviceID string, mutationID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM sync_processed_mutations
			WHERE device_id = $1 AND mutation_id = $2
		)
	`

	var exists bool
	if err := tx.QueryRow(ctx, query, deviceID, mutationID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *SyncStore) SaveProcessedMutation(
	ctx context.Context,
	tx pgx.Tx,
	deviceID string,
	mutation sync.SyncMutation,
	serverModifiedAt string,
) error {
	query := `
		INSERT INTO sync_processed_mutations (
			device_id,
			mutation_id,
			entity_name,
			record_id,
			client_modified_at,
			server_modified_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (device_id, mutation_id) DO NOTHING
	`

	clientModifiedAt, err := time.Parse(time.RFC3339Nano, mutation.ClientModifiedAt)
	if err != nil {
		return err
	}
	serverModifiedAtValue, err := time.Parse(time.RFC3339Nano, serverModifiedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		query,
		deviceID,
		mutation.MutationID,
		mutation.EntityName,
		mutation.RecordID,
		clientModifiedAt,
		serverModifiedAtValue,
	)
	return err
}

func (s *SyncStore) FindClubByCode(
	ctx context.Context,
	tx pgx.Tx,
	clubCode string,
	excludeID string,
) (*sync.StoredRecord, error) {
	query := `
		SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM clubs
		WHERE deleted_at IS NULL
			AND id <> $1
			AND code = $2
		LIMIT 1
	`

	var record sync.ClubRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, excludeID, clubCode).Scan(
		&record.ID,
		&record.Code,
		&record.Name,
		&record.Phone,
		&record.Email,
		&record.Address,
		&record.Notes,
		&record.IsActive,
		&createdAt,
		&updatedAt,
		&lastModifiedAt,
		&deletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
	record.SyncStatus = "synced"
	if deletedAt != nil {
		value := deletedAt.UTC().Format(time.RFC3339Nano)
		record.DeletedAt = &value
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		EntityName:       sync.EntityClubs,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func (s *SyncStore) FindBeltRankByOrder(
	ctx context.Context,
	tx pgx.Tx,
	order int,
	excludeID string,
) (*sync.StoredRecord, error) {
	query := `
		SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM belt_ranks
		WHERE deleted_at IS NULL
			AND id <> $1
			AND rank_order = $2
		LIMIT 1
	`

	var record sync.BeltRankRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, excludeID, order).Scan(
		&record.ID,
		&record.Name,
		&record.Order,
		&record.Description,
		&record.IsActive,
		&createdAt,
		&updatedAt,
		&lastModifiedAt,
		&deletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
	record.SyncStatus = "synced"
	if deletedAt != nil {
		value := deletedAt.UTC().Format(time.RFC3339Nano)
		record.DeletedAt = &value
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		EntityName:       sync.EntityBeltRanks,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func (s *SyncStore) FindStudentByCode(
	ctx context.Context,
	tx pgx.Tx,
	studentCode string,
	excludeID string,
) (*sync.StoredRecord, error) {
	query := `
		SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
		FROM students
		WHERE deleted_at IS NULL
			AND id <> $1
			AND student_code = $2
		LIMIT 1
	`

	record, err := scanStudentRecord(ctx, tx, query, excludeID, studentCode)
	if err != nil || record == nil {
		return record, err
	}

	record.EntityName = sync.EntityStudents
	return record, nil
}

func (s *SyncStore) RecordExists(ctx context.Context, tx pgx.Tx, entityName sync.EntityName, recordID string) (bool, error) {
	var query string
	switch entityName {
	case sync.EntityClubs:
		query = `SELECT EXISTS(SELECT 1 FROM clubs WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityBeltRanks:
		query = `SELECT EXISTS(SELECT 1 FROM belt_ranks WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityStudents:
		query = `SELECT EXISTS(SELECT 1 FROM students WHERE id = $1 AND deleted_at IS NULL)`
	default:
		return false, fmt.Errorf("unsupported entityName %q", entityName)
	}

	var exists bool
	if err := tx.QueryRow(ctx, query, recordID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *SyncStore) NextStudentCode(ctx context.Context, tx pgx.Tx) (string, error) {
	query := `
		INSERT INTO sync_counters (scope, last_value)
		VALUES ('student_code', 1)
		ON CONFLICT (scope) DO UPDATE
		SET last_value = sync_counters.last_value + 1
		RETURNING last_value
	`

	var value int64
	if err := tx.QueryRow(ctx, query).Scan(&value); err != nil {
		return "", err
	}

	return fmt.Sprintf("PQQ-%06d", value), nil
}

func (s *SyncStore) ListAllCurrent(ctx context.Context) ([]sync.ClubRecord, []sync.BeltRankRecord, []sync.StudentRecord, error) {
	clubs, err := listAllClubs(ctx, s.pool)
	if err != nil {
		return nil, nil, nil, err
	}

	beltRanks, err := listAllBeltRanks(ctx, s.pool)
	if err != nil {
		return nil, nil, nil, err
	}

	students, err := listAllStudents(ctx, s.pool)
	if err != nil {
		return nil, nil, nil, err
	}

	return clubs, beltRanks, students, nil
}

func getClubForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM clubs
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.ClubRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.Code,
		&record.Name,
		&record.Phone,
		&record.Email,
		&record.Address,
		&record.Notes,
		&record.IsActive,
		&createdAt,
		&updatedAt,
		&lastModifiedAt,
		&deletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
	record.SyncStatus = "synced"
	if deletedAt != nil {
		value := deletedAt.UTC().Format(time.RFC3339Nano)
		record.DeletedAt = &value
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		EntityName:       sync.EntityClubs,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getBeltRankForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM belt_ranks
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.BeltRankRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.Name,
		&record.Order,
		&record.Description,
		&record.IsActive,
		&createdAt,
		&updatedAt,
		&lastModifiedAt,
		&deletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
	record.SyncStatus = "synced"
	if deletedAt != nil {
		value := deletedAt.UTC().Format(time.RFC3339Nano)
		record.DeletedAt = &value
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		EntityName:       sync.EntityBeltRanks,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getStudentForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
		FROM students
		WHERE id = $1
		FOR UPDATE
	`
	record, err := scanStudentRecord(ctx, tx, query, recordID)
	if err != nil || record == nil {
		return record, err
	}
	record.EntityName = sync.EntityStudents
	return record, nil
}

func scanStudentRecord(ctx context.Context, tx pgx.Tx, query string, args ...any) (*sync.StoredRecord, error) {
	var record sync.StudentRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	var dateOfBirth *time.Time
	var joinedAt *time.Time

	if err := tx.QueryRow(ctx, query, args...).Scan(
		&record.ID,
		&record.StudentCode,
		&record.FullName,
		&dateOfBirth,
		&record.Gender,
		&record.Phone,
		&record.Email,
		&record.Address,
		&record.ClubID,
		&record.BeltRankID,
		&joinedAt,
		&record.Status,
		&record.Notes,
		&createdAt,
		&updatedAt,
		&lastModifiedAt,
		&deletedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
	record.SyncStatus = "synced"
	if deletedAt != nil {
		value := deletedAt.UTC().Format(time.RFC3339Nano)
		record.DeletedAt = &value
	}
	if dateOfBirth != nil {
		value := dateOfBirth.UTC().Format("2006-01-02")
		record.DateOfBirth = &value
	}
	if joinedAt != nil {
		value := joinedAt.UTC().Format("2006-01-02")
		record.JoinedAt = &value
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func upsertClub(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.ClubRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO clubs (
			id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
		ON CONFLICT (id) DO UPDATE
		SET code = EXCLUDED.code,
			name = EXCLUDED.name,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			address = EXCLUDED.address,
			notes = EXCLUDED.notes,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		query,
		record.ID,
		record.Code,
		record.Name,
		record.Phone,
		record.Email,
		record.Address,
		record.Notes,
		record.IsActive,
		createdAt,
		updatedAt,
		lastModifiedAt,
		deletedAt,
	)
	return err
}

func upsertBeltRank(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.BeltRankRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO belt_ranks (
			id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			rank_order = EXCLUDED.rank_order,
			description = EXCLUDED.description,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		query,
		record.ID,
		record.Name,
		record.Order,
		record.Description,
		record.IsActive,
		createdAt,
		updatedAt,
		lastModifiedAt,
		deletedAt,
	)
	return err
}

func upsertStudent(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.StudentRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO students (
			id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)
		ON CONFLICT (id) DO UPDATE
		SET student_code = EXCLUDED.student_code,
			full_name = EXCLUDED.full_name,
			date_of_birth = EXCLUDED.date_of_birth,
			gender = EXCLUDED.gender,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			address = EXCLUDED.address,
			club_id = EXCLUDED.club_id,
			belt_rank_id = EXCLUDED.belt_rank_id,
			joined_at = EXCLUDED.joined_at,
			status = EXCLUDED.status,
			notes = EXCLUDED.notes,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}
	dateOfBirth, err := parseOptionalDate(record.DateOfBirth)
	if err != nil {
		return err
	}
	joinedAt, err := parseOptionalDate(record.JoinedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		query,
		record.ID,
		record.StudentCode,
		record.FullName,
		dateOfBirth,
		record.Gender,
		record.Phone,
		record.Email,
		record.Address,
		record.ClubID,
		record.BeltRankID,
		joinedAt,
		record.Status,
		record.Notes,
		createdAt,
		updatedAt,
		lastModifiedAt,
		deletedAt,
	)
	return err
}

func insertChangeLog(ctx context.Context, tx pgx.Tx, record sync.StoredRecord) error {
	query := `
		INSERT INTO sync_change_log (entity_name, record_id, payload, server_modified_at)
		VALUES ($1, $2, $3, $4)
	`

	serverModifiedAt, err := time.Parse(time.RFC3339Nano, record.ServerModifiedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, query, record.EntityName, record.RecordID, record.Payload, serverModifiedAt)
	return err
}

func parseSyncCursor(cursor string) (*time.Time, int64, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil, 0, nil
	}

	if !strings.Contains(cursor, "#") {
		parsedTime, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			return nil, 0, err
		}
		return &parsedTime, 0, nil
	}

	timePart, idPart, ok := strings.Cut(cursor, "#")
	if !ok {
		return nil, 0, fmt.Errorf("invalid sync cursor")
	}

	parsedTime, err := time.Parse(time.RFC3339Nano, timePart)
	if err != nil {
		return nil, 0, err
	}

	changeID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return nil, 0, err
	}

	return &parsedTime, changeID, nil
}

func parseAuditTimes(createdAtText, updatedAtText, lastModifiedAtText string, deletedAtText *string) (time.Time, time.Time, time.Time, *time.Time, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtText)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, nil, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtText)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, nil, err
	}
	lastModifiedAt, err := time.Parse(time.RFC3339Nano, lastModifiedAtText)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, nil, err
	}

	var deletedAt *time.Time
	if deletedAtText != nil {
		value, err := time.Parse(time.RFC3339Nano, *deletedAtText)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, nil, err
		}
		deletedAt = &value
	}

	return createdAt, updatedAt, lastModifiedAt, deletedAt, nil
}

func parseOptionalDate(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}

	parsed, err := time.Parse("2006-01-02", *value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func listAllClubs(ctx context.Context, pool *pgxpool.Pool) ([]sync.ClubRecord, error) {
	query := `
		SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at
		FROM clubs
		WHERE deleted_at IS NULL
		ORDER BY name ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.ClubRecord, 0)
	for rows.Next() {
		var record sync.ClubRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		if err := rows.Scan(
			&record.ID,
			&record.Code,
			&record.Name,
			&record.Phone,
			&record.Email,
			&record.Address,
			&record.Notes,
			&record.IsActive,
			&createdAt,
			&updatedAt,
			&lastModifiedAt,
		); err != nil {
			return nil, err
		}

		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
		record.SyncStatus = "synced"
		records = append(records, record)
	}

	return records, rows.Err()
}

func listAllBeltRanks(ctx context.Context, pool *pgxpool.Pool) ([]sync.BeltRankRecord, error) {
	query := `
		SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at
		FROM belt_ranks
		WHERE deleted_at IS NULL
		ORDER BY rank_order ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.BeltRankRecord, 0)
	for rows.Next() {
		var record sync.BeltRankRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		if err := rows.Scan(
			&record.ID,
			&record.Name,
			&record.Order,
			&record.Description,
			&record.IsActive,
			&createdAt,
			&updatedAt,
			&lastModifiedAt,
		); err != nil {
			return nil, err
		}

		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
		record.SyncStatus = "synced"
		records = append(records, record)
	}

	return records, rows.Err()
}

func listAllStudents(ctx context.Context, pool *pgxpool.Pool) ([]sync.StudentRecord, error) {
	query := `
		SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at
		FROM students
		WHERE deleted_at IS NULL
		ORDER BY full_name ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.StudentRecord, 0)
	for rows.Next() {
		var record sync.StudentRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		var dateOfBirth *time.Time
		var joinedAt *time.Time
		if err := rows.Scan(
			&record.ID,
			&record.StudentCode,
			&record.FullName,
			&dateOfBirth,
			&record.Gender,
			&record.Phone,
			&record.Email,
			&record.Address,
			&record.ClubID,
			&record.BeltRankID,
			&joinedAt,
			&record.Status,
			&record.Notes,
			&createdAt,
			&updatedAt,
			&lastModifiedAt,
		); err != nil {
			return nil, err
		}

		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
		record.SyncStatus = "synced"
		if dateOfBirth != nil {
			value := dateOfBirth.UTC().Format("2006-01-02")
			record.DateOfBirth = &value
		}
		if joinedAt != nil {
			value := joinedAt.UTC().Format("2006-01-02")
			record.JoinedAt = &value
		}
		records = append(records, record)
	}

	return records, rows.Err()
}
