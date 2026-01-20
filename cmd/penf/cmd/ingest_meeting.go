package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/ingest/meeting"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
)

// Meeting ingest specific flags
var (
	meetingSource    string
	meetingPlatform  string
	meetingDryRun    bool
)

// DefaultTenantID for single-tenant mode
const DefaultTenantID = "00000001-0000-0000-0000-000000000001"

// newIngestMeetingCommand creates the 'ingest meeting' subcommand.
func newIngestMeetingCommand(deps *IngestCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "meeting <path>",
		Short: "Ingest meeting transcripts into Penfold",
		Long: `Ingest meeting transcripts, chat logs, and metadata into Penfold.

Supports:
  - WebVTT (.vtt) transcripts from Webex/Zoom
  - Plain text transcripts (Transcript_*.txt)
  - Chat logs (Chat messages_*.txt)
  - Meeting directories with multiple related files

Files are automatically grouped by meeting. Transcripts are parsed to extract
participants and generate embeddings for semantic search.

Examples:
  # Ingest a single VTT transcript
  penf ingest meeting ./meeting.vtt --source "project-x"

  # Ingest a meeting directory (transcript + chat)
  penf ingest meeting ./MeetingFolder/ --source "weekly-sync"

  # Ingest all meetings from a directory
  penf ingest meeting ~/meetings/ --source "archive-2025"

  # Preview without importing (dry run)
  penf ingest meeting ./meetings/ --source "test" --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIngestMeeting(cmd.Context(), deps, args[0])
		},
	}

	// Meeting-specific flags
	cmd.Flags().StringVarP(&meetingSource, "source", "s", "", "Source tag identifier (required)")
	cmd.Flags().StringVar(&meetingPlatform, "platform", "webex", "Meeting platform: webex, teams, zoom, google_meet")
	cmd.Flags().BoolVar(&meetingDryRun, "dry-run", false, "Preview import without persisting")

	cmd.MarkFlagRequired("source")

	// Add resolve subcommand
	cmd.AddCommand(newResolveMeetingParticipantsCommand(deps))
	// Add mentions subcommand
	cmd.AddCommand(newExtractMeetingMentionsCommand(deps))

	return cmd
}

// newResolveMeetingParticipantsCommand creates the 'ingest meeting resolve' subcommand.
func newResolveMeetingParticipantsCommand(deps *IngestCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve",
		Short: "Resolve meeting participants to people",
		Long: `Resolve meeting participants from transcripts to known people in the database.

This command matches participant names against the people table using:
  - Exact canonical name matches
  - Alias matches
  - Name normalization (strips pronouns like she/her, he/him)

Examples:
  # Resolve all unresolved meeting participants
  penf ingest meeting resolve

  # Resolve participants for a specific source
  penf ingest meeting resolve --source "test-data"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResolveMeetingParticipants(cmd.Context(), deps)
		},
	}
}

