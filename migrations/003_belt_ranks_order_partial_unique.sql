ALTER TABLE belt_ranks
    DROP CONSTRAINT IF EXISTS belt_ranks_rank_order_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_belt_ranks_rank_order_active_unique
    ON belt_ranks (rank_order)
    WHERE deleted_at IS NULL;
