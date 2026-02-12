// //////////////////////////////////////////////////////////
//
// Description:
// Define the metrics for table-syncher.
//
// Created: 2026/02/26 by Claude Code based on Documents/syncdata-v2.md
// //////////////////////////////////////////////////////////
package tablesyncher

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Location codes for metrics operations
const (
	LOC_METRICS_AGG   = "SHD_SYN_070"
	LOC_METRICS_QUERY = "SHD_SYN_071"
)

// Period types for metrics aggregation
const (
	PeriodTypeFreq  = "FREQ" // Per sync frequency period
	PeriodTypeWeek  = "WEEK"
	PeriodTypeMonth = "MONTH"
)

// MetricsAggregator handles periodic metrics aggregation.
type MetricsAggregator struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewMetricsAggregator creates a new metrics aggregator.
func NewMetricsAggregator(db *sql.DB, logger *slog.Logger) *MetricsAggregator {
	return &MetricsAggregator{
		db:     db,
		logger: logger,
	}
}

// AggregateMetrics computes and stores metrics for the specified period.
func (m *MetricsAggregator) AggregateMetrics(ctx context.Context, periodType string, periodStart, periodEnd time.Time) error {
	// Get list of tables that have been synced
	tables, err := m.getTablesWithActivity(ctx, periodStart, periodEnd)
	if err != nil {
		return err
	}

	for _, tableName := range tables {
		if err := m.aggregateTableMetrics(ctx, tableName, periodType, periodStart, periodEnd); err != nil {
			m.logger.Error("Failed to aggregate metrics for table",
				"table", tableName,
				"period_type", periodType,
				"error", err,
				"loc", LOC_METRICS_AGG)
			// Continue with other tables
		}
	}

	m.logger.Info("Metrics aggregation complete",
		"period_type", periodType,
		"period_start", periodStart,
		"period_end", periodEnd,
		"tables", len(tables),
		"loc", LOC_METRICS_AGG)

	return nil
}

// getTablesWithActivity returns tables that have sync activity in the period.
func (m *MetricsAggregator) getTablesWithActivity(ctx context.Context, start, end time.Time) ([]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT DISTINCT table_name FROM data_sync_logs
		 WHERE sync_time >= $1 AND sync_time < $2`,
		start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables with activity: %w (%s) (SHD_02070604)", err, LOC_METRICS_QUERY)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w (%s) (SHD_02070605)", err, LOC_METRICS_QUERY)
		}
		tables = append(tables, name)
	}

	return tables, rows.Err()
}

// aggregateTableMetrics computes metrics for a single table.
func (m *MetricsAggregator) aggregateTableMetrics(ctx context.Context, tableName, periodType string, start, end time.Time) error {
	// Count records by operation type
	// Note: This is a simplified aggregation. In practice, you'd parse the
	// archive_ref or add operation_type to data_sync_logs for accurate counts.

	var totalRows sql.NullInt64
	err := m.db.QueryRowContext(ctx,
		`SELECT SUM(rows_synced) FROM data_sync_logs
		 WHERE table_name = $1 AND sync_time >= $2 AND sync_time < $3 AND status = 'SUCCESS'`,
		tableName, start, end).Scan(&totalRows)
	if err != nil {
		return fmt.Errorf("failed to sum rows synced: %w (SHD_02070606)", err)
	}

	// For now, we'll estimate the distribution
	// In a real implementation, track operation types in data_sync_logs
	total := int64(0)
	if totalRows.Valid {
		total = totalRows.Int64
	}

	// Insert or update metrics
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO data_sync_metrics (table_name, period_start, period_end, period_type, records_added, records_updated, records_deleted)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (table_name, period_start, period_type) DO UPDATE SET
		     period_end = EXCLUDED.period_end,
		     records_added = EXCLUDED.records_added,
		     records_updated = EXCLUDED.records_updated,
		     records_deleted = EXCLUDED.records_deleted`,
		tableName, start, end, periodType,
		total, 0, 0) // Simplified: all counted as adds
	if err != nil {
		return fmt.Errorf("failed to upsert metrics: %w (SHD_02070607)", err)
	}

	return nil
}

