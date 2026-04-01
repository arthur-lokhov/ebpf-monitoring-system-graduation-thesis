package postgres

import (
	"context"
	"time"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Dashboard represents a dashboard configuration
type Dashboard struct {
	ID          uuid.UUID          `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Config      DashboardConfig    `json:"config"`
	IsDefault   bool               `json:"is_default"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// DashboardConfig represents the dashboard configuration
type DashboardConfig struct {
	Version int              `json:"version"`
	Panels  []DashboardPanel `json:"panels"`
	Layout  string           `json:"layout"`
}

// DashboardPanel represents a single panel in a dashboard
type DashboardPanel struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Type        string                 `json:"type"` // graph, stat, table, heatmap
	Queries     []DashboardQuery       `json:"queries"`
	Options     map[string]interface{} `json:"options,omitempty"`
	GridPos     GridPosition           `json:"gridPos"`
}

// GridPosition represents the position of a panel in the grid
type GridPosition struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"w"`
	Height int `json:"h"`
}

// DashboardQuery represents a query in a panel
type DashboardQuery struct {
	ID         string `json:"id"`
	Expression string `json:"expression"`
	Legend     string `json:"legend,omitempty"`
	Format     string `json:"format"` // time_series, table
}

// DashboardRepo provides database operations for dashboards
type DashboardRepo struct {
	client *Client
}

// NewDashboardRepo creates a new dashboard repository
func NewDashboardRepo(client *Client) *DashboardRepo {
	return &DashboardRepo{client: client}
}

// Create inserts a new dashboard
func (r *DashboardRepo) Create(ctx context.Context, d *Dashboard) error {
	query := `
		INSERT INTO dashboards (id, name, description, config, is_default, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	configJSON, err := json.Marshal(d.Config)
	if err != nil {
		return err
	}

	_, err = r.client.pool.Exec(ctx, query,
		d.ID, d.Name, d.Description, configJSON, d.IsDefault, d.CreatedAt, d.UpdatedAt,
	)

	return err
}

// GetByID retrieves a dashboard by ID
func (r *DashboardRepo) GetByID(ctx context.Context, id uuid.UUID) (*Dashboard, error) {
	query := `
		SELECT id, name, description, config, is_default, created_at, updated_at
		FROM dashboards
		WHERE id = $1
	`

	d := &Dashboard{}
	var configJSON []byte

	err := r.client.pool.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.Name, &d.Description, &configJSON, &d.IsDefault, &d.CreatedAt, &d.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(configJSON, &d.Config); err != nil {
		return nil, err
	}

	return d, nil
}

// GetDefault retrieves the default dashboard
func (r *DashboardRepo) GetDefault(ctx context.Context) (*Dashboard, error) {
	query := `
		SELECT id, name, description, config, is_default, created_at, updated_at
		FROM dashboards
		WHERE is_default = true
		LIMIT 1
	`

	d := &Dashboard{}
	var configJSON []byte

	err := r.client.pool.QueryRow(ctx, query).Scan(
		&d.ID, &d.Name, &d.Description, &configJSON, &d.IsDefault, &d.CreatedAt, &d.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(configJSON, &d.Config); err != nil {
		return nil, err
	}

	return d, nil
}

// List retrieves all dashboards
func (r *DashboardRepo) List(ctx context.Context) ([]*Dashboard, error) {
	query := `
		SELECT id, name, description, config, is_default, created_at, updated_at
		FROM dashboards
		ORDER BY created_at DESC
	`

	rows, err := r.client.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dashboards := make([]*Dashboard, 0)
	for rows.Next() {
		d := &Dashboard{}
		var configJSON []byte

		err := rows.Scan(&d.ID, &d.Name, &d.Description, &configJSON, &d.IsDefault, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(configJSON, &d.Config); err != nil {
			return nil, err
		}

		dashboards = append(dashboards, d)
	}

	return dashboards, rows.Err()
}

// Update updates an existing dashboard
func (r *DashboardRepo) Update(ctx context.Context, d *Dashboard) error {
	query := `
		UPDATE dashboards
		SET name = $1, description = $2, config = $3, is_default = $4, updated_at = NOW()
		WHERE id = $5
	`

	configJSON, err := json.Marshal(d.Config)
	if err != nil {
		return err
	}

	_, err = r.client.pool.Exec(ctx, query,
		d.Name, d.Description, configJSON, d.IsDefault, d.ID,
	)

	return err
}

// Delete deletes a dashboard by ID
func (r *DashboardRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM dashboards WHERE id = $1`
	_, err := r.client.pool.Exec(ctx, query, id)
	return err
}

// Save persists a dashboard (create or update)
func (r *DashboardRepo) Save(ctx context.Context, d *Dashboard) error {
	existing, err := r.GetByID(ctx, d.ID)
	if err != nil {
		return err
	}

	if existing != nil {
		return r.Update(ctx, d)
	}

	return r.Create(ctx, d)
}
