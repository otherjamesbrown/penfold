//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/timeout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: TestTimeoutConfig_LoadFromDB, TestTimeoutConfig_SetAndRefresh,
// TestTimeoutConfig_OnChangeCallback, and TestTimeoutConfig_DefaultsFallback
// depend on pkg/timeout/config.go being updated to query pipeline_operational_config
// instead of pipeline_config. That update is handled by a separate shard.
// Until that shard lands, cfg.Refresh() calls will fail because pipeline_config
// has been dropped by migration 078.

func TestTimeoutConfig_LoadFromDB(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Create a timeout.Config with the real DB pool
	cfg := timeout.New(db.Pool, testTenantID)

	// Refresh to load from DB
	err := cfg.Refresh(ctx)
	require.NoError(t, err)

	// Verify values match what's in pipeline_operational_config table
	expectedKeys := []string{
		"timeout.ai_client.request",
		"timeout.activity.fast.start_to_close",
		"timeout.activity.fast.heartbeat",
		"timeout.activity.embedding.start_to_close",
		"timeout.activity.embedding.heartbeat",
		"timeout.activity.llm.start_to_close",
		"timeout.activity.llm.heartbeat",
		"timeout.activity.batch.start_to_close",
		"timeout.activity.batch.heartbeat",
		"timeout.http.backend.gemini",
		"timeout.http.backend.mlx",
		"timeout.schedule_to_close.default",
	}

	for _, key := range expectedKeys {
		// Read from DB directly
		var dbValueStr string
		err := db.Pool.QueryRow(ctx,
			"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
			testTenantID, key,
		).Scan(&dbValueStr)
		require.NoError(t, err, "key %s should exist in DB", key)

		dbValue, err := time.ParseDuration(dbValueStr)
		require.NoError(t, err, "key %s: bad DB value %s", key, dbValueStr)

		// Read from Config
		configValue := cfg.Get(key)
		assert.Equal(t, dbValue, configValue,
			"key %s: Config value %v should match DB value %v", key, configValue, dbValue)
	}

	// All entries should be loaded
	all := cfg.All()
	assert.Equal(t, 12, len(all), "expected 12 config entries")
}

func TestTimeoutConfig_SetAndRefresh(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	const testKey = "timeout.ai_client.request"

	// Save original value for restore
	var originalValue string
	err := db.Pool.QueryRow(ctx,
		"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
		testTenantID, testKey,
	).Scan(&originalValue)
	require.NoError(t, err)

	// Restore at end
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx,
			"UPDATE pipeline_operational_config SET value = $1, updated_at = now() WHERE tenant_id = $2 AND key = $3",
			originalValue, testTenantID, testKey,
		)
	})

	// Create Config and load
	cfg := timeout.New(db.Pool, testTenantID)
	err = cfg.Refresh(ctx)
	require.NoError(t, err)

	oldVal := cfg.Get(testKey)
	require.True(t, oldVal > 0, "expected positive old value")

	// Set a new value through Config.Set()
	newVal := 90 * time.Second
	err = cfg.Set(ctx, testKey, newVal)
	require.NoError(t, err)

	// Verify in-memory value updated
	assert.Equal(t, newVal, cfg.Get(testKey))

	// Create a fresh Config and Refresh to verify DB persistence
	cfg2 := timeout.New(db.Pool, testTenantID)
	err = cfg2.Refresh(ctx)
	require.NoError(t, err)

	assert.Equal(t, newVal, cfg2.Get(testKey),
		"fresh Config after Refresh should see the updated value")
}

func TestTimeoutConfig_OnChangeCallback(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	const testKey = "timeout.ai_client.request"

	// Save original value for restore
	var originalValue string
	err := db.Pool.QueryRow(ctx,
		"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
		testTenantID, testKey,
	).Scan(&originalValue)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx,
			"UPDATE pipeline_operational_config SET value = $1, updated_at = now() WHERE tenant_id = $2 AND key = $3",
			originalValue, testTenantID, testKey,
		)
	})

	// Create Config and load initial values
	cfg := timeout.New(db.Pool, testTenantID)
	err = cfg.Refresh(ctx)
	require.NoError(t, err)
	origDuration := cfg.Get(testKey)

	// Register callback
	var callbackFired bool
	var cbKey string
	var cbOldVal, cbNewVal time.Duration

	cfg.OnChange(func(key string, oldVal, newVal time.Duration) {
		callbackFired = true
		cbKey = key
		cbOldVal = oldVal
		cbNewVal = newVal
	})

	// Update value directly in DB
	newDuration := 200 * time.Second
	_, err = db.Pool.Exec(ctx,
		"UPDATE pipeline_operational_config SET value = $1, updated_at = now() WHERE tenant_id = $2 AND key = $3",
		newDuration.String(), testTenantID, testKey,
	)
	require.NoError(t, err)

	// Refresh to trigger callback
	err = cfg.Refresh(ctx)
	require.NoError(t, err)

	assert.True(t, callbackFired, "OnChange callback should have fired")
	assert.Equal(t, testKey, cbKey)
	assert.Equal(t, origDuration, cbOldVal)
	assert.Equal(t, newDuration, cbNewVal)
}

func TestTimeoutConfig_DefaultsFallback(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Create Config with nil DB (defaults only)
	cfgDefaults := timeout.New(nil, testTenantID)

	// Create Config with real DB
	cfgDB := timeout.New(db.Pool, testTenantID)
	err := cfgDB.Refresh(ctx)
	require.NoError(t, err)

	// Compare: hardcoded defaults should match DB seed values
	// (catches drift between Go code and SQL migration)
	// NOTE: pipeline_operational_config dropped the default_value column.
	// Defaults comparison is now between hardcoded Go defaults and DB current values.
	keys := []string{
		"timeout.ai_client.request",
		"timeout.activity.fast.start_to_close",
		"timeout.activity.fast.heartbeat",
		"timeout.activity.embedding.start_to_close",
		"timeout.activity.embedding.heartbeat",
		"timeout.activity.llm.start_to_close",
		"timeout.activity.llm.heartbeat",
		"timeout.activity.batch.start_to_close",
		"timeout.activity.batch.heartbeat",
		"timeout.http.backend.gemini",
		"timeout.http.backend.mlx",
		"timeout.schedule_to_close.default",
	}

	for _, key := range keys {
		defaultVal := cfgDefaults.Get(key)
		dbVal := cfgDB.Get(key)

		// Read current value from pipeline_operational_config
		var dbValueStr string
		err := db.Pool.QueryRow(ctx,
			"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
			testTenantID, key,
		).Scan(&dbValueStr)
		require.NoError(t, err, "key %s should exist in pipeline_operational_config", key)

		dbCurrentVal, err := time.ParseDuration(dbValueStr)
		require.NoError(t, err)

		assert.Equal(t, dbCurrentVal, defaultVal,
			"key %s: hardcoded default %v should match DB value %v (drift detected)", key, defaultVal, dbCurrentVal)

		// Also verify the actual DB value is parseable and positive
		assert.True(t, dbVal > 0, "key %s: DB value should be positive, got %v", key, dbVal)
	}
}
