-- name: ListActiveClubs :many
SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM clubs
WHERE deleted_at IS NULL
ORDER BY name ASC;

-- name: ListActiveClubGroups :many
SELECT id, club_id, name, description, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM club_groups
WHERE deleted_at IS NULL
ORDER BY club_id ASC, name ASC;

-- name: ListActiveClubSchedules :many
SELECT id, club_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM club_schedules
WHERE deleted_at IS NULL
ORDER BY club_id ASC, weekday ASC;

-- name: ListActiveBeltRanks :many
SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM belt_ranks
WHERE deleted_at IS NULL
ORDER BY rank_order ASC;

-- name: ListActiveStudents :many
SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
FROM students
WHERE deleted_at IS NULL
ORDER BY full_name ASC;

-- name: ListActiveStudentScheduleProfiles :many
SELECT id, student_id, mode, created_at, updated_at, last_modified_at, deleted_at
FROM student_schedule_profiles
WHERE deleted_at IS NULL
ORDER BY student_id ASC;

-- name: ListActiveStudentSchedules :many
SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM student_schedules
WHERE deleted_at IS NULL
ORDER BY student_id ASC, weekday ASC;

-- name: ListActiveAttendanceSessions :many
SELECT id, club_id, session_date, status, notes, created_at, updated_at, last_modified_at, deleted_at
FROM attendance_sessions
WHERE deleted_at IS NULL
ORDER BY session_date DESC, updated_at DESC;

-- name: ListActiveAttendanceRecords :many
SELECT id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at, deleted_at
FROM attendance_records
WHERE deleted_at IS NULL
ORDER BY session_id ASC, student_id ASC;

-- name: FindActiveClubByCode :one
SELECT id, code, name, phone, email, address, notes, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM clubs
WHERE deleted_at IS NULL
	AND id <> $1
	AND code = $2
LIMIT 1;

-- name: FindActiveClubScheduleByWeekday :one
SELECT id, club_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM club_schedules
WHERE deleted_at IS NULL
	AND id <> $1
	AND club_id = $2
	AND weekday = $3
LIMIT 1;

-- name: FindActiveBeltRankByOrder :one
SELECT id, name, rank_order, description, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM belt_ranks
WHERE deleted_at IS NULL
	AND id <> $1
	AND rank_order = $2
LIMIT 1;

-- name: FindActiveStudentByCode :one
SELECT id, student_code, full_name, date_of_birth, gender, phone, email, address, club_id, group_id, belt_rank_id, joined_at, status, notes, created_at, updated_at, last_modified_at, deleted_at
FROM students
WHERE deleted_at IS NULL
	AND id <> $1
	AND student_code = $2
LIMIT 1;

-- name: FindActiveStudentScheduleByWeekday :one
SELECT id, student_id, weekday, is_active, created_at, updated_at, last_modified_at, deleted_at
FROM student_schedules
WHERE deleted_at IS NULL
	AND id <> $1
	AND student_id = $2
	AND weekday = $3
LIMIT 1;

-- name: FindActiveAttendanceRecordBySessionAndStudent :one
SELECT id, session_id, student_id, attendance_status, check_in_at, notes, created_at, updated_at, last_modified_at, deleted_at
FROM attendance_records
WHERE deleted_at IS NULL
	AND id <> $1
	AND session_id = $2
	AND student_id = $3
LIMIT 1;

-- name: ListActiveClubScheduleWeekdays :many
SELECT weekday
FROM club_schedules
WHERE club_id = $1
	AND deleted_at IS NULL
	AND is_active = TRUE
ORDER BY weekday ASC;
