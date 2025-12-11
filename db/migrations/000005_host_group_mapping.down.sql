-- Rollback: Remove host-group mapping table
DROP INDEX IF EXISTS idx_host_group_group;
DROP INDEX IF EXISTS idx_host_group_host;
DROP TABLE IF EXISTS host_group_mapping;
