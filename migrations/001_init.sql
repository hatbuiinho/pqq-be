CREATE TABLE IF NOT EXISTS clubs (
    id TEXT PRIMARY KEY,
    code TEXT NULL,
    name TEXT NOT NULL,
    phone TEXT NULL,
    email TEXT NULL,
    address TEXT NULL,
    notes TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_modified_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_clubs_name
    ON clubs (name);

CREATE UNIQUE INDEX IF NOT EXISTS idx_clubs_code_active_unique
    ON clubs (code)
    WHERE deleted_at IS NULL AND code IS NOT NULL;

CREATE TABLE IF NOT EXISTS club_groups (
    id TEXT PRIMARY KEY,
    club_id TEXT NOT NULL REFERENCES clubs(id),
    name TEXT NOT NULL,
    description TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_modified_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_club_groups_club_id
    ON club_groups (club_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_club_groups_club_name_active_unique
	ON club_groups (club_id, name)
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS club_schedules (
	id TEXT PRIMARY KEY,
	club_id TEXT NOT NULL REFERENCES clubs(id),
	weekday TEXT NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_club_schedules_club_id
	ON club_schedules (club_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_club_schedules_club_weekday_active_unique
	ON club_schedules (club_id, weekday)
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS belt_ranks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    rank_order INTEGER NOT NULL,
    description TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_modified_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_belt_ranks_name
    ON belt_ranks (name);

CREATE UNIQUE INDEX IF NOT EXISTS idx_belt_ranks_rank_order_active_unique
    ON belt_ranks (rank_order)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS students (
    id TEXT PRIMARY KEY,
    student_code TEXT NULL UNIQUE,
    full_name TEXT NOT NULL,
    date_of_birth DATE NULL,
    gender TEXT NULL,
    phone TEXT NULL,
    email TEXT NULL,
    address TEXT NULL,
    club_id TEXT NOT NULL REFERENCES clubs(id),
    group_id TEXT NULL REFERENCES club_groups(id),
    belt_rank_id TEXT NOT NULL REFERENCES belt_ranks(id),
    joined_at DATE NULL,
    status TEXT NOT NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_modified_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_students_full_name
    ON students (full_name);

CREATE INDEX IF NOT EXISTS idx_students_club_id
    ON students (club_id);

CREATE INDEX IF NOT EXISTS idx_students_group_id
    ON students (group_id);

CREATE INDEX IF NOT EXISTS idx_students_belt_rank_id
    ON students (belt_rank_id);

CREATE INDEX IF NOT EXISTS idx_students_status
	ON students (status);

CREATE TABLE IF NOT EXISTS student_schedule_profiles (
	id TEXT PRIMARY KEY,
	student_id TEXT NOT NULL UNIQUE REFERENCES students(id),
	mode TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_student_schedule_profiles_student_id
	ON student_schedule_profiles (student_id);

CREATE TABLE IF NOT EXISTS student_schedules (
	id TEXT PRIMARY KEY,
	student_id TEXT NOT NULL REFERENCES students(id),
	weekday TEXT NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_student_schedules_student_id
	ON student_schedules (student_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_student_schedules_student_weekday_active_unique
	ON student_schedules (student_id, weekday)
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS attendance_sessions (
    id TEXT PRIMARY KEY,
    club_id TEXT NOT NULL REFERENCES clubs(id),
    session_date DATE NOT NULL,
    status TEXT NOT NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_modified_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_attendance_sessions_club_id
    ON attendance_sessions (club_id);

CREATE INDEX IF NOT EXISTS idx_attendance_sessions_session_date
    ON attendance_sessions (session_date);

CREATE TABLE IF NOT EXISTS attendance_records (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES attendance_sessions(id),
    student_id TEXT NOT NULL REFERENCES students(id),
    attendance_status TEXT NOT NULL,
    check_in_at TIMESTAMPTZ NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_modified_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_attendance_records_session_id
    ON attendance_records (session_id);

CREATE INDEX IF NOT EXISTS idx_attendance_records_student_id
    ON attendance_records (student_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_attendance_records_session_student_active_unique
    ON attendance_records (session_id, student_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS sync_processed_mutations (
    device_id TEXT NOT NULL,
    mutation_id TEXT NOT NULL,
    entity_name TEXT NOT NULL,
    record_id TEXT NOT NULL,
    client_modified_at TIMESTAMPTZ NOT NULL,
    server_modified_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (device_id, mutation_id)
);

CREATE TABLE IF NOT EXISTS sync_counters (
    scope TEXT PRIMARY KEY,
    last_value BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_change_log (
    change_id BIGSERIAL PRIMARY KEY,
    entity_name TEXT NOT NULL,
    record_id TEXT NOT NULL,
    payload JSONB NOT NULL,
    server_modified_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sync_change_log_server_modified_at
    ON sync_change_log (server_modified_at ASC, change_id ASC);
