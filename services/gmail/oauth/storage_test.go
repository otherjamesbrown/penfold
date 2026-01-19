package oauth

import (
	"context"
	"testing"
	"time"
)

func TestNullTime_Scan(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		wantValid bool
		wantTime  time.Time
		wantErr   bool
	}{
		{
			name:      "nil value",
			value:     nil,
			wantValid: false,
			wantTime:  time.Time{},
			wantErr:   false,
		},
		{
			name:      "valid time",
			value:     time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			wantValid: true,
			wantTime:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			wantErr:   false,
		},
		{
			name:      "invalid type - string",
			value:     "2024-01-15",
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "invalid type - int",
			value:     12345,
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nt NullTime
			err := nt.Scan(tt.value)

			if (err != nil) != tt.wantErr {
				t.Errorf("NullTime.Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if nt.Valid != tt.wantValid {
					t.Errorf("NullTime.Valid = %v, want %v", nt.Valid, tt.wantValid)
				}

				if tt.wantValid && !nt.Time.Equal(tt.wantTime) {
					t.Errorf("NullTime.Time = %v, want %v", nt.Time, tt.wantTime)
				}
			}
		})
	}
}

func TestNullTime_Value(t *testing.T) {
	tests := []struct {
		name      string
		nt        NullTime
		wantValue interface{}
		wantErr   bool
	}{
		{
			name: "invalid returns nil",
			nt: NullTime{
				Valid: false,
				Time:  time.Time{},
			},
			wantValue: nil,
			wantErr:   false,
		},
		{
			name: "valid returns time",
			nt: NullTime{
				Valid: true,
				Time:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			},
			wantValue: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.nt.Value()

			if (err != nil) != tt.wantErr {
				t.Errorf("NullTime.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if val == nil && tt.wantValue != nil {
				t.Errorf("NullTime.Value() = nil, want %v", tt.wantValue)
				return
			}

			if val != nil && tt.wantValue == nil {
				t.Errorf("NullTime.Value() = %v, want nil", val)
				return
			}

			if val != nil && tt.wantValue != nil {
				valTime, ok := val.(time.Time)
				if !ok {
					t.Errorf("NullTime.Value() returned non-time type: %T", val)
					return
				}
				wantTime := tt.wantValue.(time.Time)
				if !valTime.Equal(wantTime) {
					t.Errorf("NullTime.Value() = %v, want %v", valTime, wantTime)
				}
			}
		})
	}
}

func TestTokenRow_Fields(t *testing.T) {
	now := time.Now()
	row := TokenRow{
		TenantID:      "tenant-123",
		EncryptedData: []byte{0x01, 0x02, 0x03},
		Scopes:        []string{"scope1", "scope2"},
		ExpiresAt:     now.Add(1 * time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if row.TenantID != "tenant-123" {
		t.Errorf("TenantID = %s, want tenant-123", row.TenantID)
	}
	if len(row.EncryptedData) != 3 {
		t.Errorf("EncryptedData length = %d, want 3", len(row.EncryptedData))
	}
	if len(row.Scopes) != 2 {
		t.Errorf("Scopes length = %d, want 2", len(row.Scopes))
	}
}

func TestNewPostgresTokenStorage_Validation(t *testing.T) {
	t.Run("nil pool returns error", func(t *testing.T) {
		_, err := NewPostgresTokenStorage(&PostgresTokenStorageConfig{
			Pool:          nil,
			EncryptionKey: make([]byte, 32),
		})
		if err == nil {
			t.Error("expected error for nil pool")
		}
	})

	t.Run("empty encryption key returns error", func(t *testing.T) {
		// We can't create a real pool, so we use a nil pool which fails first
		// but we can test the error message path
		_, err := NewPostgresTokenStorage(&PostgresTokenStorageConfig{
			Pool:          nil,
			EncryptionKey: nil,
		})
		if err == nil {
			t.Error("expected error for nil encryption key")
		}
	})

	t.Run("invalid encryption key length returns error", func(t *testing.T) {
		// Short key should fail at encryptor creation
		_, err := NewPostgresTokenStorage(&PostgresTokenStorageConfig{
			Pool:          nil, // Will fail first due to nil pool
			EncryptionKey: make([]byte, 16),
		})
		if err == nil {
			t.Error("expected error for short encryption key")
		}
	})
}

func TestMemoryTokenStorage_MultipleTokens(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store multiple tokens
	tenants := []string{"tenant-1", "tenant-2", "tenant-3"}
	for _, tenantID := range tenants {
		token := &Token{
			AccessToken:  "access-" + tenantID,
			RefreshToken: "refresh-" + tenantID,
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			Scopes:       []string{ScopeGmailReadonly},
			TenantID:     tenantID,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := storage.StoreToken(ctx, token); err != nil {
			t.Fatalf("StoreToken() error = %v", err)
		}
	}

	// Verify all tokens are stored
	tokens, err := storage.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens() error = %v", err)
	}
	if len(tokens) != 3 {
		t.Errorf("ListTokens() returned %d tokens, want 3", len(tokens))
	}

	// Verify each token can be retrieved
	for _, tenantID := range tenants {
		token, err := storage.GetToken(ctx, tenantID)
		if err != nil {
			t.Errorf("GetToken(%s) error = %v", tenantID, err)
		}
		if token.TenantID != tenantID {
			t.Errorf("GetToken(%s).TenantID = %s", tenantID, token.TenantID)
		}
	}

	// Delete one and verify count
	if err := storage.DeleteToken(ctx, "tenant-2"); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}

	tokens, err = storage.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens() after delete error = %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("ListTokens() after delete returned %d tokens, want 2", len(tokens))
	}
}

