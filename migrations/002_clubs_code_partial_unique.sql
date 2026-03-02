ALTER TABLE clubs
    DROP CONSTRAINT IF EXISTS clubs_code_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_clubs_code_active_unique
    ON clubs (code)
    WHERE deleted_at IS NULL AND code IS NOT NULL;
