-- +goose Up
CREATE SCHEMA IF NOT EXISTS shared;

-- Timestamp types on the existing table are deliberately preserved.
CREATE TABLE IF NOT EXISTS shared.id_mgr (
    id_name VARCHAR(128) PRIMARY KEY,
    id_desc TEXT NOT NULL,
    next_id BIGINT NOT NULL,
    start_id BIGINT NOT NULL DEFAULT 1,
    max_id BIGINT NOT NULL DEFAULT 9223372036854775806,
    allocate_size INTEGER NOT NULL DEFAULT 1,
    allow_reuse BOOLEAN NOT NULL DEFAULT FALSE,
    creator TEXT NOT NULL,
    last_requester TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'shared' AND table_name = 'id_mgr' AND column_name = 'crt_value') THEN
        ALTER TABLE shared.id_mgr RENAME COLUMN crt_value TO next_id;
        -- Legacy allocators issued IDs above crt_value. Skip the entire old block.
        UPDATE shared.id_mgr SET next_id = next_id + 1000;
        ALTER TABLE shared.id_mgr RENAME COLUMN caller_loc TO creator;
        ALTER TABLE shared.id_mgr ALTER COLUMN creator TYPE TEXT;
        ALTER TABLE shared.id_mgr
            ADD COLUMN start_id BIGINT NOT NULL DEFAULT 1,
            ADD COLUMN max_id BIGINT NOT NULL DEFAULT 9223372036854775806,
            ADD COLUMN allocate_size INTEGER NOT NULL DEFAULT 1,
            ADD COLUMN allow_reuse BOOLEAN NOT NULL DEFAULT FALSE,
            ADD COLUMN last_requester TEXT;
        UPDATE shared.id_mgr SET allocate_size = 1000 WHERE id_name = 'activity_log_id';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'shared.id_mgr'::regclass AND conname = 'id_mgr_bounds') THEN
        ALTER TABLE shared.id_mgr ADD CONSTRAINT id_mgr_bounds CHECK (
            btrim(id_name) <> '' AND start_id >= 1 AND max_id >= start_id
            AND max_id <= 9223372036854775806 AND allocate_size >= 1
            AND next_id >= start_id AND next_id <= max_id + 1
            AND (NOT allow_reuse OR next_id <= max_id)
            AND (id_name NOT IN ('global', 'activity_log_id', 'llm_run_id') OR NOT allow_reuse)
        );
    END IF;
END $$;
-- +goose StatementEnd

INSERT INTO shared.id_mgr (id_name, id_desc, next_id, creator)
VALUES ('global', 'IDs shared by all users of this database sequence', 1, 'migration:20261108000001')
ON CONFLICT (id_name) DO NOTHING;

-- +goose Down
-- Preserve all counter history, including global. Do not subtract the conversion
-- offset: cached blocks may have been issued since Up. Old allocators will skip
-- ahead from this conservative high-water mark. A later Up adds another safe gap.
-- Stop all new writers before rolling back, just as old writers must stop for Up.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'shared' AND table_name = 'id_mgr' AND column_name = 'next_id') THEN
        IF EXISTS (SELECT 1 FROM shared.id_mgr WHERE allow_reuse) THEN
            RAISE EXCEPTION 'Cannot roll back ID manager while reusing sequences exist';
        END IF;
        ALTER TABLE shared.id_mgr DROP CONSTRAINT IF EXISTS id_mgr_bounds;
        ALTER TABLE shared.id_mgr DROP COLUMN IF EXISTS last_requester,
            DROP COLUMN IF EXISTS allow_reuse, DROP COLUMN IF EXISTS allocate_size,
            DROP COLUMN IF EXISTS max_id, DROP COLUMN IF EXISTS start_id;
        ALTER TABLE shared.id_mgr RENAME COLUMN next_id TO crt_value;
        ALTER TABLE shared.id_mgr RENAME COLUMN creator TO caller_loc;
        -- Keep TEXT: new requester identities need not fit legacy VARCHAR(20).
    END IF;
END $$;
-- +goose StatementEnd
