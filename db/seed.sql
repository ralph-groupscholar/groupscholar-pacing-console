WITH snapshot AS (
    INSERT INTO groupscholar_pacing_console.pacing_snapshots (
        generated_at,
        record_count,
        due_soon_window,
        total_awarded,
        total_disbursed,
        ahead_count,
        on_track_count,
        behind_count,
        overdue_count,
        due_soon_count,
        high_risk_count,
        medium_risk_count,
        low_risk_count
    ) VALUES (
        '2026-02-08 09:00:00+00',
        6,
        14,
        66500,
        37150,
        0,
        1,
        5,
        1,
        3,
        3,
        3,
        0
    )
    RETURNING id
)
INSERT INTO groupscholar_pacing_console.pacing_awards (
    snapshot_id,
    scholar,
    cohort,
    owner,
    status,
    amount,
    disbursed_to_date,
    award_date,
    target_date,
    next_checkin,
    pace_label,
    pace_delta,
    pace_percent,
    expected_percent,
    risk_level,
    risk_score,
    checkin_label,
    checkin_days,
    notes
)
SELECT * FROM (
    VALUES
        ((SELECT id FROM snapshot), 'Avery Nguyen', 'Spring 2025', 'Maya R.', 'Active', 12000, 7800, '2025-02-15', '2026-02-15', '2026-02-20', 'Behind', -0.3300, 0.6500, 0.9800, 'High', 3, 'Due Soon', 12, 'On track with tuition schedule.'),
        ((SELECT id FROM snapshot), 'Jordan Wells', 'Spring 2025', 'Liam S.', 'Active', 10000, 5200, '2025-03-01', '2026-03-01', '2026-02-10', 'Behind', -0.4200, 0.5200, 0.9400, 'High', 3, 'Due Soon', 2, 'Awaiting spring term invoice.'),
        ((SELECT id FROM snapshot), 'Priya Desai', 'Fall 2024', 'Noah T.', 'Active', 15000, 14250, '2024-08-20', '2025-12-20', '2026-01-25', 'On Track', -0.0500, 0.9500, 1.0000, 'Medium', 2, 'Overdue', -14, 'Final milestone payment queued.'),
        ((SELECT id FROM snapshot), 'Camila Ortiz', 'Fall 2024', 'Jordan P.', 'Active', 9000, 4100, '2024-09-05', '2026-01-05', '2026-02-18', 'Behind', -0.5450, 0.4550, 1.0000, 'High', 3, 'Due Soon', 10, 'Needs documentation for next release.'),
        ((SELECT id FROM snapshot), 'Mateo Silva', 'Summer 2025', 'Rina K.', 'Active', 11000, 3300, '2025-06-10', '2026-06-10', '2026-03-05', 'Behind', -0.3700, 0.3000, 0.6700, 'Medium', 2, 'Scheduled', 25, 'Mid-year internship stipend delivered.'),
        ((SELECT id FROM snapshot), 'Zara Patel', 'Summer 2025', 'Eli G.', 'At Risk', 9500, 2500, '2025-07-01', '2026-07-01', '2026-02-27', 'Behind', -0.3400, 0.2632, 0.6030, 'Medium', 2, 'Scheduled', 19, 'Missing midterm transcript.')
) AS seed (
    snapshot_id,
    scholar,
    cohort,
    owner,
    status,
    amount,
    disbursed_to_date,
    award_date,
    target_date,
    next_checkin,
    pace_label,
    pace_delta,
    pace_percent,
    expected_percent,
    risk_level,
    risk_score,
    checkin_label,
    checkin_days,
    notes
);
