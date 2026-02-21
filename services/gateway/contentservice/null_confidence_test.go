package contentservice

// TestNullConfidenceScan is a reproduction test for the NULL confidence scan bug
// tracked in shard pf-6b3eb8.
//
// Bug: `penf content assertions <content-id>` fails with:
//   "can't scan into dest[4] (col: confidence): cannot scan NULL into *float32"
//
// Root cause: AssertionRecord.Confidence is float32 (non-pointer). The DB
// confidence column allows NULL. pgx cannot scan a SQL NULL into a *float32
// pointing at a non-pointer float32 value.
//
// The fix is to change AssertionRecord.Confidence to *float32 and update the
// proto conversion to nil-check before dereferencing (matching the pattern in
// assertionsservice/service.go lines 200-203).
//
// FAILURE MODES against unfixed code
// -----------------------------------
//
// TestNullConfidenceFieldType    — FAIL at runtime (reflect assertion)
// TestNullConfidenceProtoConversion — FAIL at compile time (nil assignment to float32)
//
// After the fix both tests PASS.

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
)

// TestNullConfidenceFieldType asserts that AssertionRecord.Confidence is a
// pointer type (*float32) so that the pgx scanner can represent a SQL NULL.
//
// This test FAILS at runtime with the current code because Confidence is float32
// (a non-pointer kind). It PASSES once the field is changed to *float32.
//
// The DB confidence column allows NULL (migration 034). When pgx tries to scan
// a NULL row into &rec.Confidence where Confidence is float32, it returns:
//   "cannot scan NULL into *float32"
// A *float32 field passes pgx a **float32; pgx sets the outer pointer to nil
// on NULL, which is the correct representation.
func TestNullConfidenceFieldType(t *testing.T) {
	rec := AssertionRecord{}
	rt := reflect.TypeOf(rec)

	field, ok := rt.FieldByName("Confidence")
	require.True(t, ok, "AssertionRecord must have a Confidence field")

	// The field must be a pointer so that pgx can scan SQL NULL into it.
	// A non-pointer float32 cannot represent NULL and causes:
	//   "cannot scan NULL into *float32"
	assert.Equal(t, reflect.Ptr, field.Type.Kind(),
		"AssertionRecord.Confidence must be *float32 (pointer kind) to handle "+
			"SQL NULL; current type is %s — change it to *float32", field.Type)
}

// TestNullConfidenceProtoConversion verifies that the GetAssertions handler
// correctly maps a nil Confidence (*float32 == nil, i.e. SQL NULL) to 0.0
// in the proto response without panicking.
//
// HOW THIS TEST FAILS AGAINST CURRENT CODE
// -----------------------------------------
// The test file will not compile because line 96 assigns nil to
// AssertionRecord.Confidence, which is currently float32 (non-pointer):
//
//   cannot use nil as float32 value in struct literal
//
// This compile error is itself the bug reproduction: the type literally cannot
// represent a SQL NULL value. Once Confidence is changed to *float32 the file
// compiles and the runtime assertions validate the nil-check in the proto
// conversion loop at service.go ~line 2079.
func TestNullConfidenceProtoConversion(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	// Construct an AssertionRecord with nil Confidence (representing SQL NULL).
	//
	// BUG MARKER: This line does NOT compile when Confidence is float32:
	//   cannot use nil as float32 value in struct literal
	//
	// It compiles only after Confidence is changed to *float32.
	rec := &AssertionRecord{
		ID:              42,
		AssertionType:   "risk",
		Description:     "Risk with unknown confidence (NULL in DB)",
		SourceQuote:     nil,
		Confidence:      nil, // SQL NULL — compile error with float32, OK with *float32
		ExtractionModel: nil,
		CreatedAt:       now,
	}

	mockRepo.On("GetAssertions", ctx, "em-nullconf", (*string)(nil)).
		Return([]*AssertionRecord{rec}, nil)

	req := &contentv1.GetAssertionsRequest{
		ContentId: "em-nullconf",
	}

	resp, err := svc.GetAssertions(ctx, req)

	assert.NoError(t, err, "GetAssertions must not error when confidence is NULL")
	require.NotNil(t, resp)
	require.Len(t, resp.Assertions, 1)

	// NULL confidence must map to 0.0 in the proto output (not panic).
	assert.Equal(t, float32(0.0), resp.Assertions[0].Confidence,
		"NULL confidence from DB must be represented as 0.0 in proto")

	mockRepo.AssertExpectations(t)
}
