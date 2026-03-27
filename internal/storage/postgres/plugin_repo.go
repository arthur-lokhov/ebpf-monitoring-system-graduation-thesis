package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Plugin represents a loaded plugin
type Plugin struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description,omitempty"`
	Author       string         `json:"author,omitempty"`
	GitURL       string         `json:"git_url"`
	GitCommit    string         `json:"git_commit,omitempty"`
	GitBranch    string         `json:"git_branch,omitempty"`
	EBPFS3Key    string         `json:"ebpf_s3_key,omitempty"`
	WASMS3Key    string         `json:"wasm_s3_key,omitempty"`
	Manifest     map[string]any `json:"manifest"`
	Status       string         `json:"status"`
	BuildLog     pgtype.Text    `json:"build_log,omitempty"`
	ErrorMessage pgtype.Text    `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// PluginStatus represents plugin status
type PluginStatus string

const (
	PluginStatusPending  PluginStatus = "pending"
	PluginStatusBuilding PluginStatus = "building"
	PluginStatusReady    PluginStatus = "ready"
	PluginStatusError    PluginStatus = "error"
)

// PluginRepo provides database operations for plugins
type PluginRepo struct {
	client *Client
}

// NewPluginRepo creates a new plugin repository
func NewPluginRepo(client *Client) *PluginRepo {
	return &PluginRepo{client: client}
}

// Create inserts a new plugin
func (r *PluginRepo) Create(ctx context.Context, p *Plugin) error {
	query := `
		INSERT INTO plugins (
			id, name, version, description, author,
			git_url, git_commit, git_branch,
			ebpf_s3_key, wasm_s3_key, manifest, status,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
	`

	_, err := r.client.pool.Exec(ctx, query,
		p.ID, p.Name, p.Version, p.Description, p.Author,
		p.GitURL, p.GitCommit, p.GitBranch,
		p.EBPFS3Key, p.WASMS3Key, p.Manifest, p.Status,
		p.CreatedAt, p.UpdatedAt,
	)

	return err
}

// GetByID retrieves a plugin by ID
func (r *PluginRepo) GetByID(ctx context.Context, id uuid.UUID) (*Plugin, error) {
	query := `
		SELECT id, name, version, description, author,
		       git_url, git_commit, git_branch,
		       ebpf_s3_key, wasm_s3_key, manifest,
		       status, build_log, error_message,
		       created_at, updated_at
		FROM plugins
		WHERE id = $1
	`

	p := &Plugin{}
	var buildLog pgtype.Text
	var errorMsg pgtype.Text
	
	err := r.client.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Version, &p.Description, &p.Author,
		&p.GitURL, &p.GitCommit, &p.GitBranch,
		&p.EBPFS3Key, &p.WASMS3Key, &p.Manifest,
		&p.Status, &buildLog, &errorMsg,
		&p.CreatedAt, &p.UpdatedAt,
	)
	
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	
	p.BuildLog = buildLog
	p.ErrorMessage = errorMsg

	return p, err
}

// GetByName retrieves a plugin by name
func (r *PluginRepo) GetByName(ctx context.Context, name string) (*Plugin, error) {
	query := `
		SELECT id, name, version, description, author,
		       git_url, git_commit, git_branch,
		       ebpf_s3_key, wasm_s3_key, manifest,
		       status, build_log, error_message,
		       created_at, updated_at
		FROM plugins
		WHERE name = $1
	`

	p := &Plugin{}
	var buildLog pgtype.Text
	var errorMsg pgtype.Text
	
	err := r.client.pool.QueryRow(ctx, query, name).Scan(
		&p.ID, &p.Name, &p.Version, &p.Description, &p.Author,
		&p.GitURL, &p.GitCommit, &p.GitBranch,
		&p.EBPFS3Key, &p.WASMS3Key, &p.Manifest,
		&p.Status, &buildLog, &errorMsg,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	
	p.BuildLog = buildLog
	p.ErrorMessage = errorMsg

	return p, err
}

// List retrieves all plugins with optional status filter
func (r *PluginRepo) List(ctx context.Context, status *PluginStatus) ([]*Plugin, error) {
	query := `
		SELECT id, name, version, description, author,
		       git_url, git_commit, git_branch,
		       ebpf_s3_key, wasm_s3_key, manifest,
		       status, build_log, error_message,
		       created_at, updated_at
		FROM plugins
		ORDER BY created_at DESC
	`

	var rows pgx.Rows
	var err error

	if status != nil {
		query += " WHERE status = $1"
		rows, err = r.client.pool.Query(ctx, query, *status)
	} else {
		rows, err = r.client.pool.Query(ctx, query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plugins := make([]*Plugin, 0)
	for rows.Next() {
		p := &Plugin{}
		var buildLog pgtype.Text
		var errorMsg pgtype.Text
		
		err := rows.Scan(
			&p.ID, &p.Name, &p.Version, &p.Description, &p.Author,
			&p.GitURL, &p.GitCommit, &p.GitBranch,
			&p.EBPFS3Key, &p.WASMS3Key, &p.Manifest,
			&p.Status, &buildLog, &errorMsg,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.BuildLog = buildLog
		p.ErrorMessage = errorMsg
		plugins = append(plugins, p)
	}

	return plugins, rows.Err()
}

// Update updates an existing plugin
func (r *PluginRepo) Update(ctx context.Context, p *Plugin) error {
	query := `
		UPDATE plugins
		SET name = $1, version = $2, description = $3, author = $4,
		    git_commit = $5, git_branch = $6,
		    ebpf_s3_key = $7, wasm_s3_key = $8,
		    manifest = $9, status = $10,
		    build_log = $11, error_message = $12,
		    updated_at = NOW()
		WHERE id = $13
	`

	_, err := r.client.pool.Exec(ctx, query,
		p.Name, p.Version, p.Description, p.Author,
		p.GitCommit, p.GitBranch,
		p.EBPFS3Key, p.WASMS3Key,
		p.Manifest, p.Status,
		p.BuildLog, p.ErrorMessage,
		p.ID,
	)

	return err
}

// UpdateStatus updates only the status and related fields
func (r *PluginRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status PluginStatus, buildLog, errorMessage string) error {
	query := `
		UPDATE plugins
		SET status = $1,
		    build_log = $2,
		    error_message = $3,
		    updated_at = NOW()
		WHERE id = $4
	`

	_, err := r.client.pool.Exec(ctx, query, status, buildLog, errorMessage, id)
	return err
}

// Delete deletes a plugin by ID
func (r *PluginRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM plugins WHERE id = $1`
	_, err := r.client.pool.Exec(ctx, query, id)
	return err
}

// Exists checks if a plugin exists by name
func (r *PluginRepo) Exists(ctx context.Context, name string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM plugins WHERE name = $1)`
	var exists bool
	err := r.client.pool.QueryRow(ctx, query, name).Scan(&exists)
	return exists, err
}
