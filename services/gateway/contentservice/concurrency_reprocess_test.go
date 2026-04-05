package contentservice

// TestReprocessContent_ConcurrencyThrottle reproduces the TOCTOU race in
// ReprocessContent (pf-a7866b, originally pf-d62802).
//
// BUG: The concurrency check (count in-flight, compare to max_concurrent) and
// the status transition (reset to 'pending') were non-atomic. Rapid concurrent
// calls all saw the same in-flight count and all passed the check.
//
// FIX: ClaimReprocessSlot in pipeline.Repository uses an advisory lock to
// atomically check concurrency AND set the source to 'processing' in one
// transaction. This makes the in-flight count immediately visible to the next
// caller.
//
// TEST STRATEGY:
//
//  1. TestReprocessContent_ConcurrencyThrottle_AtCapacity — at-capacity
//     scenario. Asserts codes.ResourceExhausted and no workflow started.
//
//  2. TestReprocessContent_ConcurrencyThrottle_UnderCapacity — under-capacity
//     scenario. Asserts success and exactly one workflow started.
//
// Both tests use a fake Postgres backend (pgproto3-based) that serves all
// queries through the pipeline.Repository pgxpool. No database/sql mock is
// needed — all concurrency logic is now in the pipeline repo.

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ---------------------------------------------------------------------------
// fakePGServer — speaks enough Postgres extended query protocol to serve:
//   - GetSourceByContentID  (SELECT id, tenant_id, source_system, …)
//   - ClaimReprocessSlot transaction:
//     BEGIN, pg_advisory_xact_lock, SELECT value FROM pipeline_operational_config,
//     SELECT COUNT(*) FROM sources, UPDATE sources (claim), COMMIT/ROLLBACK
//   - DeleteRunsBySource (DELETE FROM pipeline_runs)
//
// Per-connection state (lastParsedQuery, txStatus) is tracked in fakePGConn.
// ---------------------------------------------------------------------------

type fakePGServer struct {
	contentID     string
	tenantID      string
	sourceID      int64
	maxConcurrent int
	inFlight      int
}

func newFakePGServer(contentID, tenantID string, sourceID int64, maxConcurrent, inFlight int) *fakePGServer {
	return &fakePGServer{
		contentID:     contentID,
		tenantID:      tenantID,
		sourceID:      sourceID,
		maxConcurrent: maxConcurrent,
		inFlight:      inFlight,
	}
}

// fakePGConn holds per-connection state for the fake Postgres backend.
type fakePGConn struct {
	srv      *fakePGServer
	query    string // last parsed SQL
	txStatus byte   // 'I' idle, 'T' in-transaction
}

func (s *fakePGServer) serve(conn net.Conn) {
	defer conn.Close() //nolint:errcheck

	c := &fakePGConn{srv: s, txStatus: 'I'}
	backend := pgproto3.NewBackend(conn, conn)

	// --- startup handshake ---
	msg, err := backend.ReceiveStartupMessage()
	if err != nil {
		return
	}
	switch msg.(type) {
	case *pgproto3.SSLRequest:
		if _, err := conn.Write([]byte{'N'}); err != nil {
			return
		}
		if _, err := backend.ReceiveStartupMessage(); err != nil {
			return
		}
	}

	backend.Send(&pgproto3.AuthenticationOk{})
	backend.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: 0})
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return
	}

	for {
		raw, err := backend.Receive()
		if err != nil {
			return
		}
		c.handle(backend, raw)
	}
}

