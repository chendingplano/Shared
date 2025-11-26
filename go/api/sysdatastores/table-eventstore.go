package sysdatastores

// Append-only schema with proper indexes
/*
const schema = `
CREATE TABLE events (
    id UUID PRIMARY KEY,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    event_version INT NOT NULL,
    event_data JSONB NOT NULL,
    metadata JSONB,
    occurred_at TIMESTAMP NOT NULL,
    recorded_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Ensure events are ordered per aggregate
    UNIQUE(aggregate_id, event_version),

    -- Indexes for queries
    INDEX idx_aggregate (aggregate_id, event_version),
    INDEX idx_event_type (event_type),
    INDEX idx_occurred_at (occurred_at)
);

-- Global event sequence for ordering
CREATE SEQUENCE IF NOT EXISTS global_event_sequence;
ALTER TABLE events ADD COLUMN global_sequence BIGINT DEFAULT nextval('global_event_sequence');
CREATE INDEX idx_global_sequence ON events(global_sequence);
`
*/