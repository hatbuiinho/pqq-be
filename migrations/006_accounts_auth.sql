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
	invitee_email TEXT NOT NULL,
	club_role TEXT NOT NULL,
	token_hash TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	accepted_at TIMESTAMPTZ NULL,
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
