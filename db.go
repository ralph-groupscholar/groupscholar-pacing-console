package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type snapshotStats struct {
	GeneratedAt    time.Time
	RecordCount    int
	TotalAwarded   float64
	TotalDisbursed float64
	Ahead          int
	OnTrack        int
	Behind         int
	Overdue        int
	DueSoon        int
	High           int
	Medium         int
	Low            int
	DueSoonWindow  int
}

func syncToDatabase(items []awardItem, dueSoonDays int, dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GS_PACING_DB_DSN"))
	}
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		return errors.New("GS_PACING_DB_DSN, DATABASE_URL, or -db-url is required for db sync")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	stats := buildSnapshotStats(items, dueSoonDays)

	if err := ensureSchema(ctx, db); err != nil {
		return err
	}

	return insertSnapshot(ctx, db, stats, items)
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE SCHEMA IF NOT EXISTS groupscholar_pacing_console;`,
		`CREATE TABLE IF NOT EXISTS groupscholar_pacing_console.pacing_snapshots (
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
		);`,
		`CREATE TABLE IF NOT EXISTS groupscholar_pacing_console.pacing_awards (
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
		);`,
		`CREATE INDEX IF NOT EXISTS pacing_awards_snapshot_idx ON groupscholar_pacing_console.pacing_awards(snapshot_id);`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func insertSnapshot(ctx context.Context, db *sql.DB, stats snapshotStats, items []awardItem) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var snapshotID int64
	row := tx.QueryRowContext(ctx, `
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
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id;
	`,
		stats.GeneratedAt,
		stats.RecordCount,
		stats.DueSoonWindow,
		stats.TotalAwarded,
		stats.TotalDisbursed,
		stats.Ahead,
		stats.OnTrack,
		stats.Behind,
		stats.Overdue,
		stats.DueSoon,
		stats.High,
		stats.Medium,
		stats.Low,
	)
	if err = row.Scan(&snapshotID); err != nil {
		_ = tx.Rollback()
		return err
	}

	insertStmt, err := tx.PrepareContext(ctx, `
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
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19);
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer insertStmt.Close()

	for _, item := range items {
		record := item.data
		awardDate, _ := parseDateOptional(record.AwardDate)
		targetDate, _ := parseDateOptional(record.TargetDate)
		nextCheckin, _ := parseDateOptional(record.NextCheckin)
		checkinDays := sql.NullInt32{}
		if item.check.Label != "Unscheduled" {
			checkinDays = sql.NullInt32{Int32: int32(item.check.Days), Valid: true}
		}
		_, err = insertStmt.ExecContext(
			ctx,
			snapshotID,
			record.Scholar,
			record.Cohort,
			record.Owner,
			record.Status,
			record.Amount,
			record.DisbursedToDate,
			nullDate(awardDate, record.AwardDate),
			nullDate(targetDate, record.TargetDate),
			nullDate(nextCheckin, record.NextCheckin),
			item.pace.Label,
			item.pace.Delta,
			item.pace.Percent,
			item.pace.Expected,
			item.risk.Level,
			item.risk.Score,
			item.check.Label,
			checkinDays,
			record.Notes,
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("Synced %d awards to Postgres snapshot %d.\n", len(items), snapshotID)
	return nil
}

func nullDate(parsed time.Time, raw string) sql.NullTime {
	if raw == "" || parsed.IsZero() {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: parsed, Valid: true}
}

func buildSnapshotStats(items []awardItem, dueSoonDays int) snapshotStats {
	stats := snapshotStats{
		GeneratedAt:   time.Now(),
		RecordCount:   len(items),
		DueSoonWindow: dueSoonDays,
	}
	for _, item := range items {
		record := item.data
		stats.TotalAwarded += record.Amount
		stats.TotalDisbursed += record.DisbursedToDate
		switch item.pace.Label {
		case "Ahead":
			stats.Ahead++
		case "Behind":
			stats.Behind++
		default:
			stats.OnTrack++
		}
		switch item.check.Label {
		case "Overdue":
			stats.Overdue++
		case "Due Soon":
			stats.DueSoon++
		}
		switch item.risk.Level {
		case "High":
			stats.High++
		case "Medium":
			stats.Medium++
		default:
			stats.Low++
		}
	}
	return stats
}

func loadDataFromDB(dsn string) ([]Disbursement, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("db-url is required to load data from Postgres")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snapshotID int64
	row := db.QueryRowContext(ctx, `
		SELECT id
		FROM groupscholar_pacing_console.pacing_snapshots
		ORDER BY generated_at DESC
		LIMIT 1;
	`)
	if err := row.Scan(&snapshotID); err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT scholar, cohort, owner, status, amount, disbursed_to_date,
			award_date, target_date, next_checkin, notes
		FROM groupscholar_pacing_console.pacing_awards
		WHERE snapshot_id = $1
		ORDER BY scholar ASC;
	`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]Disbursement, 0)
	for rows.Next() {
		var (
			scholar, cohort, owner, status, notes string
			amount, disbursedToDate               float64
			awardDate, targetDate, nextCheckin    sql.NullTime
		)
		if err := rows.Scan(
			&scholar,
			&cohort,
			&owner,
			&status,
			&amount,
			&disbursedToDate,
			&awardDate,
			&targetDate,
			&nextCheckin,
			&notes,
		); err != nil {
			return nil, err
		}
		records = append(records, Disbursement{
			Scholar:         scholar,
			Cohort:          cohort,
			Owner:           owner,
			Status:          status,
			Amount:          amount,
			DisbursedToDate: disbursedToDate,
			AwardDate:       formatNullableDate(awardDate),
			TargetDate:      formatNullableDate(targetDate),
			NextCheckin:     formatNullableDate(nextCheckin),
			Notes:           notes,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func formatNullableDate(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}
