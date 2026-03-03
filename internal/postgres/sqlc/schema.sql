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
