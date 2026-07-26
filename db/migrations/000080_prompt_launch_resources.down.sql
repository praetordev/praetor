ALTER TABLE job_templates
    DROP COLUMN IF EXISTS ask_credential_on_launch,
    DROP COLUMN IF EXISTS ask_inventory_on_launch;
