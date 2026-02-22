package conversationservice

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// AuditPostgresRepository implements AuditRepository using pgxpool.
type AuditPostgresRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewAuditPostgresRepository creates a new AuditPostgresRepository.
func NewAuditPostgresRepository(pool *pgxpool.Pool, logger logging.Logger) *AuditPostgresRepository {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &AuditPostgresRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "conversation_audit_repository")),
	}
}

// FindOrphansWithThread finds content items that have a thread_id in
// content_enrichment but no entry in conversation_items.
func (r *AuditPostgresRepository) FindOrphansWithThread(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
	query := `
		SELECT s.content_id, ce.thread_id
		FROM content_enrichment ce
		JOIN sources s ON s.id = ce.source_id
		LEFT JOIN conversation_items ci ON ci.content_id = s.content_id
		WHERE ce.thread_id IS NOT NULL
		AND ce.thread_id != ''
		AND ci.conversation_id IS NULL
		AND s.tenant_id = $1
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find orphans with thread: %w", err)
	}
	defer rows.Close()

	var orphans []AuditOrphan
	for rows.Next() {
		var contentID, threadID string
		if err := rows.Scan(&contentID, &threadID); err != nil {
			return nil, fmt.Errorf("scan orphan with thread: %w", err)
		}

		// Try to find the conversation this item's thread maps to.
		suggested := r.findConversationByThreadKey(ctx, threadID, tenantID)

		orphans = append(orphans, AuditOrphan{
			ContentID:               contentID,
			Reason:                  fmt.Sprintf("has thread_id %q but no conversation membership", threadID),
			SuggestedConversationID: suggested,
		})
	}

	return orphans, rows.Err()
}

// FindOrphansWithReplySubject finds content items whose subject starts with
// Re:/FW:/Fwd: but have no thread_id in content_enrichment.
func (r *AuditPostgresRepository) FindOrphansWithReplySubject(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
	query := `
		SELECT s.content_id, s.ingestion_metadata->>'subject' AS subject
		FROM sources s
		LEFT JOIN content_enrichment ce ON ce.source_id = s.id
		LEFT JOIN conversation_items ci ON ci.content_id = s.content_id
		WHERE s.tenant_id = $1
		AND s.content_id IS NOT NULL
		AND (
			s.ingestion_metadata->>'subject' ~* '^(Re|FW|Fwd)\s*:'
		)
		AND (ce.thread_id IS NULL OR ce.thread_id = '')
		AND ci.conversation_id IS NULL
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find orphans with reply subject: %w", err)
	}
	defer rows.Close()

	var orphans []AuditOrphan
	for rows.Next() {
		var contentID string
		var subject *string
		if err := rows.Scan(&contentID, &subject); err != nil {
			return nil, fmt.Errorf("scan orphan with reply subject: %w", err)
		}

		subjectStr := ""
		if subject != nil {
			subjectStr = *subject
		}

		orphans = append(orphans, AuditOrphan{
			ContentID: contentID,
			Reason:    fmt.Sprintf("reply subject %q but no thread_id — parser may have missed References header", subjectStr),
		})
	}

	return orphans, rows.Err()
}