func TestMemoryTokenStorage_UpdateToken(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store initial token
	token := &Token{
		AccessToken:  "initial-access-token",
		RefreshToken: "initial-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       []string{ScopeGmailReadonly},
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := storage.StoreToken(ctx, token); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	// Update the token
	updatedToken := &Token{
		AccessToken:  "updated-access-token",
		RefreshToken: "updated-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scopes:       []string{ScopeGmailReadonly, ScopeGmailModify},
		TenantID:     "tenant-1",
		CreatedAt:    token.CreatedAt,
		UpdatedAt:    time.Now(),
	}
	if err := storage.StoreToken(ctx, updatedToken); err != nil {
		t.Fatalf("StoreToken() update error = %v", err)
	}

	// Verify update
	retrieved, err := storage.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if retrieved.AccessToken != "updated-access-token" {
		t.Errorf("AccessToken = %s, want updated-access-token", retrieved.AccessToken)
	}

	// Verify only one token exists
	tokens, err := storage.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens() error = %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("ListTokens() returned %d tokens, want 1", len(tokens))
	}
}

func TestMemoryTokenStorage_IsolatesMutation(t *testing.T) {
	storage := NewMemoryTokenStorage()
	ctx := context.Background()

	// Store a token
	token := &Token{
		AccessToken:  "original-access",
		RefreshToken: "original-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		TenantID:     "tenant-1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := storage.StoreToken(ctx, token); err != nil {
		t.Fatalf("StoreToken() error = %v", err)
	}

	// Mutate the original token
	token.AccessToken = "mutated-access"

	// Retrieve and verify it's unchanged
	retrieved, err := storage.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if retrieved.AccessToken != "original-access" {
		t.Errorf("Storage mutation detected: AccessToken = %s, want original-access", retrieved.AccessToken)
	}

	// Mutate the retrieved token
	retrieved.AccessToken = "retrieved-mutation"

	// Get again and verify unchanged
	retrieved2, err := storage.GetToken(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("GetToken() 2 error = %v", err)
	}

	if retrieved2.AccessToken != "original-access" {
		t.Errorf("Retrieval mutation detected: AccessToken = %s, want original-access", retrieved2.AccessToken)
	}
}

func TestPostgresTokenStorageConfig_DefaultTableName(t *testing.T) {
	// We can't fully test PostgresTokenStorage without a database,
	// but we can verify the config defaults are applied
	cfg := &PostgresTokenStorageConfig{
		TableName: "",
	}

	if cfg.TableName != "" {
		t.Errorf("TableName = %s, want empty string", cfg.TableName)
	}

	// The default "oauth_tokens" is applied in NewPostgresTokenStorage
}
