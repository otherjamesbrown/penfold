package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindMigrations(t *testing.T) {
	// Create a temporary directory with test migration files
	tmpDir, err := os.MkdirTemp("", "migrations_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test migration files
	files := []string{
		"001_create_users.sql",
		"002_add_email_column.sql",
		"003_create_posts.sql",
		"README.md", // Should be ignored
	}

	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("-- test"), 0644); err != nil {
			t.Fatalf("failed to create test file %s: %v", f, err)
		}
	}

	migrations, err := findMigrations(tmpDir)
	if err != nil {
		t.Fatalf("findMigrations failed: %v", err)
	}

	if len(migrations) != 3 {
		t.Errorf("expected 3 migrations, got %d", len(migrations))
	}

	// Verify order
	expectedVersions := []string{"001_create_users", "002_add_email_column", "003_create_posts"}
	for i, m := range migrations {
		if m.Version != expectedVersions[i] {
			t.Errorf("migration %d: expected version '%s', got '%s'", i, expectedVersions[i], m.Version)
		}
	}
}

func TestFindMigrations_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrations_empty")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	migrations, err := findMigrations(tmpDir)
	if err != nil {
		t.Fatalf("findMigrations failed: %v", err)
	}

	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestFindMigrations_NonExistentDir(t *testing.T) {
	_, err := findMigrations("/nonexistent/path/to/migrations")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestRunMigrations_NilPool(t *testing.T) {
	_, err := RunMigrations(nil, nil, "/tmp")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}

func TestGetPendingMigrations_NilPool(t *testing.T) {
	_, err := GetPendingMigrations(nil, nil, "/tmp")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}
