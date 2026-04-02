package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pqq/be/internal/postgres/db"
	"pqq/be/internal/sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SyncStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewSyncStore(pool *pgxpool.Pool) *SyncStore {
	return &SyncStore{
		pool:    pool,
		queries: db.New(pool),
	}
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
	case sync.EntityClubGroups:
		return getClubGroupForUpdate(ctx, tx, recordID)
	case sync.EntityClubSchedules:
		return getClubScheduleForUpdate(ctx, tx, recordID)
	case sync.EntityBeltRanks:
		return getBeltRankForUpdate(ctx, tx, recordID)
	case sync.EntityStudents:
		return getStudentForUpdate(ctx, tx, recordID)
	case sync.EntityStudentScheduleProfiles:
		return getStudentScheduleProfileForUpdate(ctx, tx, recordID)
	case sync.EntityStudentSchedules:
		return getStudentScheduleForUpdate(ctx, tx, recordID)
	case sync.EntityAttendanceSessions:
		return getAttendanceSessionForUpdate(ctx, tx, recordID)
	case sync.EntityAttendanceRecords:
		return getAttendanceRecordForUpdate(ctx, tx, recordID)
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
	case sync.EntityClubGroups:
		if err := upsertClubGroup(ctx, tx, record); err != nil {
			return err
		}
	case sync.EntityClubSchedules:
		if err := upsertClubSchedule(ctx, tx, record); err != nil {
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
	case sync.EntityStudentScheduleProfiles:
		if err := upsertStudentScheduleProfile(ctx, tx, record); err != nil {
			return err
		}
	case sync.EntityStudentSchedules:
		if err := upsertStudentSchedule(ctx, tx, record); err != nil {
			return err
		}
	case sync.EntityAttendanceSessions:
		if err := upsertAttendanceSession(ctx, tx, record); err != nil {
			return err
		}
	case sync.EntityAttendanceRecords:
		if err := upsertAttendanceRecord(ctx, tx, record); err != nil {
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
	row, err := s.queries.WithTx(tx).FindActiveClubByCode(ctx, db.FindActiveClubByCodeParams{
		ID: excludeID,
		Code: pgtype.Text{
			String: clubCode,
			Valid:  true,
		},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record := clubRecordFromRow(row)
	return storedRecordFromSyncRecord(sync.EntityClubs, record.ID, record.DeletedAt, record.LastModifiedAt, record)
}

func (s *SyncStore) FindClubByName(
	ctx context.Context,
	tx pgx.Tx,
	clubName string,
) (*sync.StoredRecord, error) {
	rows, err := s.queries.WithTx(tx).ListActiveClubs(ctx)
	if err != nil {
		return nil, err
	}

	targetName := sync.NormalizeSearchText(clubName)
	for _, row := range rows {
		if sync.NormalizeSearchText(row.Name) != targetName {
			continue
		}
		record := clubRecordFromRow(row)
		return storedRecordFromSyncRecord(sync.EntityClubs, record.ID, record.DeletedAt, record.LastModifiedAt, record)
	}

	return nil, nil
}

func (s *SyncStore) FindClubGroupByName(
	ctx context.Context,
	tx pgx.Tx,
	clubID string,
	groupName string,
	excludeID string,
) (*sync.StoredRecord, error) {
	rows, err := s.queries.WithTx(tx).ListActiveClubGroups(ctx)
	if err != nil {
		return nil, err
	}

	targetName := sync.NormalizeSearchText(groupName)
	for _, row := range rows {
		if row.ClubID != clubID || row.ID == excludeID || sync.NormalizeSearchText(row.Name) != targetName {
			continue
		}
		record := clubGroupRecordFromRow(row)
		return storedRecordFromSyncRecord(sync.EntityClubGroups, record.ID, record.DeletedAt, record.LastModifiedAt, record)
	}

	return nil, nil
}

func (s *SyncStore) FindClubScheduleByWeekday(
	ctx context.Context,
	tx pgx.Tx,
	clubID string,
	weekday string,
	excludeID string,
) (*sync.StoredRecord, error) {
	row, err := s.queries.WithTx(tx).FindActiveClubScheduleByWeekday(ctx, db.FindActiveClubScheduleByWeekdayParams{
		ID:      excludeID,
		ClubID:  clubID,
		Weekday: weekday,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	record := clubScheduleRecordFromRow(row)
	return storedRecordFromSyncRecord(sync.EntityClubSchedules, record.ID, record.DeletedAt, record.LastModifiedAt, record)
}

func (s *SyncStore) FindBeltRankByOrder(
	ctx context.Context,
	tx pgx.Tx,
	order int,
	excludeID string,
) (*sync.StoredRecord, error) {
	row, err := s.queries.WithTx(tx).FindActiveBeltRankByOrder(ctx, db.FindActiveBeltRankByOrderParams{
		ID:        excludeID,
		RankOrder: int32(order),
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	record := beltRankRecordFromRow(row)
	return storedRecordFromSyncRecord(sync.EntityBeltRanks, record.ID, record.DeletedAt, record.LastModifiedAt, record)
}

func (s *SyncStore) FindBeltRankByName(
	ctx context.Context,
	tx pgx.Tx,
	beltRankName string,
) (*sync.StoredRecord, error) {
	rows, err := s.queries.WithTx(tx).ListActiveBeltRanks(ctx)
	if err != nil {
		return nil, err
	}

	targetName := sync.NormalizeSearchText(beltRankName)
	for _, row := range rows {
		if sync.NormalizeSearchText(row.Name) != targetName {
			continue
		}
		record := beltRankRecordFromRow(row)
		return storedRecordFromSyncRecord(sync.EntityBeltRanks, record.ID, record.DeletedAt, record.LastModifiedAt, record)
	}

	return nil, nil
}

func (s *SyncStore) FindStudentByCode(
	ctx context.Context,
	tx pgx.Tx,
	studentCode string,
	excludeID string,
) (*sync.StoredRecord, error) {
	row, err := s.queries.WithTx(tx).FindActiveStudentByCode(ctx, db.FindActiveStudentByCodeParams{
		ID: excludeID,
		StudentCode: pgtype.Text{
			String: studentCode,
			Valid:  true,
		},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record := studentRecordFromRow(row)
	return storedRecordFromSyncRecord(sync.EntityStudents, record.ID, record.DeletedAt, record.LastModifiedAt, record)
}

func (s *SyncStore) FindStudentScheduleByWeekday(
	ctx context.Context,
	tx pgx.Tx,
	studentID string,
	weekday string,
	excludeID string,
) (*sync.StoredRecord, error) {
	row, err := s.queries.WithTx(tx).FindActiveStudentScheduleByWeekday(ctx, db.FindActiveStudentScheduleByWeekdayParams{
		ID:        excludeID,
		StudentID: studentID,
		Weekday:   weekday,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	record := studentScheduleRecordFromRow(row)
	return storedRecordFromSyncRecord(sync.EntityStudentSchedules, record.ID, record.DeletedAt, record.LastModifiedAt, record)
}

func (s *SyncStore) FindAttendanceRecordBySessionAndStudent(
	ctx context.Context,
	tx pgx.Tx,
	sessionID string,
	studentID string,
	excludeID string,
) (*sync.StoredRecord, error) {
	row, err := s.queries.WithTx(tx).FindActiveAttendanceRecordBySessionAndStudent(ctx, db.FindActiveAttendanceRecordBySessionAndStudentParams{
		ID:        excludeID,
		SessionID: sessionID,
		StudentID: studentID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	record := attendanceRecordFromRow(row)
	return storedRecordFromSyncRecord(sync.EntityAttendanceRecords, record.ID, record.DeletedAt, record.LastModifiedAt, record)
}

func (s *SyncStore) ListClubScheduleWeekdays(ctx context.Context, tx pgx.Tx, clubID string) ([]string, error) {
	return s.queries.WithTx(tx).ListActiveClubScheduleWeekdays(ctx, clubID)
}

func (s *SyncStore) ReplaceStudentSchedule(ctx context.Context, tx pgx.Tx, studentID string, mode string, weekdays []string, serverNow string) error {
	existingProfile, err := getStudentScheduleProfileForUpdate(ctx, tx, studentID)
	if err != nil {
		return err
	}

	profileCreatedAt := serverNow
	if existingProfile != nil {
		var existingRecord sync.StudentScheduleProfileRecord
		if err := json.Unmarshal(existingProfile.Payload, &existingRecord); err != nil {
			return err
		}
		profileCreatedAt = existingRecord.CreatedAt
	}

	profileRecord := sync.StudentScheduleProfileRecord{
		BaseRecord: sync.BaseRecord{
			ID:             studentID,
			CreatedAt:      profileCreatedAt,
			UpdatedAt:      serverNow,
			LastModifiedAt: serverNow,
			SyncStatus:     "synced",
		},
		StudentID: studentID,
		Mode:      mode,
	}

	profilePayload, err := json.Marshal(profileRecord)
	if err != nil {
		return err
	}

	profileStored := sync.StoredRecord{
		EntityName:       sync.EntityStudentScheduleProfiles,
		RecordID:         studentID,
		Payload:          profilePayload,
		LastModifiedAt:   serverNow,
		ServerModifiedAt: serverNow,
	}
	if err := upsertStudentScheduleProfile(ctx, tx, profileStored); err != nil {
		return err
	}
	if err := insertChangeLog(ctx, tx, profileStored); err != nil {
		return err
	}

	query := `
		SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM student_schedules
		WHERE student_id = $1
		FOR UPDATE
	`
	rows, err := tx.Query(ctx, query, studentID)
	if err != nil {
		return err
	}
	defer rows.Close()

	existingSchedules := make(map[string]sync.StudentScheduleRecord)
	for rows.Next() {
		var record sync.StudentScheduleRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		var deletedAt *time.Time
		if err := rows.Scan(
			&record.ID,
			&record.StudentID,
			&record.Weekday,
			&record.IsActive,
			&createdAt,
			&updatedAt,
			&lastModifiedAt,
			&deletedAt,
		); err != nil {
			return err
		}
		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
		record.SyncStatus = "synced"
		if deletedAt != nil {
			value := deletedAt.UTC().Format(time.RFC3339Nano)
			record.DeletedAt = &value
		}
		existingSchedules[record.Weekday] = record
	}
	if err := rows.Err(); err != nil {
		return err
	}

	incomingSet := make(map[string]struct{}, len(weekdays))
	for _, weekday := range weekdays {
		incomingSet[weekday] = struct{}{}
		existingRecord, exists := existingSchedules[weekday]
		createdAt := serverNow
		if exists {
			createdAt = existingRecord.CreatedAt
		}

		record := sync.StudentScheduleRecord{
			BaseRecord: sync.BaseRecord{
				ID:             fmt.Sprintf("%s:%s", studentID, weekday),
				CreatedAt:      createdAt,
				UpdatedAt:      serverNow,
				LastModifiedAt: serverNow,
				SyncStatus:     "synced",
			},
			StudentID: studentID,
			Weekday:   weekday,
			IsActive:  true,
		}

		payload, err := json.Marshal(record)
		if err != nil {
			return err
		}
		stored := sync.StoredRecord{
			EntityName:       sync.EntityStudentSchedules,
			RecordID:         record.ID,
			Payload:          payload,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		}
		if err := upsertStudentSchedule(ctx, tx, stored); err != nil {
			return err
		}
		if err := insertChangeLog(ctx, tx, stored); err != nil {
			return err
		}
	}

	for weekday, existingRecord := range existingSchedules {
		if _, exists := incomingSet[weekday]; exists {
			continue
		}
		deletedAt := serverNow
		existingRecord.UpdatedAt = serverNow
		existingRecord.LastModifiedAt = serverNow
		existingRecord.DeletedAt = &deletedAt

		payload, err := json.Marshal(existingRecord)
		if err != nil {
			return err
		}
		stored := sync.StoredRecord{
			EntityName:       sync.EntityStudentSchedules,
			RecordID:         existingRecord.ID,
			Payload:          payload,
			DeletedAt:        &deletedAt,
			LastModifiedAt:   serverNow,
			ServerModifiedAt: serverNow,
		}
		if err := upsertStudentSchedule(ctx, tx, stored); err != nil {
			return err
		}
		if err := insertChangeLog(ctx, tx, stored); err != nil {
			return err
		}
	}

	return nil
}

func (s *SyncStore) RecordExists(ctx context.Context, tx pgx.Tx, entityName sync.EntityName, recordID string) (bool, error) {
	var query string
	switch entityName {
	case sync.EntityClubs:
		query = `SELECT EXISTS(SELECT 1 FROM clubs WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityClubGroups:
		query = `SELECT EXISTS(SELECT 1 FROM club_groups WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityClubSchedules:
		query = `SELECT EXISTS(SELECT 1 FROM club_schedules WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityBeltRanks:
		query = `SELECT EXISTS(SELECT 1 FROM belt_ranks WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityStudents:
		query = `SELECT EXISTS(SELECT 1 FROM students WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityStudentScheduleProfiles:
		query = `SELECT EXISTS(SELECT 1 FROM student_schedule_profiles WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityStudentSchedules:
		query = `SELECT EXISTS(SELECT 1 FROM student_schedules WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityAttendanceSessions:
		query = `SELECT EXISTS(SELECT 1 FROM attendance_sessions WHERE id = $1 AND deleted_at IS NULL)`
	case sync.EntityAttendanceRecords:
		query = `SELECT EXISTS(SELECT 1 FROM attendance_records WHERE id = $1 AND deleted_at IS NULL)`
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

func (s *SyncStore) FindActiveStudentProfileByCode(
	ctx context.Context,
	studentCode string,
) (*sync.StudentPublicProfile, error) {
	normalizedCode := strings.TrimSpace(studentCode)
	if normalizedCode == "" {
		return nil, nil
	}

	query := `
		SELECT
			s.id,
			s.student_code,
			s.full_name,
			to_char(s.date_of_birth, 'YYYY-MM-DD') AS date_of_birth,
			s.gender,
			s.phone,
			s.email,
			s.address,
			s.status,
			to_char(s.joined_at, 'YYYY-MM-DD') AS joined_at,
			s.notes,
			s.club_id,
			c.name AS club_name,
			s.group_id,
			g.name AS group_name,
			s.belt_rank_id,
			b.name AS belt_rank_name
		FROM students s
		INNER JOIN clubs c ON c.id = s.club_id
		INNER JOIN belt_ranks b ON b.id = s.belt_rank_id
		LEFT JOIN club_groups g ON g.id = s.group_id
		WHERE s.deleted_at IS NULL
			AND c.deleted_at IS NULL
			AND b.deleted_at IS NULL
			AND LOWER(s.student_code) = LOWER($1)
		LIMIT 1
	`

	var profile sync.StudentPublicProfile
	if err := s.pool.QueryRow(ctx, query, normalizedCode).Scan(
		&profile.ID,
		&profile.StudentCode,
		&profile.FullName,
		&profile.DateOfBirth,
		&profile.Gender,
		&profile.Phone,
		&profile.Email,
		&profile.Address,
		&profile.Status,
		&profile.JoinedAt,
		&profile.Notes,
		&profile.ClubID,
		&profile.ClubName,
		&profile.GroupID,
		&profile.GroupName,
		&profile.BeltRankID,
		&profile.BeltRank,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &profile, nil
}

func (s *SyncStore) ListAllCurrent(ctx context.Context) ([]sync.ClubRecord, []sync.ClubGroupRecord, []sync.ClubScheduleRecord, []sync.BeltRankRecord, []sync.StudentRecord, []sync.StudentScheduleProfileRecord, []sync.StudentScheduleRecord, []sync.AttendanceSessionRecord, []sync.AttendanceRecord, error) {
	clubRows, err := s.queries.ListActiveClubs(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	clubGroupRows, err := s.queries.ListActiveClubGroups(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	clubScheduleRows, err := s.queries.ListActiveClubSchedules(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	beltRankRows, err := s.queries.ListActiveBeltRanks(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	studentRows, err := s.queries.ListActiveStudents(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	studentScheduleProfileRows, err := s.queries.ListActiveStudentScheduleProfiles(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	studentScheduleRows, err := s.queries.ListActiveStudentSchedules(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	attendanceSessionRows, err := s.queries.ListActiveAttendanceSessions(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	attendanceRecordRows, err := s.queries.ListActiveAttendanceRecords(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	clubs := make([]sync.ClubRecord, 0, len(clubRows))
	for _, row := range clubRows {
		clubs = append(clubs, clubRecordFromRow(row))
	}

	clubGroups := make([]sync.ClubGroupRecord, 0, len(clubGroupRows))
	for _, row := range clubGroupRows {
		clubGroups = append(clubGroups, clubGroupRecordFromRow(row))
	}

	clubSchedules := make([]sync.ClubScheduleRecord, 0, len(clubScheduleRows))
	for _, row := range clubScheduleRows {
		clubSchedules = append(clubSchedules, clubScheduleRecordFromRow(row))
	}

	beltRanks := make([]sync.BeltRankRecord, 0, len(beltRankRows))
	for _, row := range beltRankRows {
		beltRanks = append(beltRanks, beltRankRecordFromRow(row))
	}

	students := make([]sync.StudentRecord, 0, len(studentRows))
	for _, row := range studentRows {
		students = append(students, studentRecordFromRow(row))
	}

	studentScheduleProfiles := make([]sync.StudentScheduleProfileRecord, 0, len(studentScheduleProfileRows))
	for _, row := range studentScheduleProfileRows {
		studentScheduleProfiles = append(studentScheduleProfiles, studentScheduleProfileRecordFromRow(row))
	}

	studentSchedules := make([]sync.StudentScheduleRecord, 0, len(studentScheduleRows))
	for _, row := range studentScheduleRows {
		studentSchedules = append(studentSchedules, studentScheduleRecordFromRow(row))
	}

	attendanceSessions := make([]sync.AttendanceSessionRecord, 0, len(attendanceSessionRows))
	for _, row := range attendanceSessionRows {
		attendanceSessions = append(attendanceSessions, attendanceSessionRecordFromRow(row))
	}

	attendanceRecords := make([]sync.AttendanceRecord, 0, len(attendanceRecordRows))
	for _, row := range attendanceRecordRows {
		attendanceRecords = append(attendanceRecords, attendanceRecordFromRow(row))
	}

	return clubs, clubGroups, clubSchedules, beltRanks, students, studentScheduleProfiles, studentSchedules, attendanceSessions, attendanceRecords, nil
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

func getClubGroupForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, club_id, name, description, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM club_groups
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.ClubGroupRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.ClubID,
		&record.Name,
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
		EntityName:       sync.EntityClubGroups,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getClubScheduleForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, club_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM club_schedules
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.ClubScheduleRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.ClubID,
		&record.Weekday,
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
		EntityName:       sync.EntityClubSchedules,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getStudentForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
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

func getStudentScheduleProfileForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, student_id, mode, created_at, updated_at, last_modified_at, deleted_at
		FROM student_schedule_profiles
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.StudentScheduleProfileRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.StudentID,
		&record.Mode,
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
		EntityName:       sync.EntityStudentScheduleProfiles,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getStudentScheduleForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
		FROM student_schedules
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.StudentScheduleRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.StudentID,
		&record.Weekday,
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
		EntityName:       sync.EntityStudentSchedules,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getAttendanceSessionForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, club_id, session_date, status, notes, created_at, updated_at, last_modified_at, deleted_at
		FROM attendance_sessions
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.AttendanceSessionRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	var sessionDate time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.ClubID,
		&sessionDate,
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

	record.SessionDate = sessionDate.UTC().Format("2006-01-02")
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
		EntityName:       sync.EntityAttendanceSessions,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
}

func getAttendanceRecordForUpdate(ctx context.Context, tx pgx.Tx, recordID string) (*sync.StoredRecord, error) {
	query := `
		SELECT id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at, deleted_at
		FROM attendance_records
		WHERE id = $1
		FOR UPDATE
	`

	var record sync.AttendanceRecord
	var createdAt, updatedAt, lastModifiedAt time.Time
	var deletedAt *time.Time
	var checkInAt *time.Time
	if err := tx.QueryRow(ctx, query, recordID).Scan(
		&record.ID,
		&record.SessionID,
		&record.StudentID,
		&record.AttendanceStatus,
		&checkInAt,
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
	if checkInAt != nil {
		value := checkInAt.UTC().Format(time.RFC3339Nano)
		record.CheckInAt = &value
	}
	if deletedAt != nil {
		value := deletedAt.UTC().Format(time.RFC3339Nano)
		record.DeletedAt = &value
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return &sync.StoredRecord{
		EntityName:       sync.EntityAttendanceRecords,
		RecordID:         record.ID,
		Payload:          payload,
		DeletedAt:        record.DeletedAt,
		LastModifiedAt:   record.LastModifiedAt,
		ServerModifiedAt: record.LastModifiedAt,
	}, nil
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
		&record.GroupID,
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

func upsertClubGroup(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.ClubGroupRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO club_groups (
			id, club_id, name, description, is_active, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		ON CONFLICT (id) DO UPDATE
		SET club_id = EXCLUDED.club_id,
			name = EXCLUDED.name,
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
		record.ClubID,
		record.Name,
		record.Description,
		record.IsActive,
		createdAt,
		updatedAt,
		lastModifiedAt,
		deletedAt,
	)
	return err
}

func upsertClubSchedule(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.ClubScheduleRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO club_schedules (
			id, club_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		ON CONFLICT (id) DO UPDATE
		SET club_id = EXCLUDED.club_id,
			weekday = EXCLUDED.weekday,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, query, record.ID, record.ClubID, record.Weekday, record.IsActive, createdAt, updatedAt, lastModifiedAt, deletedAt)
	return err
}

func upsertStudent(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.StudentRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO students (
			id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
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
			group_id = EXCLUDED.group_id,
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
		record.GroupID,
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

func upsertStudentScheduleProfile(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.StudentScheduleProfileRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO student_schedule_profiles (
			id, student_id, mode, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		ON CONFLICT (id) DO UPDATE
		SET student_id = EXCLUDED.student_id,
			mode = EXCLUDED.mode,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, query, record.ID, record.StudentID, record.Mode, createdAt, updatedAt, lastModifiedAt, deletedAt)
	return err
}

func upsertStudentSchedule(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.StudentScheduleRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO student_schedules (
			id, student_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		ON CONFLICT (id) DO UPDATE
		SET student_id = EXCLUDED.student_id,
			weekday = EXCLUDED.weekday,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, query, record.ID, record.StudentID, record.Weekday, record.IsActive, createdAt, updatedAt, lastModifiedAt, deletedAt)
	return err
}

func upsertAttendanceSession(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.AttendanceSessionRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO attendance_sessions (
			id, club_id, session_date, status, notes, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		ON CONFLICT (id) DO UPDATE
		SET club_id = EXCLUDED.club_id,
			session_date = EXCLUDED.session_date,
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
	sessionDate, err := parseRequiredDate(record.SessionDate)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, query, record.ID, record.ClubID, sessionDate, record.Status, record.Notes, createdAt, updatedAt, lastModifiedAt, deletedAt)
	return err
}

func upsertAttendanceRecord(ctx context.Context, tx pgx.Tx, stored sync.StoredRecord) error {
	var record sync.AttendanceRecord
	if err := json.Unmarshal(stored.Payload, &record); err != nil {
		return err
	}

	query := `
		INSERT INTO attendance_records (
			id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at, deleted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
		ON CONFLICT (id) DO UPDATE
		SET session_id = EXCLUDED.session_id,
			student_id = EXCLUDED.student_id,
			attendance_status = EXCLUDED.attendance_status,
			check_in_at = EXCLUDED.check_in_at,
			notes = EXCLUDED.notes,
			updated_at = EXCLUDED.updated_at,
			last_modified_at = EXCLUDED.last_modified_at,
			deleted_at = EXCLUDED.deleted_at
	`

	createdAt, updatedAt, lastModifiedAt, deletedAt, err := parseAuditTimes(record.CreatedAt, record.UpdatedAt, record.LastModifiedAt, record.DeletedAt)
	if err != nil {
		return err
	}
	checkInAt, err := parseOptionalTimestamp(record.CheckInAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, query, record.ID, record.SessionID, record.StudentID, record.AttendanceStatus, checkInAt, record.Notes, createdAt, updatedAt, lastModifiedAt, deletedAt)
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

func parseRequiredDate(value string) (time.Time, error) {
	return time.Parse("2006-01-02", value)
}

func parseOptionalTimestamp(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, *value)
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

func listAllClubGroups(ctx context.Context, pool *pgxpool.Pool) ([]sync.ClubGroupRecord, error) {
	query := `
		SELECT id, club_id, name, description, is_active, created_at, updated_at, last_modified_at
		FROM club_groups
		WHERE deleted_at IS NULL
		ORDER BY club_id ASC, name ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.ClubGroupRecord, 0)
	for rows.Next() {
		var record sync.ClubGroupRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		if err := rows.Scan(
			&record.ID,
			&record.ClubID,
			&record.Name,
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

func listAllClubSchedules(ctx context.Context, pool *pgxpool.Pool) ([]sync.ClubScheduleRecord, error) {
	query := `
		SELECT id, club_id, weekday, is_active, created_at, updated_at, last_modified_at
		FROM club_schedules
		WHERE deleted_at IS NULL
		ORDER BY club_id ASC, weekday ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.ClubScheduleRecord, 0)
	for rows.Next() {
		var record sync.ClubScheduleRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		if err := rows.Scan(
			&record.ID,
			&record.ClubID,
			&record.Weekday,
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
		SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at
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
			&record.GroupID,
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

func listAllStudentScheduleProfiles(ctx context.Context, pool *pgxpool.Pool) ([]sync.StudentScheduleProfileRecord, error) {
	query := `
		SELECT id, student_id, mode, created_at, updated_at, last_modified_at
		FROM student_schedule_profiles
		WHERE deleted_at IS NULL
		ORDER BY student_id ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.StudentScheduleProfileRecord, 0)
	for rows.Next() {
		var record sync.StudentScheduleProfileRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		if err := rows.Scan(
			&record.ID,
			&record.StudentID,
			&record.Mode,
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

func listAllStudentSchedules(ctx context.Context, pool *pgxpool.Pool) ([]sync.StudentScheduleRecord, error) {
	query := `
		SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at
		FROM student_schedules
		WHERE deleted_at IS NULL
		ORDER BY student_id ASC, weekday ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.StudentScheduleRecord, 0)
	for rows.Next() {
		var record sync.StudentScheduleRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		if err := rows.Scan(
			&record.ID,
			&record.StudentID,
			&record.Weekday,
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

func listAllAttendanceSessions(ctx context.Context, pool *pgxpool.Pool) ([]sync.AttendanceSessionRecord, error) {
	query := `
		SELECT id, club_id, session_date, status, notes, created_at, updated_at, last_modified_at
		FROM attendance_sessions
		WHERE deleted_at IS NULL
		ORDER BY session_date DESC, updated_at DESC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.AttendanceSessionRecord, 0)
	for rows.Next() {
		var record sync.AttendanceSessionRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		var sessionDate time.Time
		if err := rows.Scan(
			&record.ID,
			&record.ClubID,
			&sessionDate,
			&record.Status,
			&record.Notes,
			&createdAt,
			&updatedAt,
			&lastModifiedAt,
		); err != nil {
			return nil, err
		}

		record.SessionDate = sessionDate.UTC().Format("2006-01-02")
		record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		record.LastModifiedAt = lastModifiedAt.UTC().Format(time.RFC3339Nano)
		record.SyncStatus = "synced"
		records = append(records, record)
	}

	return records, rows.Err()
}

func listAllAttendanceRecords(ctx context.Context, pool *pgxpool.Pool) ([]sync.AttendanceRecord, error) {
	query := `
		SELECT id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at
		FROM attendance_records
		WHERE deleted_at IS NULL
		ORDER BY session_id ASC, student_id ASC
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]sync.AttendanceRecord, 0)
	for rows.Next() {
		var record sync.AttendanceRecord
		var createdAt, updatedAt, lastModifiedAt time.Time
		var checkInAt *time.Time
		if err := rows.Scan(
			&record.ID,
			&record.SessionID,
			&record.StudentID,
			&record.AttendanceStatus,
			&checkInAt,
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
		if checkInAt != nil {
			value := checkInAt.UTC().Format(time.RFC3339Nano)
			record.CheckInAt = &value
		}
		records = append(records, record)
	}

	return records, rows.Err()
}
