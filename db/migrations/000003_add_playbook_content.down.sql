-- Rollback: Remove playbook_content column from job_templates
ALTER TABLE job_templates DROP COLUMN IF EXISTS playbook_content;
