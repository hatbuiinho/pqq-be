package db

import "context"

const listActiveClubs = `-- name: ListActiveClubs :many
SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at
FROM clubs
WHERE deleted_at IS NULL
ORDER BY name ASC
`

func (q *Queries) ListActiveClubs(ctx context.Context) ([]Club, error) {
	rows, err := q.db.Query(ctx, listActiveClubs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Club, 0)
	for rows.Next() {
		var item Club
		if err := rows.Scan(
			&item.ID,
			&item.Code,
			&item.Name,
			&item.Phone,
			&item.Email,
			&item.Address,
			&item.Notes,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveClubGroups = `-- name: ListActiveClubGroups :many
SELECT id, club_id, name, description, is_active, created_at, updated_at, last_modified_at
FROM club_groups
WHERE deleted_at IS NULL
ORDER BY club_id ASC, name ASC
`

func (q *Queries) ListActiveClubGroups(ctx context.Context) ([]ClubGroup, error) {
	rows, err := q.db.Query(ctx, listActiveClubGroups)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ClubGroup, 0)
	for rows.Next() {
		var item ClubGroup
		if err := rows.Scan(
			&item.ID,
			&item.ClubID,
			&item.Name,
			&item.Description,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveClubSchedules = `-- name: ListActiveClubSchedules :many
SELECT id, club_id, weekday, is_active, created_at, updated_at, last_modified_at
FROM club_schedules
WHERE deleted_at IS NULL
ORDER BY club_id ASC, weekday ASC
`

func (q *Queries) ListActiveClubSchedules(ctx context.Context) ([]ClubSchedule, error) {
	rows, err := q.db.Query(ctx, listActiveClubSchedules)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ClubSchedule, 0)
	for rows.Next() {
		var item ClubSchedule
		if err := rows.Scan(
			&item.ID,
			&item.ClubID,
			&item.Weekday,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveBeltRanks = `-- name: ListActiveBeltRanks :many
SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at
FROM belt_ranks
WHERE deleted_at IS NULL
ORDER BY rank_order ASC
`

func (q *Queries) ListActiveBeltRanks(ctx context.Context) ([]BeltRank, error) {
	rows, err := q.db.Query(ctx, listActiveBeltRanks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]BeltRank, 0)
	for rows.Next() {
		var item BeltRank
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.RankOrder,
			&item.Description,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveStudents = `-- name: ListActiveStudents :many
SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at
FROM students
WHERE deleted_at IS NULL
ORDER BY full_name ASC
`

func (q *Queries) ListActiveStudents(ctx context.Context) ([]Student, error) {
	rows, err := q.db.Query(ctx, listActiveStudents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Student, 0)
	for rows.Next() {
		var item Student
		if err := rows.Scan(
			&item.ID,
			&item.StudentCode,
			&item.FullName,
			&item.DateOfBirth,
			&item.Gender,
			&item.Phone,
			&item.Email,
			&item.Address,
			&item.ClubID,
			&item.GroupID,
			&item.BeltRankID,
			&item.JoinedAt,
			&item.Status,
			&item.Notes,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveStudentScheduleProfiles = `-- name: ListActiveStudentScheduleProfiles :many
SELECT id, student_id, mode, created_at, updated_at, last_modified_at
FROM student_schedule_profiles
WHERE deleted_at IS NULL
ORDER BY student_id ASC
`

func (q *Queries) ListActiveStudentScheduleProfiles(ctx context.Context) ([]StudentScheduleProfile, error) {
	rows, err := q.db.Query(ctx, listActiveStudentScheduleProfiles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]StudentScheduleProfile, 0)
	for rows.Next() {
		var item StudentScheduleProfile
		if err := rows.Scan(
			&item.ID,
			&item.StudentID,
			&item.Mode,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveStudentSchedules = `-- name: ListActiveStudentSchedules :many
SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at
FROM student_schedules
WHERE deleted_at IS NULL
ORDER BY student_id ASC, weekday ASC
`

func (q *Queries) ListActiveStudentSchedules(ctx context.Context) ([]StudentSchedule, error) {
	rows, err := q.db.Query(ctx, listActiveStudentSchedules)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]StudentSchedule, 0)
	for rows.Next() {
		var item StudentSchedule
		if err := rows.Scan(
			&item.ID,
			&item.StudentID,
			&item.Weekday,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveAttendanceSessions = `-- name: ListActiveAttendanceSessions :many
SELECT id, club_id, session_date, status, notes, created_at, updated_at, last_modified_at
FROM attendance_sessions
WHERE deleted_at IS NULL
ORDER BY session_date DESC, updated_at DESC
`

func (q *Queries) ListActiveAttendanceSessions(ctx context.Context) ([]AttendanceSession, error) {
	rows, err := q.db.Query(ctx, listActiveAttendanceSessions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AttendanceSession, 0)
	for rows.Next() {
		var item AttendanceSession
		if err := rows.Scan(
			&item.ID,
			&item.ClubID,
			&item.SessionDate,
			&item.Status,
			&item.Notes,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const listActiveAttendanceRecords = `-- name: ListActiveAttendanceRecords :many
SELECT id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at
FROM attendance_records
WHERE deleted_at IS NULL
ORDER BY session_id ASC, student_id ASC
`

func (q *Queries) ListActiveAttendanceRecords(ctx context.Context) ([]AttendanceRecord, error) {
	rows, err := q.db.Query(ctx, listActiveAttendanceRecords)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AttendanceRecord, 0)
	for rows.Next() {
		var item AttendanceRecord
		if err := rows.Scan(
			&item.ID,
			&item.SessionID,
			&item.StudentID,
			&item.AttendanceStatus,
			&item.CheckInAt,
			&item.Notes,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LastModifiedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const findActiveClubByCode = `-- name: FindActiveClubByCode :one
SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM clubs
WHERE deleted_at IS NULL
	AND id <> $1
	AND code = $2
LIMIT 1
`

func (q *Queries) FindActiveClubByCode(ctx context.Context, arg FindActiveClubByCodeParams) (Club, error) {
	row := q.db.QueryRow(ctx, findActiveClubByCode, arg.ExcludeID, arg.Code)
	var item Club
	err := row.Scan(
		&item.ID,
		&item.Code,
		&item.Name,
		&item.Phone,
		&item.Email,
		&item.Address,
		&item.Notes,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastModifiedAt,
		&item.DeletedAt,
	)
	return item, err
}

const findActiveClubScheduleByWeekday = `-- name: FindActiveClubScheduleByWeekday :one
SELECT id, club_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM club_schedules
WHERE deleted_at IS NULL
	AND id <> $1
	AND club_id = $2
	AND weekday = $3
LIMIT 1
`

func (q *Queries) FindActiveClubScheduleByWeekday(ctx context.Context, arg FindActiveClubScheduleByWeekdayParams) (ClubSchedule, error) {
	row := q.db.QueryRow(ctx, findActiveClubScheduleByWeekday, arg.ExcludeID, arg.ClubID, arg.Weekday)
	var item ClubSchedule
	err := row.Scan(
		&item.ID,
		&item.ClubID,
		&item.Weekday,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastModifiedAt,
		&item.DeletedAt,
	)
	return item, err
}

const findActiveBeltRankByOrder = `-- name: FindActiveBeltRankByOrder :one
SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM belt_ranks
WHERE deleted_at IS NULL
	AND id <> $1
	AND rank_order = $2
LIMIT 1
`

func (q *Queries) FindActiveBeltRankByOrder(ctx context.Context, arg FindActiveBeltRankByOrderParams) (BeltRank, error) {
	row := q.db.QueryRow(ctx, findActiveBeltRankByOrder, arg.ExcludeID, arg.RankOrder)
	var item BeltRank
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.RankOrder,
		&item.Description,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastModifiedAt,
		&item.DeletedAt,
	)
	return item, err
}

const findActiveStudentByCode = `-- name: FindActiveStudentByCode :one
SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
FROM students
WHERE deleted_at IS NULL
	AND id <> $1
	AND student_code = $2
LIMIT 1
`

func (q *Queries) FindActiveStudentByCode(ctx context.Context, arg FindActiveStudentByCodeParams) (Student, error) {
	row := q.db.QueryRow(ctx, findActiveStudentByCode, arg.ExcludeID, arg.StudentCode)
	var item Student
	err := row.Scan(
		&item.ID,
		&item.StudentCode,
		&item.FullName,
		&item.DateOfBirth,
		&item.Gender,
		&item.Phone,
		&item.Email,
		&item.Address,
		&item.ClubID,
		&item.GroupID,
		&item.BeltRankID,
		&item.JoinedAt,
		&item.Status,
		&item.Notes,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastModifiedAt,
		&item.DeletedAt,
	)
	return item, err
}

const findActiveStudentScheduleByWeekday = `-- name: FindActiveStudentScheduleByWeekday :one
SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM student_schedules
WHERE deleted_at IS NULL
	AND id <> $1
	AND student_id = $2
	AND weekday = $3
LIMIT 1
`

func (q *Queries) FindActiveStudentScheduleByWeekday(ctx context.Context, arg FindActiveStudentScheduleByWeekdayParams) (StudentSchedule, error) {
	row := q.db.QueryRow(ctx, findActiveStudentScheduleByWeekday, arg.ExcludeID, arg.StudentID, arg.Weekday)
	var item StudentSchedule
	err := row.Scan(
		&item.ID,
		&item.StudentID,
		&item.Weekday,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastModifiedAt,
		&item.DeletedAt,
	)
	return item, err
}

const findActiveAttendanceRecordBySessionAndStudent = `-- name: FindActiveAttendanceRecordBySessionAndStudent :one
SELECT id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at, deleted_at
FROM attendance_records
WHERE deleted_at IS NULL
	AND id <> $1
	AND session_id = $2
	AND student_id = $3
LIMIT 1
`

func (q *Queries) FindActiveAttendanceRecordBySessionAndStudent(ctx context.Context, arg FindActiveAttendanceRecordBySessionAndStudentParams) (AttendanceRecord, error) {
	row := q.db.QueryRow(ctx, findActiveAttendanceRecordBySessionAndStudent, arg.ExcludeID, arg.SessionID, arg.StudentID)
	var item AttendanceRecord
	err := row.Scan(
		&item.ID,
		&item.SessionID,
		&item.StudentID,
		&item.AttendanceStatus,
		&item.CheckInAt,
		&item.Notes,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.LastModifiedAt,
		&item.DeletedAt,
	)
	return item, err
}

const listActiveClubScheduleWeekdays = `-- name: ListActiveClubScheduleWeekdays :many
SELECT weekday
FROM club_schedules
WHERE club_id = $1
	AND deleted_at IS NULL
	AND is_active = TRUE
ORDER BY weekday ASC
`

func (q *Queries) ListActiveClubScheduleWeekdays(ctx context.Context, clubID string) ([]string, error) {
	rows, err := q.db.Query(ctx, listActiveClubScheduleWeekdays, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var weekday string
		if err := rows.Scan(&weekday); err != nil {
			return nil, err
		}
		items = append(items, weekday)
	}
	return items, rows.Err()
}
