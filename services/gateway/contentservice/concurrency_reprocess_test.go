package contentservice

// TestReprocessContent_ConcurrencyThrottle reproduces bug pf-d62802.
//
// BUG: ReprocessContent in service.go fires ExecuteWorkflow with no concurrency
// check. The throttle from pf-b2f050 only covers KickProcessing in
// pipelineservice (lines 138-172). Individual reprocess calls bypass it entirely.
//
// EXPECTED FIX: After source lookup and before ExecuteWorkflow, read
// pipeline.max_concurrent from pipeline_config, count sources with
// processing_status='processing', and return codes.ResourceExhausted if
// inFlight >= maxConcurrent.
//
// TEST STRATEGY:
//
//  1. TestReprocessContent_ConcurrencyThrottle_AtCapacity — the primary
//     reproduction test. Sets up max_concurrent=5 with 5 in-flight sources.
//     Asserts codes.ResourceExhausted is returned. FAILS on current code
//     because no concurrency check exists — the service proceeds and calls
//     ExecuteWorkflow returning OK instead.
//
//  2. TestReprocessContent_ConcurrencyThrottle_UnderCapacity — the
//     complementary green-path test. Sets up max_concurrent=5 with 2
//     in-flight. Asserts success and that exactly one workflow is fired.
//     PASSES on current (unfixed) code and must continue to pass after fix.
//
// Both tests require a fake Postgres backend (pgproto3-based) to serve
// the GetSourceByContentID and ResetSourceStatus queries issued by
// pipeline.Repository, and a custom database/sql driver to serve the
// pipeline_config and sources count queries that the fix will issue against
// s.db.
//
// All of this is pure in-process — no external database is needed.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
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
// concurrencyMockSQLDriver — a minimal database/sql driver for s.db.
//
// The fix will use s.db to run two queries:
//   1. SELECT value FROM pipeline_config WHERE key = 'pipeline.max_concurrent'
//   2. SELECT COUNT(*) FROM sources WHERE processing_status = 'processing'
//
// This driver answers those two queries with controlled values.
// ---------------------------------------------------------------------------

const concurrencyMockDriverName = "concurrency-mock-pf-d62802"

func init() {
	sql.Register(concurrencyMockDriverName, &concurrencyMockDriver{})
}

type concurrencyMockDriver struct{}

func (d *concurrencyMockDriver) Open(name string) (driver.Conn, error) {
	var maxConcurrent, inFlight int
	fmt.Sscanf(name, "maxConcurrent=%d;inFlight=%d", &maxConcurrent, &inFlight)
	return &concurrencyMockConn{maxConcurrent: maxConcurrent, inFlight: inFlight}, nil
}

type concurrencyMockConn struct {
	maxConcurrent int
	inFlight      int
}

func (c *concurrencyMockConn) Prepare(query string) (driver.Stmt, error) {
	return &concurrencyMockStmt{conn: c, query: query}, nil
}
func (c *concurrencyMockConn) Close() error                        { return nil }
func (c *concurrencyMockConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("mock: transactions not supported")
}

type concurrencyMockStmt struct {
	conn  *concurrencyMockConn
	query string
}

func (s *concurrencyMockStmt) Close() error                               { return nil }
func (s *concurrencyMockStmt) NumInput() int                              { return -1 }
func (s *concurrencyMockStmt) Exec(args []driver.Value) (driver.Result, error) {
	return nil, fmt.Errorf("mock: unexpected Exec")
}
func (s *concurrencyMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	q := strings.TrimSpace(s.query)
	switch {
	case strings.Contains(q, "pipeline.max_concurrent"):
		return &singleColRows{value: fmt.Sprintf("%d", s.conn.maxConcurrent)}, nil
	case strings.Contains(q, "processing_status") && strings.Contains(q, "COUNT"):
		return &singleColRows{value: fmt.Sprintf("%d", s.conn.inFlight)}, nil
	default:
		return nil, fmt.Errorf("mock: unexpected query: %s", q)
	}
}

type singleColRows struct {
	value string
	done  bool
}

func (r *singleColRows) Columns() []string { return []string{"value"} }
func (r *singleColRows) Close() error      { return nil }
func (r *singleColRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = r.value
	r.done = true
	return nil
}