// runIngestMeeting executes the meeting ingestion command.
func runIngestMeeting(ctx context.Context, deps *IngestCommandDeps, path string) error {
	// Load configuration
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Validate path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("path not found: %s", path)
	}
	if err != nil {
		return fmt.Errorf("accessing path: %w", err)
	}

	// Validate source is provided
	if meetingSource == "" {
		return fmt.Errorf("--source flag is required")
	}

	// Determine tenant ID
	tenantID := ingestTenantID
	if tenantID == "" {
		tenantID = cfg.TenantID
	}
	if tenantID == "" {
		tenantID = "default"
	}

	// Display startup message
	fmt.Printf("Meeting Ingest: %s\n", path)
	fmt.Printf("  Source:      %s\n", meetingSource)
	fmt.Printf("  Platform:    %s\n", meetingPlatform)
	fmt.Printf("  Tenant:      %s\n", tenantID)
	if meetingDryRun {
		fmt.Printf("  Mode:        DRY RUN (no changes will be made)\n")
	}
	if info.IsDir() {
		fmt.Printf("  Path type:   directory\n")
	} else {
		fmt.Printf("  Path type:   single file\n")
	}
	fmt.Println()

	// Scan for meetings
	fmt.Printf("Scanning for meetings...\n")
	meetings, err := meeting.ScanMeetingFiles(path)
	if err != nil {
		return fmt.Errorf("scanning for meetings: %w", err)
	}

	if len(meetings) == 0 {
		fmt.Println("No meetings found.")
		return nil
	}

	fmt.Printf("Found %d meeting(s)\n\n", len(meetings))

	if meetingDryRun {
		// Just show what would be imported
		for i, m := range meetings {
			fmt.Printf("%d. %s (%s)\n", i+1, m.Title, m.Date.Format("2006-01-02"))
			if m.Files.TranscriptPath != "" {
				fmt.Printf("   Transcript: %s\n", m.Files.TranscriptPath)
			}
			if m.Files.ChatPath != "" {
				fmt.Printf("   Chat: %s\n", m.Files.ChatPath)
			}
			if m.Files.VideoPath != "" {
				fmt.Printf("   Video: %s\n", m.Files.VideoPath)
			}
		}
		fmt.Println("\nDry run complete. No changes made.")
		return nil
	}

	// Initialize database connection
	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Create logger
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().
		Timestamp().
		Str("component", "meeting_ingest").
		Logger()

	// Process each meeting
	startTime := time.Now()
	var importedCount, failedCount int

	for i, m := range meetings {
		fmt.Printf("[%d/%d] Processing: %s\n", i+1, len(meetings), m.Title)

		err := processMeeting(ctx, pool, logger, m, tenantID, meetingSource, meetingPlatform)
		if err != nil {
			logger.Error().Err(err).Str("meeting", m.Title).Msg("Failed to process meeting")
			fmt.Printf("  ERROR: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("  OK\n")
			importedCount++
		}
	}

	// Display results
	duration := time.Since(startTime)
	fmt.Println()
	fmt.Println("Ingest Complete")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Total:       %d\n", len(meetings))
	fmt.Printf("  Imported:    \033[32m%d\033[0m\n", importedCount)
	fmt.Printf("  Failed:      \033[31m%d\033[0m\n", failedCount)
	fmt.Printf("  Duration:    %s\n", formatDuration(duration))

	if failedCount > 0 {
		return fmt.Errorf("%d meetings failed to import", failedCount)
	}

	return nil
}