// AggregateFrequencyPeriod aggregates metrics for the last sync frequency period.
func (m *MetricsAggregator) AggregateFrequencyPeriod(ctx context.Context, freqSeconds int) error {
	now := time.Now()
	periodEnd := now.Truncate(time.Duration(freqSeconds) * time.Second)
	periodStart := periodEnd.Add(-time.Duration(freqSeconds) * time.Second)

	return m.AggregateMetrics(ctx, PeriodTypeFreq, periodStart, periodEnd)
}

// AggregateWeekly aggregates metrics for the previous week.
func (m *MetricsAggregator) AggregateWeekly(ctx context.Context) error {
	now := time.Now()
	// Find start of current week (Sunday)
	weekday := int(now.Weekday())
	startOfWeek := now.AddDate(0, 0, -weekday).Truncate(24 * time.Hour)
	// Previous week
	periodEnd := startOfWeek
	periodStart := periodEnd.AddDate(0, 0, -7)

	return m.AggregateMetrics(ctx, PeriodTypeWeek, periodStart, periodEnd)
}

// AggregateMonthly aggregates metrics for the previous month.
func (m *MetricsAggregator) AggregateMonthly(ctx context.Context) error {
	now := time.Now()
	// Start of current month
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	// Previous month
	periodEnd := startOfMonth
	periodStart := periodEnd.AddDate(0, -1, 0)

	return m.AggregateMetrics(ctx, PeriodTypeMonth, periodStart, periodEnd)
}

// GetMetrics retrieves metrics for a table and period type.
func (m *MetricsAggregator) GetMetrics(ctx context.Context, tableName, periodType string, limit int) ([]SyncMetric, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, table_name, period_start, period_end, period_type, records_added, records_updated, records_deleted
		 FROM data_sync_metrics
		 WHERE table_name = $1 AND period_type = $2
		 ORDER BY period_start DESC
		 LIMIT $3`,
		tableName, periodType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w (%s) (SHD_02070608)", err, LOC_METRICS_QUERY)
	}
	defer rows.Close()

	var metrics []SyncMetric
	for rows.Next() {
		var m SyncMetric
		if err := rows.Scan(&m.ID, &m.TableName, &m.PeriodStart, &m.PeriodEnd, &m.PeriodType,
			&m.RecordsAdded, &m.RecordsUpdated, &m.RecordsDeleted); err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w (%s) (SHD_02070609)", err, LOC_METRICS_QUERY)
		}
		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// GetAllTableMetrics retrieves recent metrics for all tables.
func (m *MetricsAggregator) GetAllTableMetrics(ctx context.Context, periodType string, limit int) ([]SyncMetric, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, table_name, period_start, period_end, period_type, records_added, records_updated, records_deleted
		 FROM data_sync_metrics
		 WHERE period_type = $1
		 ORDER BY period_start DESC
		 LIMIT $2`,
		periodType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get all metrics: %w (%s) (SHD_02070610)", err, LOC_METRICS_QUERY)
	}
	defer rows.Close()

	var metrics []SyncMetric
	for rows.Next() {
		var m SyncMetric
		if err := rows.Scan(&m.ID, &m.TableName, &m.PeriodStart, &m.PeriodEnd, &m.PeriodType,
			&m.RecordsAdded, &m.RecordsUpdated, &m.RecordsDeleted); err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w (%s) (SHD_02070611)", err, LOC_METRICS_QUERY)
		}
		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// CleanupOldMetrics removes metrics older than the specified duration.
func (m *MetricsAggregator) CleanupOldMetrics(ctx context.Context, retainDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retainDays)

	result, err := m.db.ExecContext(ctx,
		`DELETE FROM data_sync_metrics WHERE period_end < $1`,
		cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old metrics: %w (%s) (SHD_02070612)", err, LOC_METRICS_AGG)
	}

	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		m.logger.Info("Cleaned up old metrics",
			"deleted", deleted,
			"cutoff", cutoff,
			"loc", LOC_METRICS_AGG)
	}

	return deleted, nil
}

// CleanupOldLogs removes sync logs older than the specified duration.
func (m *MetricsAggregator) CleanupOldLogs(ctx context.Context, retainDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retainDays)

	result, err := m.db.ExecContext(ctx,
		`DELETE FROM data_sync_logs WHERE sync_time < $1`,
		cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old logs: %w (%s) (SHD_02070613)", err, LOC_METRICS_AGG)
	}

	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		m.logger.Info("Cleaned up old logs",
			"deleted", deleted,
			"cutoff", cutoff,
			"loc", LOC_METRICS_AGG)
	}

	return deleted, nil
}
