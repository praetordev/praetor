-- Extend the existing prompt-on-launch contract with resource references. The
-- flags are administrator-owned template policy; launch callers can only
-- override a saved resource when its corresponding flag is enabled.
ALTER TABLE job_templates
    ADD COLUMN IF NOT EXISTS ask_inventory_on_launch BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ask_credential_on_launch BOOLEAN NOT NULL DEFAULT false;
