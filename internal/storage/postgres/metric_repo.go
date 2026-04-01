package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Metric represents a metric definition
type Metric struct {
	ID        uuid.UUID      `json:"id"`
	PluginID  uuid.UUID      `json:"plugin_id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"` // counter, gauge, histogram, summary
	Help      string         `json:"help,omitempty"`
	Labels    []string       `json:"labels"`
	CreatedAt time.Time      `json:"created_at"`
}

// MetricRepo provides database operations for metrics
type MetricRepo struct {
	client *Client
}

// NewMetricRepo creates a new metric repository
func NewMetricRepo(client *Client) *MetricRepo {
	return &MetricRepo{client: client}
}

// Create inserts a new metric
func (r *MetricRepo) Create(ctx context.Context, m *Metric) error {
	query := `
		INSERT INTO metrics (id, plugin_id, name, type, help, labels, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (plugin_id, name) DO UPDATE
		SET type = EXCLUDED.type,
		    help = EXCLUDED.help,
		    labels = EXCLUDED.labels
	`

	labelsJSON, err := json.Marshal(m.Labels)
	if err != nil {
		return err
	}

	_, err = r.client.pool.Exec(ctx, query,
		m.ID, m.PluginID, m.Name, m.Type, m.Help, labelsJSON, m.CreatedAt,
	)

	return err
}

// GetByID retrieves a metric by ID
func (r *MetricRepo) GetByID(ctx context.Context, id uuid.UUID) (*Metric, error) {
	query := `
		SELECT id, plugin_id, name, type, help, labels, created_at
		FROM metrics
		WHERE id = $1
	`

	m := &Metric{}
	var labelsJSON []byte

	err := r.client.pool.QueryRow(ctx, query, id).Scan(
		&m.ID, &m.PluginID, &m.Name, &m.Type, &m.Help, &labelsJSON, &m.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(labelsJSON, &m.Labels); err != nil {
		return nil, err
	}

	return m, nil
}

// GetByPluginID retrieves all metrics for a plugin
func (r *MetricRepo) GetByPluginID(ctx context.Context, pluginID uuid.UUID) ([]*Metric, error) {
	query := `
		SELECT id, plugin_id, name, type, help, labels, created_at
		FROM metrics
		WHERE plugin_id = $1
		ORDER BY name
	`

	rows, err := r.client.pool.Query(ctx, query, pluginID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metrics := make([]*Metric, 0)
	for rows.Next() {
		m := &Metric{}
		var labelsJSON []byte

		err := rows.Scan(&m.ID, &m.PluginID, &m.Name, &m.Type, &m.Help, &labelsJSON, &m.CreatedAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(labelsJSON, &m.Labels); err != nil {
			return nil, err
		}

		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// GetByName retrieves a metric by name
func (r *MetricRepo) GetByName(ctx context.Context, name string) (*Metric, error) {
	query := `
		SELECT id, plugin_id, name, type, help, labels, created_at
		FROM metrics
		WHERE name = $1
		LIMIT 1
	`

	m := &Metric{}
	var labelsJSON []byte

	err := r.client.pool.QueryRow(ctx, query, name).Scan(
		&m.ID, &m.PluginID, &m.Name, &m.Type, &m.Help, &labelsJSON, &m.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(labelsJSON, &m.Labels); err != nil {
		return nil, err
	}

	return m, nil
}

// List retrieves all metrics
func (r *MetricRepo) List(ctx context.Context) ([]*Metric, error) {
	query := `
		SELECT id, plugin_id, name, type, help, labels, created_at
		FROM metrics
		ORDER BY name
	`

	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metrics := make([]*Metric, 0)
	for rows.Next() {
		m := &Metric{}
		var labelsJSON []byte

		err := rows.Scan(&m.ID, &m.PluginID, &m.Name, &m.Type, &m.Help, &labelsJSON, &m.CreatedAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(labelsJSON, &m.Labels); err != nil {
			return nil, err
		}

		metrics = append(metrics, m)
	}

	return metrics, rows.Err()
}

// Delete deletes a metric by ID
func (r *MetricRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM metrics WHERE id = $1`
	_, err := r.client.pool.Exec(ctx, query, id)
	return err
}

// DeleteByPluginID deletes all metrics for a plugin
func (r *MetricRepo) DeleteByPluginID(ctx context.Context, pluginID uuid.UUID) error {
	query := `DELETE FROM metrics WHERE plugin_id = $1`
	_, err := r.client.pool.Exec(ctx, query, pluginID)
	return err
}
