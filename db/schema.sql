CREATE SCHEMA IF NOT EXISTS groupscholar_pacing_console;

CREATE TABLE IF NOT EXISTS groupscholar_pacing_console.pacing_snapshots (
    id BIGSERIAL PRIMARY KEY,
    generated_at TIMESTAMPTZ NOT NULL,
    record_count INT NOT NULL,
    due_soon_window INT NOT NULL,
    total_awarded NUMERIC(12,2) NOT NULL,
    total_disbursed NUMERIC(12,2) NOT NULL,
    ahead_count INT NOT NULL,
    on_track_count INT NOT NULL,
    behind_count INT NOT NULL,
    overdue_count INT NOT NULL,
    due_soon_count INT NOT NULL,
    high_risk_count INT NOT NULL,
    medium_risk_count INT NOT NULL,
    low_risk_count INT NOT NULL
);

CREATE TABLE IF NOT EXISTS groupscholar_pacing_console.pacing_awards (
    id BIGSERIAL PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES groupscholar_pacing_console.pacing_snapshots(id) ON DELETE CASCADE,
    scholar TEXT NOT NULL,
    cohort TEXT NOT NULL,
    owner TEXT NOT NULL,
    status TEXT NOT NULL,
    amount NUMERIC(12,2) NOT NULL,
    disbursed_to_date NUMERIC(12,2) NOT NULL,
    award_date DATE,
    target_date DATE,
    next_checkin DATE,
    pace_label TEXT NOT NULL,
    pace_delta NUMERIC(8,4) NOT NULL,
    pace_percent NUMERIC(8,4) NOT NULL,
    expected_percent NUMERIC(8,4) NOT NULL,
    risk_level TEXT NOT NULL,
    risk_score INT NOT NULL,
    checkin_label TEXT NOT NULL,
    checkin_days INT,
    notes TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS pacing_awards_snapshot_idx
    ON groupscholar_pacing_console.pacing_awards(snapshot_id);