func openConcurrencyMockDB(t *testing.T, maxConcurrent, inFlight int) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("maxConcurrent=%d;inFlight=%d", maxConcurrent, inFlight)
	db, err := sql.Open(concurrencyMockDriverName, dsn)
	if err != nil {
		t.Fatalf("sql.Open mock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// fakePGServer — speaks enough Postgres extended query protocol to serve:
//   • GetSourceByContentID  (SELECT id, tenant_id, source_system, …)
//   • ResetSourceStatus     (UPDATE sources SET processing_status = 'pending' …)
//
// pgx/v5 uses the extended query protocol (Parse / Bind / Execute / Sync)
// rather than simple query protocol for prepared statements.
// ---------------------------------------------------------------------------

type fakePGServer struct {
	contentID string
	tenantID  string
	sourceID  int64

	// lastParsedQuery holds the SQL text from the most recent Parse message,
	// so we can dispatch correctly on Execute.
	lastParsedQuery string
}

func newFakePGServer(contentID, tenantID string, sourceID int64) *fakePGServer {
	return &fakePGServer{contentID: contentID, tenantID: tenantID, sourceID: sourceID}
}

// serve runs the fake backend on conn until the client disconnects or sends
// Terminate.
func (s *fakePGServer) serve(conn net.Conn) {
	defer conn.Close()

	backend := pgproto3.NewBackend(conn, conn)

	// --- startup handshake ---
	msg, err := backend.ReceiveStartupMessage()
	if err != nil {
		return
	}
	// Accept any startup (StartupMessage or SSLRequest).
	switch msg.(type) {
	case *pgproto3.SSLRequest:
		// Decline SSL — send 'N'
		if _, err := conn.Write([]byte{'N'}); err != nil {
			return
		}
		// Re-read the real startup message
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

	// --- message loop ---
	for {
		raw, err := backend.Receive()
		if err != nil {
			return
		}
		s.handle(backend, raw)
	}
}

func (s *fakePGServer) handle(b *pgproto3.Backend, msg pgproto3.FrontendMessage) {
	switch m := msg.(type) {
	case *pgproto3.Parse:
		s.lastParsedQuery = m.Query
		b.Send(&pgproto3.ParseComplete{})
		b.Flush() //nolint:errcheck

	case *pgproto3.Describe:
		// Describe asks for parameter types and row description.
		q := s.lastParsedQuery
		isSelect := strings.Contains(q, "SELECT id") && strings.Contains(q, "content_id")
		isUpdate := strings.Contains(q, "UPDATE sources") && strings.Contains(q, "processing_status")

		if m.ObjectType == 'S' {
			// Statement-level describe: send ParameterDescription + RowDescription/NoData
			if isSelect {
				// SELECT query has one text parameter ($1 = content_id)
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
			} else if isUpdate {
				// UPDATE query has one int8 parameter ($1 = source_id int64)
				b.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20}}) // OID 20 = int8
				b.Send(&pgproto3.NoData{})
			} else {
				b.Send(&pgproto3.ParameterDescription{})
				b.Send(&pgproto3.NoData{})
			}
		} else {
			// Portal-level describe: send RowDescription or NoData
			if isSelect {
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
			} else {
				b.Send(&pgproto3.NoData{})
			}
		}
		b.Flush() //nolint:errcheck

	case *pgproto3.Bind:
		b.Send(&pgproto3.BindComplete{})
		b.Flush() //nolint:errcheck

	case *pgproto3.Execute:
		s.sendExecuteResponse(b)

	case *pgproto3.Sync:
		b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		b.Flush() //nolint:errcheck

	case *pgproto3.Close:
		b.Send(&pgproto3.CloseComplete{})
		b.Flush() //nolint:errcheck

	case *pgproto3.Terminate:
		return

	default:
		// Ignore unknown messages
	}
}

