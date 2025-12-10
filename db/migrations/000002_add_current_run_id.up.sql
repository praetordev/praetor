-- Add current_run_id to unified_jobs and FK
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='unified_jobs' AND column_name='current_run_id') THEN
        ALTER TABLE unified_jobs ADD COLUMN current_run_id UUID REFERENCES execution_runs(id) ON DELETE SET NULL;
    END IF;
END $$;