func (c *fakePGConn) handle(b *pgproto3.Backend, msg pgproto3.FrontendMessage) {
	switch m := msg.(type) {
	// Simple query protocol — used by pgx for parameterless queries
	// (BEGIN, COMMIT, ROLLBACK, advisory lock, count query).
	case *pgproto3.Query:
		c.query = m.String
		c.sendSimpleQueryResponse(b)

	// Extended query protocol — used for parameterized queries.
	case *pgproto3.Parse:
		c.query = m.Query
		b.Send(&pgproto3.ParseComplete{})
		b.Flush() //nolint:errcheck

	case *pgproto3.Describe:
		c.sendDescribe(b, m)

	case *pgproto3.Bind:
		b.Send(&pgproto3.BindComplete{})
		b.Flush() //nolint:errcheck

	case *pgproto3.Execute:
		c.sendExecuteResponse(b)

	case *pgproto3.Sync:
		b.Send(&pgproto3.ReadyForQuery{TxStatus: c.txStatus})
		b.Flush() //nolint:errcheck

	case *pgproto3.Close:
		b.Send(&pgproto3.CloseComplete{})
		b.Flush() //nolint:errcheck

	case *pgproto3.Terminate:
		return
	}
}

// sendSimpleQueryResponse handles the simple query protocol. pgx uses this for
// parameterless queries like BEGIN, COMMIT, advisory locks, and simple SELECTs.
func (c *fakePGConn) sendSimpleQueryResponse(b *pgproto3.Backend) {
	q := strings.ToLower(strings.TrimSpace(c.query))

	switch {
	case q == "begin" || q == "begin;":
		c.txStatus = 'T'
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")})

	case q == "commit" || q == "commit;":
		c.txStatus = 'I'
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")})

	case q == "rollback" || q == "rollback;":
		c.txStatus = 'I'
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")})

	case strings.Contains(q, "pg_advisory_xact_lock"):
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	case strings.Contains(q, "count") && strings.Contains(q, "processing_status"):
		b.Send(&pgproto3.RowDescription{
			Fields: []pgproto3.FieldDescription{
				{Name: []byte("count"), DataTypeOID: 20},
			},
		})
		b.Send(&pgproto3.DataRow{
			Values: [][]byte{[]byte(fmt.Sprintf("%d", c.srv.inFlight))},
		})
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	case strings.Contains(q, "delete") && strings.Contains(q, "pipeline_runs"):
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("DELETE 0")})

	default:
		b.Send(&pgproto3.ErrorResponse{
			Severity: "ERROR",
			Code:     "42601",
			Message:  fmt.Sprintf("fakePGServer: unexpected simple query: %s", c.query),
		})
	}

	b.Send(&pgproto3.ReadyForQuery{TxStatus: c.txStatus})
	b.Flush() //nolint:errcheck
}

func (c *fakePGConn) sendDescribe(b *pgproto3.Backend, m *pgproto3.Describe) {
	q := c.query
	isSourceSelect := strings.Contains(q, "SELECT id") && strings.Contains(q, "content_id")
	isConfigSelect := strings.Contains(q, "pipeline_operational_config") && strings.Contains(q, "value")
	isCountSelect := strings.Contains(q, "COUNT") && strings.Contains(q, "processing_status")
	isUpdateSources := strings.Contains(q, "UPDATE sources") && strings.Contains(q, "processing_status")
	isDeleteRuns := strings.Contains(q, "DELETE") && strings.Contains(q, "pipeline_runs")

	if m.ObjectType == 'S' {
		// Statement-level describe: ParameterDescription + RowDescription/NoData
		switch {
		case isSourceSelect:
			b.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{25}})
			b.Send(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("id"), DataTypeOID: 20},
					{Name: []byte("tenant_id"), DataTypeOID: 25},
					{Name: []byte("source_system"), DataTypeOID: 25},
					{Name: []byte("content_hash"), DataTypeOID: 25},
					{Name: []byte("content_id"), DataTypeOID: 25},
					{Name: []byte("processing_status"), DataTypeOID: 25},
				},
			})
		case isConfigSelect:
			b.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{25}})
			b.Send(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("value"), DataTypeOID: 25},
				},
			})
		case isCountSelect:
			b.Send(&pgproto3.ParameterDescription{})
			b.Send(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("count"), DataTypeOID: 20},
				},
			})
		case isUpdateSources:
			b.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20}}) // $1 = int8 source_id
			b.Send(&pgproto3.NoData{})
		case isDeleteRuns:
			b.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20}}) // $1 = int8 source_id
			b.Send(&pgproto3.NoData{})
		default:
			b.Send(&pgproto3.ParameterDescription{})
			b.Send(&pgproto3.NoData{})
		}
	} else {
		// Portal-level describe
		switch {
		case isSourceSelect:
			b.Send(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("id"), DataTypeOID: 20},
					{Name: []byte("tenant_id"), DataTypeOID: 25},
					{Name: []byte("source_system"), DataTypeOID: 25},
					{Name: []byte("content_hash"), DataTypeOID: 25},
					{Name: []byte("content_id"), DataTypeOID: 25},
					{Name: []byte("processing_status"), DataTypeOID: 25},
				},
			})
		case isConfigSelect:
			b.Send(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("value"), DataTypeOID: 25},
				},
			})
		case isCountSelect:
			b.Send(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("count"), DataTypeOID: 20},
				},
			})
		default:
			b.Send(&pgproto3.NoData{})
		}
	}
	b.Flush() //nolint:errcheck
}

