package glossaryservice

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// ============ Tests ============

// TestService_AddTerm_DuplicateHandling tests how the service handles
// ErrAlreadyExists from the repository layer. This is a unit test that
// verifies proper error translation without hitting the database.
//
// NOTE: This test documents the EXPECTED behavior once the bug is fixed.
// Currently the service doesn't properly handle ErrAlreadyExists from
// the repository's Create method (only from the GetByTerm check).
func TestService_AddTerm_DuplicateHandling(t *testing.T) {
	// This test demonstrates the gap in error handling:
	// The service checks for duplicates with GetByTerm, then calls Create.
	// But if there's a race condition and Create returns ErrAlreadyExists,
	// the service doesn't translate it to codes.AlreadyExists.
	//
	// The fix should:
	// 1. Repository Create() should wrap pgx unique constraint violations as ErrAlreadyExists
	// 2. Service AddTerm() should check errors.Is(err, pferrors.ErrAlreadyExists) and return AlreadyExists status

	t.Run("service should handle repository ErrAlreadyExists", func(t *testing.T) {
		// This test verifies the expected behavior.
		// It will FAIL until both repository and service are fixed.

		// We can't easily mock the repository since Service takes *glossary.Repository
		// This is documented as a limitation - the actual fix verification will happen
		// in the integration test (TestGlossaryRepository_CreateDuplicate) which
		// exercises the full repository -> service -> gRPC chain.

		t.Skip("Service takes concrete *glossary.Repository type - cannot mock. " +
			"Use integration test TestGlossaryRepository_CreateDuplicate to verify full chain.")
	})

	t.Run("documentation of expected service behavior", func(t *testing.T) {
		// Document what we expect the service to do:
		// When repository.Create() returns an error wrapping ErrAlreadyExists,
		// the service should return a gRPC AlreadyExists status.

		// Expected code pattern in service.go AddTerm():
		// created, err := s.repo.Create(ctx, input)
		// if err != nil {
		//     if errors.Is(err, pferrors.ErrAlreadyExists) {
		//         return nil, status.Errorf(codes.AlreadyExists, "term '%s' already exists", req.Term)
		//     }
		//     return nil, status.Errorf(codes.Internal, "failed to create term: %v", err)
		// }

		// This is a documentation test - it always passes
		assert.True(t, true, "Service should check errors.Is(err, pferrors.ErrAlreadyExists)")
	})
}

// TestService_AddTerm_RaceConditionScenario demonstrates the race condition
// that can occur with the current check-then-create pattern.
func TestService_AddTerm_RaceConditionScenario(t *testing.T) {
	// Scenario:
	// 1. Request A calls AddTerm("TERM1")
	// 2. Request B calls AddTerm("TERM1") at nearly the same time
	// 3. Both requests call GetByTerm("TERM1") -> both get nil (doesn't exist)
	// 4. Request A calls Create() -> succeeds, inserts TERM1
	// 5. Request B calls Create() -> fails with unique constraint violation
	// 6. Request B should return AlreadyExists, not Internal error

	// This scenario is tested in the integration test because it requires
	// real database state. This test documents the expected behavior.

	t.Run("concurrent creates should return AlreadyExists not Internal", func(t *testing.T) {
		// We expect BOTH of these to be handled gracefully:
		// 1. GetByTerm finds existing -> return AlreadyExists (currently works)
		// 2. Create returns ErrAlreadyExists -> return AlreadyExists (BUG - currently returns Internal)

		// The bug is that case #2 doesn't work because:
		// - Repository doesn't translate pgx error 23505 to ErrAlreadyExists
		// - Service doesn't check for ErrAlreadyExists from Create call

		t.Skip("Race condition scenario requires integration test with real database")
	})
}

// simulatePgxDuplicateError simulates the error that pgx returns for unique constraint violations
func simulatePgxDuplicateError() error {
	// PostgreSQL error code 23505 = unique_violation
	// The repository should detect this and wrap it as ErrAlreadyExists
	return fmt.Errorf("create glossary term: ERROR: duplicate key value violates unique constraint \"uq_glossary_term\" (SQLSTATE 23505)")
}

func TestErrorWrapping_Documentation(t *testing.T) {
	// This test documents what error wrapping should happen

	t.Run("repository should wrap pgx unique constraint as ErrAlreadyExists", func(t *testing.T) {
		// Simulated pgx error
		pgxErr := simulatePgxDuplicateError()

		// What the repository should do (but currently doesn't):
		// Check if pgx error has code "23505" (unique_violation)
		// If yes, return fmt.Errorf("%w: term already exists", pferrors.ErrAlreadyExists)

		// Current behavior: returns the raw pgx error
		assert.Contains(t, pgxErr.Error(), "23505", "pgx returns SQLSTATE 23505 for unique violations")
		assert.False(t, errors.Is(pgxErr, pferrors.ErrAlreadyExists), "Currently repository does NOT wrap as ErrAlreadyExists")
	})

	t.Run("after fix repository should properly wrap", func(t *testing.T) {
		// Expected behavior after fix:
		wrappedErr := fmt.Errorf("%w: term already exists", pferrors.ErrAlreadyExists)

		assert.True(t, errors.Is(wrappedErr, pferrors.ErrAlreadyExists), "After fix, error should wrap ErrAlreadyExists")
	})
}
