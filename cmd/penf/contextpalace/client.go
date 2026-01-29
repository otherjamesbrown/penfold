// Package contextpalace provides a client for Context-Palace database operations.
package contextpalace

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/lib/pq"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// Client provides Context-Palace database operations.
type Client struct {
	db      *sql.DB
	config  *config.ContextPalaceConfig
	project string
	agent   string
}

// CommandEntry represents a CLI command log entry.
type CommandEntry struct {
	ID           int64     `json:"id"`
	Project      string    `json:"project"`
	Agent        string    `json:"agent"`
	Command      string    `json:"command"`
	Args         []string  `json:"args"`
	FullCommand  string    `json:"full_command"`
	DurationMs   int       `json:"duration_ms"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Response     string    `json:"response,omitempty"`
	TenantID     string    `json:"tenant_id,omitempty"`
	Hostname     string    `json:"hostname,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// NewClient creates a new Context-Palace client from configuration.
func NewClient(cfg *config.ContextPalaceConfig) (*Client, error) {
	if cfg == nil || !cfg.IsConfigured() {
		return nil, fmt.Errorf("context-palace not configured")
	}

	connStr := cfg.ConnectionString()
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &Client{
		db:      db,
		config:  cfg,
		project: cfg.GetProject(),
		agent:   cfg.GetAgent(),
	}, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping checks the database connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// LogCommand logs a CLI command execution to Context-Palace.
func (c *Client) LogCommand(ctx context.Context, entry *CommandEntry) error {
	// Use provided values or defaults from config.
	project := entry.Project
	if project == "" {
		project = c.project
	}
	agent := entry.Agent
	if agent == "" {
		agent = c.agent
	}

	// Get hostname if not provided.
	hostname := entry.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	// Truncate response and error message to 500 chars (server does this too, but be safe).
	response := truncate(entry.Response, 500)
	errorMsg := truncate(entry.ErrorMessage, 500)

	query := `SELECT log_cli_command($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := c.db.ExecContext(ctx, query,
		project,
		agent,
		entry.Command,
		pq.Array(entry.Args),
		entry.FullCommand,
		entry.DurationMs,
		entry.Success,
		nullIfEmpty(errorMsg),
		nullIfEmpty(response),
		nullIfEmpty(entry.TenantID),
		nullIfEmpty(hostname),
	)
	if err != nil {
		return fmt.Errorf("logging command: %w", err)
	}

	return nil
}

// History retrieves recent CLI command history from Context-Palace.
func (c *Client) History(ctx context.Context, agent string, limit int) ([]CommandEntry, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT * FROM cli_history($1, $2, $3)`

	rows, err := c.db.QueryContext(ctx, query, c.project, nullIfEmpty(agent), limit)
	if err != nil {
		return nil, fmt.Errorf("querying history: %w", err)
	}
	defer rows.Close()

	var entries []CommandEntry
	for rows.Next() {
		var e CommandEntry
		var errorMsg sql.NullString

		// cli_history returns: id, agent, command, full_command, duration_ms, success, error_message, created_at
		err := rows.Scan(
			&e.ID,
			&e.Agent,
			&e.Command,
			&e.FullCommand,
			&e.DurationMs,
			&e.Success,
			&errorMsg,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		e.Project = c.project
		e.ErrorMessage = errorMsg.String

		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return entries, nil
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// nullIfEmpty returns nil if s is empty, otherwise returns s.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