func (c *fakePGConn) sendExecuteResponse(b *pgproto3.Backend) {
	q := strings.ToLower(c.query)
	switch {
	// Transaction control
	case q == "begin" || q == "begin;":
		c.txStatus = 'T'
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")})

	case q == "commit" || q == "commit;":
		c.txStatus = 'I'
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")})

	case q == "rollback" || q == "rollback;":
		c.txStatus = 'I'
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")})

	// Advisory lock (no-op in test)
	case strings.Contains(q, "pg_advisory_xact_lock"):
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	// GetSourceByContentID
	case strings.Contains(q, "select id") && strings.Contains(q, "content_id"):
		b.Send(&pgproto3.DataRow{
			Values: [][]byte{
				[]byte(fmt.Sprintf("%d", c.srv.sourceID)),
				[]byte(c.srv.tenantID),
				[]byte("email"),
				[]byte("abc123"),
				[]byte(c.srv.contentID),
				[]byte("completed"),
			},
		})
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	// Config query — pipeline.max_concurrent
	case strings.Contains(q, "pipeline_operational_config") && strings.Contains(q, "value"):
		b.Send(&pgproto3.DataRow{
			Values: [][]byte{
				[]byte(fmt.Sprintf("%d", c.srv.maxConcurrent)),
			},
		})
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	// Count in-flight sources
	case strings.Contains(q, "count") && strings.Contains(q, "processing_status"):
		b.Send(&pgproto3.DataRow{
			Values: [][]byte{
				[]byte(fmt.Sprintf("%d", c.srv.inFlight)),
			},
		})
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	// UPDATE sources (claim slot or reset)
	case strings.Contains(q, "update sources") && strings.Contains(q, "processing_status"):
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("UPDATE 1")})

	// DELETE pipeline_runs
	case strings.Contains(q, "delete") && strings.Contains(q, "pipeline_runs"):
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("DELETE 0")})

	default:
		b.Send(&pgproto3.ErrorResponse{
			Severity: "ERROR",
			Code:     "42601",
			Message:  fmt.Sprintf("fakePGServer: unexpected query: %s", c.query),
		})
	}
	b.Flush() //nolint:errcheck
}

