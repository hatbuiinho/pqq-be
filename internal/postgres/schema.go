package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const initSchemaSQL = `
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

ALTER TABLE clubs
	DROP CONSTRAINT IF EXISTS clubs_code_key;

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

ALTER TABLE belt_ranks
	DROP CONSTRAINT IF EXISTS belt_ranks_rank_order_key;

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
	group_id TEXT NULL,
	belt_rank_id TEXT NOT NULL REFERENCES belt_ranks(id),
	joined_at DATE NULL,
	status TEXT NOT NULL,
	notes TEXT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

ALTER TABLE students
	ADD COLUMN IF NOT EXISTS group_id TEXT NULL REFERENCES club_groups(id);

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

CREATE TABLE IF NOT EXISTS student_media (
	id TEXT PRIMARY KEY,
	student_id TEXT NOT NULL REFERENCES students(id),
	media_type TEXT NOT NULL,
	title TEXT NULL,
	description TEXT NULL,
	storage_bucket TEXT NOT NULL,
	storage_key TEXT NOT NULL,
	thumbnail_key TEXT NULL,
	original_filename TEXT NOT NULL,
	mime_type TEXT NOT NULL,
	file_size BIGINT NOT NULL,
	checksum_sha256 TEXT NULL,
	is_primary BOOLEAN NOT NULL DEFAULT FALSE,
	source TEXT NOT NULL DEFAULT 'manual',
	captured_at TIMESTAMPTZ NULL,
	uploaded_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_student_media_student_id
	ON student_media (student_id);

CREATE INDEX IF NOT EXISTS idx_student_media_student_media_type
	ON student_media (student_id, media_type);

CREATE UNIQUE INDEX IF NOT EXISTS idx_student_media_primary_avatar_unique
	ON student_media (student_id)
	WHERE media_type = 'avatar' AND is_primary = TRUE AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS media_import_batches (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	source_type TEXT NOT NULL,
	original_filename TEXT NULL,
	total_items INTEGER NOT NULL DEFAULT 0,
	matched_items INTEGER NOT NULL DEFAULT 0,
	ambiguous_items INTEGER NOT NULL DEFAULT 0,
	unmatched_items INTEGER NOT NULL DEFAULT 0,
	failed_items INTEGER NOT NULL DEFAULT 0,
	imported_items INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	processed_at TIMESTAMPTZ NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_media_import_batches_status
	ON media_import_batches (status);

CREATE TABLE IF NOT EXISTS media_import_batch_items (
	id TEXT PRIMARY KEY,
	batch_id TEXT NOT NULL REFERENCES media_import_batches(id),
	original_filename TEXT NOT NULL,
	temp_storage_bucket TEXT NOT NULL,
	temp_storage_key TEXT NOT NULL,
	mime_type TEXT NOT NULL,
	file_size BIGINT NOT NULL,
	guessed_student_id TEXT NULL REFERENCES students(id),
	guessed_student_name TEXT NULL,
	match_method TEXT NULL,
	match_score INTEGER NULL,
	confirmed_student_id TEXT NULL REFERENCES students(id),
	media_type TEXT NOT NULL DEFAULT 'avatar',
	title TEXT NULL,
	description TEXT NULL,
	status TEXT NOT NULL,
	error_message TEXT NULL,
	final_media_id TEXT NULL REFERENCES student_media(id),
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_media_import_batch_items_batch_id
	ON media_import_batch_items (batch_id);

CREATE INDEX IF NOT EXISTS idx_media_import_batch_items_batch_status
	ON media_import_batch_items (batch_id, status);

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

CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL,
	full_name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	system_role TEXT NOT NULL DEFAULT 'user',
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	last_login_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL,
	CONSTRAINT chk_users_system_role CHECK (system_role IN ('user', 'sys_admin'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_active_unique
	ON users (LOWER(email))
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS club_memberships (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	club_id TEXT NOT NULL REFERENCES clubs(id),
	club_role TEXT NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ NULL,
	CONSTRAINT chk_club_memberships_role CHECK (club_role IN ('owner', 'assistant'))
);

CREATE INDEX IF NOT EXISTS idx_club_memberships_user_id
	ON club_memberships (user_id);

CREATE INDEX IF NOT EXISTS idx_club_memberships_club_id
	ON club_memberships (club_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_club_memberships_active_unique
	ON club_memberships (user_id, club_id)
	WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS club_invites (
	id TEXT PRIMARY KEY,
	club_id TEXT NOT NULL REFERENCES clubs(id),
	inviter_user_id TEXT NOT NULL REFERENCES users(id),
	invitee_email TEXT NULL,
	club_role TEXT NOT NULL,
	token_hash TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	max_uses INTEGER NOT NULL DEFAULT 1,
	use_count INTEGER NOT NULL DEFAULT 0,
	last_used_at TIMESTAMPTZ NULL,
	accepted_at TIMESTAMPTZ NULL,
	accepted_by_user_id TEXT NULL REFERENCES users(id),
	revoked_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	CONSTRAINT chk_club_invites_role CHECK (club_role IN ('owner', 'assistant'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_club_invites_token_hash_active_unique
	ON club_invites (token_hash)
	WHERE revoked_at IS NULL AND accepted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_club_invites_club_id
	ON club_invites (club_id);

CREATE INDEX IF NOT EXISTS idx_club_invites_invitee_email
	ON club_invites (LOWER(invitee_email));

ALTER TABLE club_invites
	ALTER COLUMN invitee_email DROP NOT NULL;

ALTER TABLE club_invites
	ADD COLUMN IF NOT EXISTS max_uses INTEGER NOT NULL DEFAULT 1,
	ADD COLUMN IF NOT EXISTS use_count INTEGER NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ NULL,
	ADD COLUMN IF NOT EXISTS accepted_by_user_id TEXT NULL REFERENCES users(id);

CREATE TABLE IF NOT EXISTS audit_logs (
	id TEXT PRIMARY KEY,
	actor_user_id TEXT NULL REFERENCES users(id),
	club_id TEXT NULL REFERENCES clubs(id),
	entity_type TEXT NOT NULL,
	entity_id TEXT NULL,
	action TEXT NOT NULL,
	old_values JSONB NULL,
	new_values JSONB NULL,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id
	ON audit_logs (actor_user_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_club_id_created_at
	ON audit_logs (club_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_entity
	ON audit_logs (entity_type, entity_id, created_at DESC);

CREATE TABLE IF NOT EXISTS student_messages (
	id TEXT PRIMARY KEY,
	student_id TEXT NOT NULL REFERENCES students(id),
	club_id TEXT NOT NULL REFERENCES clubs(id),
	message_type TEXT NOT NULL,
	content TEXT NOT NULL,
	author_user_id TEXT NULL REFERENCES users(id),
	author_name TEXT NOT NULL,
	attendance_session_id TEXT NULL REFERENCES attendance_sessions(id),
	attendance_record_id TEXT NULL REFERENCES attendance_records(id),
	attendance_session_date DATE NULL,
	attendance_status TEXT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_student_messages_student_id
	ON student_messages (student_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_student_messages_club_id
	ON student_messages (club_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_student_messages_attendance_record_id
	ON student_messages (attendance_record_id)
	WHERE attendance_record_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_student_messages_attendance_note_unique
	ON student_messages (attendance_record_id)
	WHERE message_type = 'attendance_note' AND deleted_at IS NULL;

ALTER TABLE student_messages
	ALTER COLUMN author_user_id DROP NOT NULL;

WITH inserted_student_messages AS (
	INSERT INTO student_messages (
		id,
		student_id,
		club_id,
		message_type,
		content,
		author_user_id,
		author_name,
		attendance_session_id,
		attendance_record_id,
		attendance_session_date,
		attendance_status,
		created_at,
		updated_at,
		last_modified_at,
		deleted_at
	)
	SELECT
		'attendance-note-' || ar.id,
		ar.student_id,
		ats.club_id,
		'attendance_note',
		BTRIM(ar.notes),
		NULL,
		'Hệ thống',
		ar.session_id,
		ar.id,
		ats.session_date,
		ar.attendance_status,
		ar.last_modified_at,
		ar.updated_at,
		ar.last_modified_at,
		NULL
	FROM attendance_records ar
	INNER JOIN attendance_sessions ats ON ats.id = ar.session_id
	WHERE ar.deleted_at IS NULL
		AND ats.deleted_at IS NULL
		AND NULLIF(BTRIM(ar.notes), '') IS NOT NULL
	ON CONFLICT (id) DO NOTHING
	RETURNING
		id,
		student_id,
		club_id,
		message_type,
		content,
		author_user_id,
		author_name,
		attendance_session_id,
		attendance_record_id,
		attendance_session_date,
		attendance_status,
		created_at,
		updated_at,
		last_modified_at
)
INSERT INTO sync_change_log (entity_name, record_id, payload, server_modified_at)
SELECT
	'student_messages',
	inserted_student_messages.id,
	jsonb_build_object(
		'id', inserted_student_messages.id,
		'studentId', inserted_student_messages.student_id,
		'clubId', inserted_student_messages.club_id,
		'messageType', inserted_student_messages.message_type,
		'content', inserted_student_messages.content,
		'authorUserId', inserted_student_messages.author_user_id,
		'authorName', inserted_student_messages.author_name,
		'attendanceSessionId', inserted_student_messages.attendance_session_id,
		'attendanceRecordId', inserted_student_messages.attendance_record_id,
		'attendanceSessionDate', TO_CHAR(inserted_student_messages.attendance_session_date, 'YYYY-MM-DD'),
		'attendanceStatus', inserted_student_messages.attendance_status,
		'createdAt', TO_CHAR(inserted_student_messages.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
		'updatedAt', TO_CHAR(inserted_student_messages.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
		'lastModifiedAt', TO_CHAR(inserted_student_messages.last_modified_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
		'syncStatus', 'synced'
	),
	inserted_student_messages.last_modified_at
FROM inserted_student_messages;
`

func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, initSchemaSQL)
	return err
}
