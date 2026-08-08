CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_by TEXT,
    lease_until TIMESTAMPTZ,
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT jobs_type_not_blank
        CHECK (btrim(type) <> ''),

    CONSTRAINT jobs_status_valid
        CHECK (status IN (
            'queued',
            'running',
            'succeeded',
            'failed'
        )),

    CONSTRAINT jobs_attempt_non_negative
        CHECK (attempt >= 0),

    CONSTRAINT jobs_max_attempts_positive
        CHECK (max_attempts > 0),

    CONSTRAINT jobs_attempt_within_limit
        CHECK (attempt <= max_attempts)
);