func (s *fakePGServer) sendExecuteResponse(b *pgproto3.Backend) {
	q := s.lastParsedQuery
	switch {
	case strings.Contains(q, "SELECT id") && strings.Contains(q, "content_id"):
		// GetSourceByContentID — return one row with 6 fields:
		// id, tenant_id, source_system, content_hash, content_id, processing_status
		b.Send(&pgproto3.DataRow{
			Values: [][]byte{
				[]byte(fmt.Sprintf("%d", s.sourceID)),
				[]byte(s.tenantID),
				[]byte("email"),
				[]byte("abc123"),
				[]byte(s.contentID),
				[]byte("pending"),
			},
		})
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})

	case strings.Contains(q, "UPDATE sources") && strings.Contains(q, "processing_status"):
		// ResetSourceStatus — report 1 row updated
		b.Send(&pgproto3.CommandComplete{CommandTag: []byte("UPDATE 1")})

	default:
		b.Send(&pgproto3.ErrorResponse{
			Severity: "ERROR",
			Code:     "42601",
			Message:  fmt.Sprintf("fakePGServer: unexpected query: %s", q),
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

	// Custom DialFunc: every connection attempt returns one end of a net.Pipe()
	// while a goroutine serves the other end with our fake Postgres backend.
	config.ConnConfig.Config.DialFunc = func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go srv.serve(server)
		return client, nil
	}
	// Disable TLS — our fake server does not speak SSL.
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
// BUG REPRODUCTION TEST — pf-d62802
//
// Scenario: pipeline.max_concurrent = 5, 5 sources currently processing
// (inFlight == maxConcurrent). The request should be rejected with
// codes.ResourceExhausted because we are at capacity.
//
// Current behaviour (BUG): ReprocessContent ignores the concurrency limit,
// calls ExecuteWorkflow, and returns OK.
//
// Expected behaviour (after fix): returns codes.ResourceExhausted immediately
// without calling ExecuteWorkflow.
//
// This test FAILS on the current (unfixed) code because:
//   - The service returns nil error (success), which triggers t.Fatalf.
//   - Or the service returns a non-ResourceExhausted error (e.g. the fake DB
//     returns an error on an unexpected query path), which still triggers
//     t.Fatalf with a clear message.
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

	// Fake Postgres backend for pipeline.Repository calls
	pgSrv := newFakePGServer(testContentID, testTenantID, testSourceID)
	pool := newFakePGPool(t, pgSrv)
	pipelineRepo := pipeline.NewRepository(pool)

	// Mock sql.DB — responds to concurrency check queries with at-capacity values
	sqlDB := openConcurrencyMockDB(t, maxConcurrent, inFlight)

	// Mock Temporal client — captures ExecuteWorkflow calls
	temporalMock := &mockTemporalClient{}

	logger := logging.NewLogger(nil)
	svc := &Service{
		repo:           new(MockRepository),
		tenantRepo:     nil,
		pipelineRepo:   pipelineRepo,
		temporalClient: temporalMock,
		db:             sqlDB,
		logger:         logger,
		langfuseClient: nil,
	}

	req := &contentv1.ReprocessContentRequest{
		ContentId: testContentID,
		Reason:    "pf-d62802 concurrency throttle reproduction — at capacity",
	}

	_, err := svc.ReprocessContent(ctx, req)

	// -----------------------------------------------------------------------
	// ASSERTION: when inFlight >= maxConcurrent, expect ResourceExhausted.
	//
	// Current code returns nil (success) — this triggers the first t.Fatalf.
	// After the fix, ResourceExhausted is expected and the test passes.
	// -----------------------------------------------------------------------
	if err == nil {
		t.Fatalf(
			"pf-d62802 REPRODUCED: ReprocessContent returned nil (OK) when "+
				"in-flight (%d) >= max_concurrent (%d). "+
				"No concurrency check exists — ExecuteWorkflow was called %d time(s). "+
				"Expected: codes.ResourceExhausted",
			inFlight, maxConcurrent, temporalMock.executeWorkflowCallCount,
		)
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got: %v (type %T)", err, err)
	}

	if st.Code() != codes.ResourceExhausted {
		t.Fatalf(
			"pf-d62802 REPRODUCED: ReprocessContent returned %v (%q) instead of "+
				"codes.ResourceExhausted. "+
				"This confirms the concurrency check does not yet exist in ReprocessContent. "+
				"The throttle in pipelineservice.KickProcessing (lines 138-172) is bypassed "+
				"by direct reprocess calls.",
			st.Code(), st.Message(),
		)
	}

	// Fix is in place — verify no workflow was started.
	if temporalMock.executeWorkflowCallCount != 0 {
		t.Errorf(
			"expected 0 ExecuteWorkflow calls when at capacity, got %d — "+
				"fix should prevent workflow start",
			temporalMock.executeWorkflowCallCount,
		)
	}
}

// ---------------------------------------------------------------------------
// TestReprocessContent_ConcurrencyThrottle_UnderCapacity
//
// Green-path companion test: when inFlight < maxConcurrent, ReprocessContent
// should succeed and fire exactly one workflow.
//
// This test PASSES on both current code (which always fires the workflow) and
// after the fix (which should still fire when under capacity).
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

	pgSrv := newFakePGServer(testContentID, testTenantID, testSourceID)
	pool := newFakePGPool(t, pgSrv)
	pipelineRepo := pipeline.NewRepository(pool)

	sqlDB := openConcurrencyMockDB(t, maxConcurrent, inFlight)
	temporalMock := &mockTemporalClient{}

	logger := logging.NewLogger(nil)
	svc := &Service{
		repo:           new(MockRepository),
		tenantRepo:     nil,
		pipelineRepo:   pipelineRepo,
		temporalClient: temporalMock,
		db:             sqlDB,
		logger:         logger,
		langfuseClient: nil,
	}

	req := &contentv1.ReprocessContentRequest{
		ContentId: testContentID,
		Reason:    "pf-d62802 under-capacity should succeed",
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