// processMeeting processes a single meeting and stores it in the database.
func processMeeting(ctx context.Context, pool *pgxpool.Pool, logger zerolog.Logger, m *meeting.Meeting, tenantID, sourceTag, platform string) error {
	// Resolve tenant ID
	resolvedTenantID := tenantID
	if resolvedTenantID == "" || resolvedTenantID == "default" {
		resolvedTenantID = DefaultTenantID
	}

	// Parse transcript if available
	var transcriptResult *meeting.TranscriptResult
	if m.Files.TranscriptPath != "" {
		f, err := os.Open(m.Files.TranscriptPath)
		if err != nil {
			return fmt.Errorf("opening transcript: %w", err)
		}
		defer f.Close()

		// Detect format and parse
		if strings.HasSuffix(strings.ToLower(m.Files.TranscriptPath), ".vtt") {
			transcriptResult, err = meeting.ParseVTT(f)
		} else {
			transcriptResult, err = meeting.ParseTXTTranscript(f)
		}
		if err != nil {
			return fmt.Errorf("parsing transcript: %w", err)
		}
		m.Transcript = transcriptResult
		m.Participants = transcriptResult.Speakers
		if m.DurationSeconds == 0 {
			m.DurationSeconds = transcriptResult.DurationSeconds
		}
	}

	// Parse chat if available
	var chatResult *meeting.ChatResult
	if m.Files.ChatPath != "" {
		f, err := os.Open(m.Files.ChatPath)
		if err != nil {
			return fmt.Errorf("opening chat: %w", err)
		}
		defer f.Close()

		chatResult, err = meeting.ParseChatLog(f)
		if err != nil {
			return fmt.Errorf("parsing chat: %w", err)
		}
		m.Chat = chatResult

		// Merge chat participants with transcript participants
		for _, speaker := range chatResult.Speakers {
			found := false
			for _, p := range m.Participants {
				if p == speaker {
					found = true
					break
				}
			}
			if !found {
				m.Participants = append(m.Participants, speaker)
			}
		}
	}

	// Start transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert meeting record
	participantsJSON := "[]"
	if len(m.Participants) > 0 {
		participantsJSON = `["` + strings.Join(m.Participants, `","`) + `"]`
	}

	// Normalize title for search
	normalizedTitle := meeting.NormalizeTitle(m.Title)

	var meetingID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO meetings (
			tenant_id, title, normalized_title, meeting_date, platform, duration_seconds,
			participant_count, participants, source_tag, source_path,
			has_transcript, has_chat, has_video, has_audio,
			processing_status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
		RETURNING id
	`,
		resolvedTenantID,
		m.Title,
		normalizedTitle,
		m.Date,
		platform,
		m.DurationSeconds,
		len(m.Participants),
		participantsJSON,
		sourceTag,
		m.Files.TranscriptPath,
		m.Files.TranscriptPath != "",
		m.Files.ChatPath != "",
		m.Files.VideoPath != "",
		m.Files.AudioPath != "",
		"pending",
	).Scan(&meetingID)
	if err != nil {
		return fmt.Errorf("inserting meeting: %w", err)
	}

	logger.Info().Int64("meeting_id", meetingID).Str("title", m.Title).Msg("Created meeting record")

	// Insert transcript as source if available
	if transcriptResult != nil && transcriptResult.FullText != "" {
		// Generate external_id from meeting ID and file path
		externalID := fmt.Sprintf("meeting-%d-transcript", meetingID)

		// Compute SHA256 hash
		hash := sha256.Sum256([]byte(transcriptResult.FullText))
		contentHash := hex.EncodeToString(hash[:])

		var sourceID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO sources (
				tenant_id, meeting_id, source_system, external_id, content_type,
				raw_content, content_hash, processing_status,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			RETURNING id
		`,
			resolvedTenantID,
			meetingID,
			"meeting_transcript",
			externalID,
			"text/plain",
			transcriptResult.FullText,
			contentHash,
			"pending",
		).Scan(&sourceID)
		if err != nil {
			return fmt.Errorf("inserting transcript source: %w", err)
		}

		logger.Info().Int64("source_id", sourceID).Msg("Created transcript source")
	}

	// Insert chat as source if available
	if chatResult != nil && len(chatResult.Messages) > 0 {
		// Build chat text
		var chatText strings.Builder
		for _, msg := range chatResult.Messages {
			chatText.WriteString(fmt.Sprintf("%s: %s\n", msg.Speaker, msg.Message))
		}

		// Generate external_id from meeting ID
		externalID := fmt.Sprintf("meeting-%d-chat", meetingID)

		// Compute SHA256 hash
		chatHash := sha256.Sum256([]byte(chatText.String()))
		chatContentHash := hex.EncodeToString(chatHash[:])

		var sourceID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO sources (
				tenant_id, meeting_id, source_system, external_id, content_type,
				raw_content, content_hash, processing_status,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			RETURNING id
		`,
			resolvedTenantID,
			meetingID,
			"meeting_chat",
			externalID,
			"text/plain",
			chatText.String(),
			chatContentHash,
			"pending",
		).Scan(&sourceID)
		if err != nil {
			return fmt.Errorf("inserting chat source: %w", err)
		}

		logger.Info().Int64("source_id", sourceID).Msg("Created chat source")
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	// Detect acronyms in transcript and queue for review
	if transcriptResult != nil && transcriptResult.FullText != "" {
		detectAndQueueAcronyms(ctx, pool, logger, transcriptResult, meetingID, m.Title, resolvedTenantID)
	}

	return nil
}

// detectAndQueueAcronyms detects unknown acronyms in transcript and queues them for review.
func detectAndQueueAcronyms(ctx context.Context, pool *pgxpool.Pool, logger zerolog.Logger, transcript *meeting.TranscriptResult, meetingID int64, meetingTitle, tenantID string) {
	// Load known terms from glossary
	glossaryRepo := glossary.NewRepository(pool)
	terms, err := glossaryRepo.List(ctx, glossary.TermFilter{Limit: 1000})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to load glossary terms for acronym detection")
		return
	}

	// Create detector with known terms
	detector := meeting.NewAcronymDetector()
	knownTerms := make([]string, len(terms))
	for i, t := range terms {
		knownTerms[i] = t.Term
	}
	detector.SetKnownTerms(knownTerms)

	// Detect acronyms (require at least 1 occurrence)
	acronyms := detector.DetectInTranscript(transcript, 1)

	if len(acronyms) == 0 {
		return
	}

	// Queue acronyms for review
	reviewRepo := reviewqueue.NewRepository(pool)
	sourceRef := fmt.Sprintf("Meeting: %s", meetingTitle)

	var queued int
	for _, acr := range acronyms {
		question := reviewqueue.AcronymQuestion{
			Term:            acr.Term,
			Context:         acr.Context,
			SourceType:      "meeting",
			SourceID:        meetingID,
			SourceReference: sourceRef,
			Confidence:      calculateAcronymConfidence(acr),
		}

		_, created, err := reviewRepo.CreateIfNotExists(ctx, question.ToInput())
		if err != nil {
			logger.Warn().Err(err).Str("term", acr.Term).Msg("Failed to queue acronym for review")
			continue
		}
		if created {
			queued++
			logger.Debug().Str("term", acr.Term).Int("count", acr.Count).Msg("Queued acronym for review")
		}
	}

	if queued > 0 {
		logger.Info().Int("queued", queued).Int("total", len(acronyms)).Msg("Queued acronyms for review")
	}
}

// calculateAcronymConfidence estimates confidence that a detected term is a meaningful acronym.
func calculateAcronymConfidence(acr meeting.DetectedAcronym) float64 {
	// Base confidence
	confidence := 0.5

	// Higher confidence for terms appearing multiple times
	if acr.Count >= 3 {
		confidence += 0.2
	} else if acr.Count >= 2 {
		confidence += 0.1
	}

	// Higher confidence for longer acronyms (less likely to be noise)
	if len(acr.Term) >= 4 {
		confidence += 0.1
	}

	// Cap at 0.9
	if confidence > 0.9 {
		confidence = 0.9
	}

	return confidence
}

// runResolveMeetingParticipants resolves meeting participants to people.
func runResolveMeetingParticipants(ctx context.Context, deps *IngestCommandDeps) error {
	// Load configuration
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Determine tenant ID
	tenantID := ingestTenantID
	if tenantID == "" {
		tenantID = cfg.TenantID
	}
	if tenantID == "" || tenantID == "default" {
		tenantID = DefaultTenantID
	}

	fmt.Printf("Resolving Meeting Participants\n")
	fmt.Printf("  Tenant: %s\n\n", tenantID)

	// Initialize database connection
	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Load all people for resolution
	fmt.Println("Loading people from database...")
	people, err := loadPeople(ctx, pool, tenantID)
	if err != nil {
		return fmt.Errorf("loading people: %w", err)
	}
	fmt.Printf("  Found %d people\n\n", len(people))

	if len(people) == 0 {
		fmt.Println("No people found. Add people to the database first.")
		return nil
	}

	// Create resolver
	resolver := meeting.NewParticipantResolver(people)

	// Load meetings with participants
	fmt.Println("Loading meetings...")
	var sourceFilter string
	if meetingSource != "" {
		sourceFilter = " AND source_tag = $2"
	}

	query := `SELECT id, title, participants FROM meetings WHERE tenant_id = $1` + sourceFilter + ` ORDER BY id`
	var rows interface{ Close() }
	var meetingRows interface {
		Next() bool
		Scan(dest ...interface{}) error
		Err() error
	}

	if meetingSource != "" {
		r, err := pool.Query(ctx, query, tenantID, meetingSource)
		if err != nil {
			return fmt.Errorf("querying meetings: %w", err)
		}
		rows = r
		meetingRows = r
	} else {
		r, err := pool.Query(ctx, query, tenantID)
		if err != nil {
			return fmt.Errorf("querying meetings: %w", err)
		}
		rows = r
		meetingRows = r
	}
	defer rows.Close()

	var totalMeetings, totalParticipants, totalMatched int

	for meetingRows.Next() {
		var meetingID int64
		var title string
		var participantsJSON []byte

		if err := meetingRows.Scan(&meetingID, &title, &participantsJSON); err != nil {
			return fmt.Errorf("scanning meeting: %w", err)
		}

		// Parse participants JSON array
		var participants []string
		if len(participantsJSON) > 0 {
			// Simple JSON array parsing
			participantsStr := string(participantsJSON)
			participantsStr = strings.TrimPrefix(participantsStr, "[")
			participantsStr = strings.TrimSuffix(participantsStr, "]")
			if participantsStr != "" {
				for _, p := range strings.Split(participantsStr, ",") {
					p = strings.TrimSpace(p)
					p = strings.Trim(p, "\"")
					if p != "" {
						participants = append(participants, p)
					}
				}
			}
		}

		if len(participants) == 0 {
			continue
		}

		totalMeetings++

		// Resolve participants
		results := resolver.ResolveAll(participants)
		stats := results.Stats()

		totalParticipants += stats.Total
		totalMatched += stats.Matched

		// Insert into meeting_participants table
		for _, result := range results {
			var personID *int64
			var matchType *string
			var confidence *float64

			if result.Match != nil {
				personID = &result.Match.PersonID
				mt := string(result.Match.MatchType)
				matchType = &mt
				confidence = &result.Match.Confidence
			}

			_, err := pool.Exec(ctx, `
				INSERT INTO meeting_participants (
					tenant_id, meeting_id, person_id, display_name, match_type, confidence
				) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (meeting_id, display_name) DO UPDATE SET
					person_id = EXCLUDED.person_id,
					match_type = EXCLUDED.match_type,
					confidence = EXCLUDED.confidence,
					updated_at = NOW()
			`, tenantID, meetingID, personID, result.DisplayName, matchType, confidence)

			if err != nil {
				return fmt.Errorf("inserting participant %s for meeting %d: %w", result.DisplayName, meetingID, err)
			}
		}

		fmt.Printf("  [%d] %s: %d/%d matched\n", meetingID, truncateMeetingTitle(title, 40), stats.Matched, stats.Total)
	}

	if err := meetingRows.Err(); err != nil {
		return fmt.Errorf("iterating meetings: %w", err)
	}

	// Print summary
	fmt.Println()
	fmt.Println("Resolution Complete")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Meetings:     %d\n", totalMeetings)
	fmt.Printf("  Participants: %d\n", totalParticipants)
	fmt.Printf("  Matched:      \033[32m%d\033[0m\n", totalMatched)
	fmt.Printf("  Unmatched:    \033[33m%d\033[0m\n", totalParticipants-totalMatched)
	if totalParticipants > 0 {
		fmt.Printf("  Match Rate:   %.1f%%\n", float64(totalMatched)/float64(totalParticipants)*100)
	}

	return nil
}

// loadPeople loads all people from the database for entity resolution.
func loadPeople(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]meeting.Person, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, canonical_name, aliases
		FROM people
		WHERE tenant_id = $1 AND (is_deleted = false OR is_deleted IS NULL)
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var people []meeting.Person
	for rows.Next() {
		var id int64
		var canonicalName string
		var aliases []string

		if err := rows.Scan(&id, &canonicalName, &aliases); err != nil {
			return nil, err
		}

		people = append(people, meeting.Person{
			ID:            id,
			CanonicalName: canonicalName,
			Aliases:       aliases,
		})
	}

	return people, rows.Err()
}

// truncateMeetingTitle truncates a meeting title to maxLen characters.
func truncateMeetingTitle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// newExtractMeetingMentionsCommand creates the 'ingest meeting mentions' subcommand.
func newExtractMeetingMentionsCommand(deps *IngestCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "mentions",
		Short: "Extract mentions of people from meeting transcripts",
		Long: `Extract mentions of known people from meeting transcript content.

This identifies people who were discussed/mentioned in meetings (distinct from
attendees who spoke). Useful for queries like "meetings where Rishi was mentioned".

Attendees are excluded from mention extraction to avoid false positives.

Examples:
  # Extract mentions from all meetings
  penf ingest meeting mentions`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtractMeetingMentions(cmd.Context(), deps)
		},
	}
}

// runExtractMeetingMentions extracts mentions of people from meeting transcripts.
func runExtractMeetingMentions(ctx context.Context, deps *IngestCommandDeps) error {
	// Load configuration
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Determine tenant ID
	tenantID := ingestTenantID
	if tenantID == "" {
		tenantID = cfg.TenantID
	}
	if tenantID == "" || tenantID == "default" {
		tenantID = DefaultTenantID
	}

	fmt.Printf("Extracting Meeting Mentions\n")
	fmt.Printf("  Tenant: %s\n\n", tenantID)

	// Initialize database connection
	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Load all people for extraction
	fmt.Println("Loading people from database...")
	people, err := loadPeople(ctx, pool, tenantID)
	if err != nil {
		return fmt.Errorf("loading people: %w", err)
	}
	fmt.Printf("  Found %d people\n\n", len(people))

	if len(people) == 0 {
		fmt.Println("No people found. Add people to the database first.")
		return nil
	}

	// Create mention extractor
	extractor := meeting.NewMentionExtractor(people)

	// Load meetings with transcript sources
	fmt.Println("Processing meeting transcripts...")
	rows, err := pool.Query(ctx, `
		SELECT m.id, m.title, s.id as source_id, s.raw_content
		FROM meetings m
		JOIN sources s ON s.meeting_id = m.id
		WHERE m.tenant_id = $1
		  AND s.source_system = 'meeting_transcript'
		  AND s.raw_content IS NOT NULL
		ORDER BY m.id
	`, tenantID)
	if err != nil {
		return fmt.Errorf("querying meetings: %w", err)
	}
	defer rows.Close()

	var totalMeetings, totalMentions int

	for rows.Next() {
		var meetingID, sourceID int64
		var title, rawContent string

		if err := rows.Scan(&meetingID, &title, &sourceID, &rawContent); err != nil {
			return fmt.Errorf("scanning meeting: %w", err)
		}

		// Get attendee IDs to exclude from mentions
		attendeeIDs, err := getAttendeeIDs(ctx, pool, meetingID)
		if err != nil {
			return fmt.Errorf("getting attendees for meeting %d: %w", meetingID, err)
		}

		// Extract mentions (excluding attendees)
		mentions := extractor.ExtractExcluding(rawContent, attendeeIDs)

		if len(mentions) == 0 {
			continue
		}

		totalMeetings++
		totalMentions += len(mentions)

		// Insert mentions into database
		for _, mention := range mentions {
			_, err := pool.Exec(ctx, `
				INSERT INTO meeting_mentions (
					tenant_id, meeting_id, source_id, person_id,
					matched_text, match_type, context, mention_count
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (meeting_id, person_id) DO UPDATE SET
					matched_text = EXCLUDED.matched_text,
					match_type = EXCLUDED.match_type,
					context = EXCLUDED.context,
					mention_count = meeting_mentions.mention_count + EXCLUDED.mention_count,
					updated_at = NOW()
			`, tenantID, meetingID, sourceID, mention.PersonID,
				mention.MatchedText, string(mention.MatchType), mention.Context, mention.Count)

			if err != nil {
				return fmt.Errorf("inserting mention for meeting %d: %w", meetingID, err)
			}
		}

		// Show progress
		mentionNames := make([]string, len(mentions))
		for i, m := range mentions {
			mentionNames[i] = m.CanonicalName
		}
		fmt.Printf("  [%d] %s: %d mentions (%s)\n",
			meetingID, truncateMeetingTitle(title, 35), len(mentions), strings.Join(mentionNames, ", "))
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating meetings: %w", err)
	}

	// Print summary
	fmt.Println()
	fmt.Println("Mention Extraction Complete")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Meetings with mentions: %d\n", totalMeetings)
	fmt.Printf("  Total mentions:         %d\n", totalMentions)

	return nil
}

// getAttendeeIDs returns the person IDs of attendees for a meeting.
func getAttendeeIDs(ctx context.Context, pool *pgxpool.Pool, meetingID int64) (map[int64]bool, error) {
	rows, err := pool.Query(ctx, `
		SELECT person_id FROM meeting_participants
		WHERE meeting_id = $1 AND person_id IS NOT NULL
	`, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[int64]bool)
	for rows.Next() {
		var personID int64
		if err := rows.Scan(&personID); err != nil {
			return nil, err
		}
		ids[personID] = true
	}

	return ids, rows.Err()
}
