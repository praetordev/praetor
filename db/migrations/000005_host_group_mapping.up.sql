-- Host-Group mapping table for many-to-many relationship
CREATE TABLE IF NOT EXISTS host_group_mapping (
    host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (host_id, group_id)
);

-- Index for faster lookups
CREATE INDEX IF NOT EXISTS idx_host_group_host ON host_group_mapping(host_id);
CREATE INDEX IF NOT EXISTS idx_host_group_group ON host_group_mapping(group_id);