// FindDuplicateMemberships finds content items linked to multiple conversations.
func (r *AuditPostgresRepository) FindDuplicateMemberships(ctx context.Context, tenantID string) ([]AuditDuplicate, error) {
	query := `
		SELECT ci.content_id, array_agg(ci.conversation_id ORDER BY ci.conversation_id) AS conv_ids
		FROM conversation_items ci
		WHERE ci.tenant_id = $1
		GROUP BY ci.content_id
		HAVING count(DISTINCT ci.conversation_id) > 1
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find duplicate memberships: %w", err)
	}
	defer rows.Close()

	var duplicates []AuditDuplicate
	for rows.Next() {
		var contentID string
		var convIDs []string
		if err := rows.Scan(&contentID, &convIDs); err != nil {
			return nil, fmt.Errorf("scan duplicate membership: %w", err)
		}
		duplicates = append(duplicates, AuditDuplicate{
			ContentID:       contentID,
			ConversationIDs: convIDs,
		})
	}

	return duplicates, rows.Err()
}

// FindMergeCandidatesByOverlap finds conversations that share content items.
func (r *AuditPostgresRepository) FindMergeCandidatesByOverlap(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
	query := `
		WITH shared AS (
			SELECT
				ci1.conversation_id AS conv_a,
				ci2.conversation_id AS conv_b,
				array_agg(ci1.content_id ORDER BY ci1.content_id) AS shared_items
			FROM conversation_items ci1
			JOIN conversation_items ci2
				ON ci1.content_id = ci2.content_id
				AND ci1.conversation_id < ci2.conversation_id
			WHERE ci1.tenant_id = $1
			GROUP BY ci1.conversation_id, ci2.conversation_id
		)
		SELECT
			s.conv_a,
			s.conv_b,
			COALESCE(ca.topic, '') AS topic_a,
			COALESCE(cb.topic, '') AS topic_b,
			s.shared_items
		FROM shared s
		JOIN conversations ca ON ca.id = s.conv_a
		JOIN conversations cb ON cb.id = s.conv_b
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find merge candidates by overlap: %w", err)
	}
	defer rows.Close()

	var candidates []AuditMergeCandidate
	for rows.Next() {
		var mc AuditMergeCandidate
		if err := rows.Scan(&mc.ConversationIDA, &mc.ConversationIDB, &mc.TopicA, &mc.TopicB, &mc.SharedItems); err != nil {
			return nil, fmt.Errorf("scan merge candidate by overlap: %w", err)
		}
		mc.Reason = fmt.Sprintf("overlapping items: %d shared content IDs", len(mc.SharedItems))
		candidates = append(candidates, mc)
	}

	return candidates, rows.Err()
}

// FindMergeCandidatesByTopic finds conversations with similar topics after
// stripping Re:/FW:/Fwd: prefixes.
func (r *AuditPostgresRepository) FindMergeCandidatesByTopic(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
	query := `
		WITH normalized AS (
			SELECT
				id,
				topic,
				lower(trim(regexp_replace(topic, '^(Re|FW|Fwd)\s*:\s*', '', 'gi'))) AS norm_topic
			FROM conversations
			WHERE tenant_id = $1
			AND topic IS NOT NULL
			AND topic != ''
		)
		SELECT
			a.id AS conv_a,
			b.id AS conv_b,
			a.topic AS topic_a,
			b.topic AS topic_b
		FROM normalized a
		JOIN normalized b
			ON a.norm_topic = b.norm_topic
			AND a.id < b.id
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find merge candidates by topic: %w", err)
	}
	defer rows.Close()

	var candidates []AuditMergeCandidate
	for rows.Next() {
		var mc AuditMergeCandidate
		if err := rows.Scan(&mc.ConversationIDA, &mc.ConversationIDB, &mc.TopicA, &mc.TopicB); err != nil {
			return nil, fmt.Errorf("scan merge candidate by topic: %w", err)
		}
		mc.Reason = "similar topics after stripping reply/forward prefixes"
		candidates = append(candidates, mc)
	}

	return candidates, rows.Err()
}

// CountConversations returns the total conversation count for a tenant.
func (r *AuditPostgresRepository) CountConversations(ctx context.Context, tenantID string) (int32, error) {
	var count int32
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversations WHERE tenant_id = $1`, tenantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count conversations: %w", err)
	}
	return count, nil
}

// findConversationByThreadKey looks up a conversation by its thread_key.
func (r *AuditPostgresRepository) findConversationByThreadKey(ctx context.Context, threadKey, tenantID string) string {
	var convID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM conversations WHERE thread_key = $1 AND tenant_id = $2 LIMIT 1`,
		threadKey, tenantID,
	).Scan(&convID)
	if err != nil {
		return ""
	}
	return convID
}
