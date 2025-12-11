-- Rollback: Remove content column from inventories
ALTER TABLE inventories DROP COLUMN IF EXISTS content;
