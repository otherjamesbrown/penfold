package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStoredToken_ExpiryCheck(t *testing.T) {
	t.Run("token not expired", func(t *testing.T) {
		token := &StoredToken{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(1 * time.Hour),
		}
		needsRefresh := time.Now().Add(5 * time.Minute).After(token.Expiry)
		assert.False(t, needsRefresh, "token with 1h remaining should not need refresh")
	})

	t.Run("token expires within margin", func(t *testing.T) {
		token := &StoredToken{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(3 * time.Minute),
		}
		margin := 5 * time.Minute
		needsRefresh := time.Now().Add(margin).After(token.Expiry)
		assert.True(t, needsRefresh, "token expiring in 3min should need refresh with 5min margin")
	})

	t.Run("token already expired", func(t *testing.T) {
		token := &StoredToken{
			AccessToken: "test-token",
			Expiry:      time.Now().Add(-10 * time.Minute),
		}
		needsRefresh := time.Now().After(token.Expiry)
		assert.True(t, needsRefresh, "expired token should need refresh")
	})
}
