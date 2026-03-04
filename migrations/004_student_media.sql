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
