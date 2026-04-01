package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Filter represents a filter definition
type Filter struct {
	ID          uuid.UUID   `json:"id"`
	PluginID    uuid.UUID   `json:"plugin_id,omitempty"`
	Name        string      `json:"name"`
	Expression  string      `json:"expression"`
	Description string      `json:"description,omitempty"`
	IsDefault   bool        `json:"is_default"`
	CreatedAt   time.Time   `json:"created_at"`
}

// FilterRepo provides database operations for filters
type FilterRepo struct {
	client *Client
}

// NewFilterRepo creates a new filter repository
func NewFilterRepo(client *Client) *FilterRepo {
	return &FilterRepo{client: client}
}

// Create inserts a new filter
func (r *FilterRepo) Create(ctx context.Context, f *Filter) error {
	query := `
		INSERT INTO filters (id, plugin_id, name, expression, description, is_default, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (plugin_id, name) DO UPDATE
		SET expression = EXCLUDED.expression,
		    description = EXCLUDED.description,
		    is_default = EXCLUDED.is_default
	`

	pluginID := pgtype.UUID{}
	if f.PluginID != uuid.Nil {
		pluginID = pgtype.UUID{Bytes: f.PluginID, Valid: true}
	}

	_, err := r.client.pool.Exec(ctx, query,
		f.ID, pluginID, f.Name, f.Expression, f.Description, f.IsDefault, f.CreatedAt,
	)

	return err
}

// GetByID retrieves a filter by ID
func (r *FilterRepo) GetByID(ctx context.Context, id uuid.UUID) (*Filter, error) {
	query := `
		SELECT id, plugin_id, name, expression, description, is_default, created_at
		FROM filters
		WHERE id = $1
	`

	f := &Filter{}
	var pluginID pgtype.UUID

	err := r.client.pool.QueryRow(ctx, query, id).Scan(
		&f.ID, &pluginID, &f.Name, &f.Expression, &f.Description, &f.IsDefault, &f.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if pluginID.Valid {
		f.PluginID = pluginID.Bytes
	}

	return f, err
}

// GetByPluginID retrieves all filters for a plugin
func (r *FilterRepo) GetByPluginID(ctx context.Context, pluginID uuid.UUID) ([]*Filter, error) {
	query := `
		SELECT id, plugin_id, name, expression, description, is_default, created_at
		FROM filters
		WHERE plugin_id = $1
		ORDER BY name
	`

	rows, err := r.client.pool.Query(ctx, query, pluginID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filters := make([]*Filter, 0)
	for rows.Next() {
		f := &Filter{}
		var pid pgtype.UUID

		err := rows.Scan(&f.ID, &pid, &f.Name, &f.Expression, &f.Description, &f.IsDefault, &f.CreatedAt)
		if err != nil {
			return nil, err
		}

		if pid.Valid {
			f.PluginID = pid.Bytes
		}

		filters = append(filters, f)
	}

	return filters, rows.Err()
}

// List retrieves all filters
func (r *FilterRepo) List(ctx context.Context) ([]*Filter, error) {
	query := `
		SELECT id, plugin_id, name, expression, description, is_default, created_at
		FROM filters
		ORDER BY created_at DESC
	`

	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filters := make([]*Filter, 0)
	for rows.Next() {
		f := &Filter{}
		var pluginID pgtype.UUID

		err := rows.Scan(&f.ID, &pluginID, &f.Name, &f.Expression, &f.Description, &f.IsDefault, &f.CreatedAt)
		if err != nil {
			return nil, err
		}

		if pluginID.Valid {
			f.PluginID = pluginID.Bytes
		}

		filters = append(filters, f)
	}

	return filters, rows.Err()
}

// Delete deletes a filter by ID
func (r *FilterRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM filters WHERE id = $1`
	_, err := r.client.pool.Exec(ctx, query, id)
	return err
}

// DeleteByPluginID deletes all filters for a plugin
func (r *FilterRepo) DeleteByPluginID(ctx context.Context, pluginID uuid.UUID) error {
	query := `DELETE FROM filters WHERE plugin_id = $1`
	_, err := r.client.pool.Exec(ctx, query, pluginID)
	return err
}

// GetDefaults retrieves all default filters
func (r *FilterRepo) GetDefaults(ctx context.Context) ([]*Filter, error) {
	query := `
		SELECT id, plugin_id, name, expression, description, is_default, created_at
		FROM filters
		WHERE is_default = true
		ORDER BY name
	`

	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filters := make([]*Filter, 0)
	for rows.Next() {
		f := &Filter{}
		var pluginID pgtype.UUID

		err := rows.Scan(&f.ID, &pluginID, &f.Name, &f.Expression, &f.Description, &f.IsDefault, &f.CreatedAt)
		if err != nil {
			return nil, err
		}

		if pluginID.Valid {
			f.PluginID = pluginID.Bytes
		}

		filters = append(filters, f)
	}

	return filters, rows.Err()
}
