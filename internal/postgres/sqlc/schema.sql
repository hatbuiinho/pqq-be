CREATE TABLE clubs (
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

CREATE TABLE club_groups (
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

CREATE TABLE club_schedules (
	id TEXT PRIMARY KEY,
	club_id TEXT NOT NULL REFERENCES clubs(id),
	weekday TEXT NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE belt_ranks (
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

CREATE TABLE students (
	id TEXT PRIMARY KEY,
	student_code TEXT NULL,
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

CREATE TABLE student_schedule_profiles (
	id TEXT PRIMARY KEY,
	student_id TEXT NOT NULL UNIQUE REFERENCES students(id),
	mode TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE student_schedules (
	id TEXT PRIMARY KEY,
	student_id TEXT NOT NULL REFERENCES students(id),
	weekday TEXT NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_modified_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE attendance_sessions (
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

CREATE TABLE attendance_records (
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

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL,
	full_name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	system_role TEXT NOT NULL DEFAULT 'user',
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	last_login_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE club_memberships (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	club_id TEXT NOT NULL REFERENCES clubs(id),
	club_role TEXT NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ NULL
);

CREATE TABLE club_invites (
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
	updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE audit_logs (
	id TEXT PRIMARY KEY,
	actor_user_id TEXT NULL REFERENCES users(id),
	club_id TEXT NULL REFERENCES clubs(id),
	entity_type TEXT NOT NULL,
	entity_id TEXT NULL,
	action TEXT NOT NULL,
	old_values JSONB NULL,
	new_values JSONB NULL,
	metadata JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL
);
