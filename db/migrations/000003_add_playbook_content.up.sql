-- Add playbook_content column to job_templates for inline playbook YAML
ALTER TABLE job_templates ADD COLUMN IF NOT EXISTS playbook_content TEXT;

-- Comment: playbook_content allows storing inline playbook YAML directly in the template
-- If project_id is set, the Git-synced playbook takes priority over playbook_content