// newFakePGPool creates a *pgxpool.Pool whose connections are served by srv
// via in-process net.Pipe() pairs (no real network socket).
func newFakePGPool(t *testing.T, srv *fakePGServer) *pgxpool.Pool {
	t.Helper()

	config, err := pgxpool.ParseConfig("host=127.0.0.1 port=65432 dbname=fakedb user=fakeuser password=fake")
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}

	config.ConnConfig.Config.DialFunc = func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go srv.serve(server)
		return client, nil
	}
	config.ConnConfig.Config.TLSConfig = nil

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// ---------------------------------------------------------------------------
// TestReprocessContent_ConcurrencyThrottle_AtCapacity
//
// Scenario: pipeline.max_concurrent = 5, 5 sources currently processing.
// ClaimReprocessSlot should return claimed=false, and ReprocessContent should
// return codes.ResourceExhausted without starting any workflow.
// ---------------------------------------------------------------------------
func TestReprocessContent_ConcurrencyThrottle_AtCapacity(t *testing.T) {
	const (
		testContentID = "content-id-throttle-at-capacity"
		testTenantID  = "tenant-id-throttle-test"
		testSourceID  = int64(42)
		maxConcurrent = 5
		inFlight      = 5 // exactly at limit
	)

	ctx := context.Background()

	pgSrv := newFakePGServer(testContentID, testTenantID, testSourceID, maxConcurrent, inFlight)
	pool := newFakePGPool(t, pgSrv)
	pipelineRepo := pipeline.NewRepository(pool)

	temporalMock := &mockTemporalClient{}

	logger := logging.NewLogger(nil)
	svc := &Service{
		repo:           new(MockRepository),
		tenantRepo:     nil,
		pipelineRepo:   pipelineRepo,
		temporalClient: temporalMock,
		logger:         logger,
		langfuseClient: nil,
	}

	req := &contentv1.ReprocessContentRequest{
		ContentId: testContentID,
		Reason:    "pf-a7866b concurrency throttle — at capacity",
	}

	_, err := svc.ReprocessContent(ctx, req)

	if err == nil {
		t.Fatalf(
			"ReprocessContent returned nil (OK) when in-flight (%d) >= max_concurrent (%d). "+
				"ExecuteWorkflow called %d time(s). Expected: codes.ResourceExhausted",
			inFlight, maxConcurrent, temporalMock.executeWorkflowCallCount,
		)
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got: %v (type %T)", err, err)
	}

	if st.Code() != codes.ResourceExhausted {
		t.Fatalf(
			"expected codes.ResourceExhausted, got %v: %q",
			st.Code(), st.Message(),
		)
	}

	if temporalMock.executeWorkflowCallCount != 0 {
		t.Errorf(
			"expected 0 ExecuteWorkflow calls when at capacity, got %d",
			temporalMock.executeWorkflowCallCount,
		)
	}
}

// ---------------------------------------------------------------------------
// TestReprocessContent_ConcurrencyThrottle_UnderCapacity
//
// Scenario: pipeline.max_concurrent = 5, 2 sources currently processing.
// ClaimReprocessSlot should claim the slot, and ReprocessContent should
// succeed and fire exactly one workflow.
// ---------------------------------------------------------------------------
func TestReprocessContent_ConcurrencyThrottle_UnderCapacity(t *testing.T) {
	const (
		testContentID = "content-id-throttle-under-capacity"
		testTenantID  = "tenant-id-throttle-test-2"
		testSourceID  = int64(43)
		maxConcurrent = 5
		inFlight      = 2 // well under limit
	)

	ctx := context.Background()

	pgSrv := newFakePGServer(testContentID, testTenantID, testSourceID, maxConcurrent, inFlight)
	pool := newFakePGPool(t, pgSrv)
	pipelineRepo := pipeline.NewRepository(pool)

	temporalMock := &mockTemporalClient{}

	logger := logging.NewLogger(nil)
	svc := &Service{
		repo:           new(MockRepository),
		tenantRepo:     nil,
		pipelineRepo:   pipelineRepo,
		temporalClient: temporalMock,
		logger:         logger,
		langfuseClient: nil,
	}

	req := &contentv1.ReprocessContentRequest{
		ContentId: testContentID,
		Reason:    "pf-a7866b under-capacity should succeed",
	}

	_, err := svc.ReprocessContent(ctx, req)

	if err != nil {
		st, _ := status.FromError(err)
		t.Fatalf(
			"expected success when inFlight (%d) < maxConcurrent (%d), got %v: %q",
			inFlight, maxConcurrent, st.Code(), st.Message(),
		)
	}

	if temporalMock.executeWorkflowCallCount != 1 {
		t.Errorf(
			"expected exactly 1 ExecuteWorkflow call when under capacity, got %d",
			temporalMock.executeWorkflowCallCount,
		)
	}
}
