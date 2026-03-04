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
