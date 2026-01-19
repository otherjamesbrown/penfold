package batch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold/pkg/ingest/attachments"
	"github.com/otherjamesbrown/penfold/pkg/ingest/eml"
	"github.com/otherjamesbrown/penfold/pkg/ingest/events"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
)

// DefaultConcurrency is the default number of concurrent workers.
const DefaultConcurrency = 4

// ProcessorConfig configures the batch processor.
type ProcessorConfig struct {
	// Concurrency is the number of worker goroutines.
	Concurrency int

	// TenantID is the tenant to process for.
	TenantID string

	// SourceTag identifies the source of the emails.
	SourceTag string

	// Labels are optional labels to apply to all ingested emails.
	Labels []string

	// DryRun if true, validates files without persisting.
	DryRun bool

	// ResumeJobID if set, resumes an existing job.
	ResumeJobID string
}

// ProcessResult contains the result of a batch processing operation.
type ProcessResult struct {
	JobID         string
	TotalFiles    int
	ImportedCount int
	SkippedCount  int
	FailedCount   int
	StartedAt     time.Time
	CompletedAt   time.Time
	Success       bool
	Errors        []FileError
}

// FileError records an error for a specific file.
type FileError struct {
	FilePath string
	Error    string
}

// Processor handles batch processing of email files.
type Processor struct {
	cfg       ProcessorConfig
	repo      *storage.Repository
	publisher *events.Publisher
	parser    *eml.Parser
	extractor *attachments.Extractor
	logger    zerolog.Logger

	progress *Progress
	mu       sync.Mutex
}

// NewProcessor creates a new batch processor.
func NewProcessor(
	pool *pgxpool.Pool,
	redisClient *redis.Client,
	logger zerolog.Logger,
	cfg ProcessorConfig,
) *Processor {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = DefaultConcurrency
	}

	repo := storage.NewRepository(pool, logger)

	// Enable attachment content loading for extraction
	parseOpts := eml.DefaultParseOptions()
	parseOpts.IncludeAttachmentContent = true

	// Create attachment extractor
	extractor, err := attachments.NewExtractor(repo, logger)
	if err != nil {
		// Log warning but continue - attachment extraction will be disabled
		logger.Warn().Err(err).Msg("Failed to create attachment extractor, attachment extraction disabled")
	}

	proc := &Processor{
		cfg:       cfg,
		repo:      repo,
		publisher: events.NewPublisher(redisClient, logger),
		parser:    eml.NewParser(parseOpts),
		extractor: extractor,
		logger:    logger.With().Str("component", "batch_processor").Logger(),
	}

	// Set the processor as the embedded email handler for recursive processing
	if extractor != nil {
		extractor.SetEmbeddedEmailHandler(proc)
	}

	return proc
}

// Process processes all .eml files at the given path (file or directory).
func (p *Processor) Process(ctx context.Context, path string) (*ProcessResult, error) {
	// Discover files
	files, err := p.discoverFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files: %w", err)
	}

	if len(files) == 0 {
		return &ProcessResult{
			JobID:       "",
			TotalFiles:  0,
			Success:     true,
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
		}, nil
	}

	// Initialize or resume job
	jobID := p.cfg.ResumeJobID
	if jobID == "" {
		jobID = uuid.New().String()
	}

	// Filter files if resuming
	if p.cfg.ResumeJobID != "" {
		files, err = p.repo.GetRemainingFilesForJob(ctx, jobID, files)
		if err != nil {
			return nil, fmt.Errorf("failed to get remaining files: %w", err)
		}
		if len(files) == 0 {
			p.logger.Info().Msg("No remaining files to process")
			return &ProcessResult{
				JobID:       jobID,
				TotalFiles:  0,
				Success:     true,
				StartedAt:   time.Now(),
				CompletedAt: time.Now(),
			}, nil
		}
	}

	// Create job record
	if p.cfg.ResumeJobID == "" {
		job := &storage.IngestJob{
			ID:           jobID,
			TenantID:     p.cfg.TenantID,
			Status:       storage.IngestJobStatusInProgress,
			SourceTag:    p.cfg.SourceTag,
			ContentType:  "email",
			TotalFiles:   len(files),
			FileManifest: files,
			Options: map[string]interface{}{
				"labels":  p.cfg.Labels,
				"dry_run": p.cfg.DryRun,
			},
		}
		if !p.cfg.DryRun {
			if err := p.repo.CreateJob(ctx, job); err != nil {
				p.logger.Warn().Err(err).Msg("Failed to create job record")
			}
		}
	}

	// Initialize progress
	p.progress = NewProgress(len(files))
	p.progress.Start()

	result := &ProcessResult{
		JobID:      jobID,
		TotalFiles: len(files),
		StartedAt:  time.Now(),
		Errors:     []FileError{},
	}

	// Process files
	if p.cfg.Concurrency == 1 {
		p.processSequential(ctx, jobID, files, result)
	} else {
		p.processParallel(ctx, jobID, files, result)
	}

	result.CompletedAt = time.Now()
	result.Success = result.FailedCount == 0

	// Update job status
	if !p.cfg.DryRun {
		status := storage.IngestJobStatusCompleted
		if !result.Success {
			status = storage.IngestJobStatusFailed
		}
		if err := p.repo.CompleteJob(ctx, jobID, status); err != nil {
			p.logger.Warn().Err(err).Msg("Failed to update job status")
		}

		// Publish completion event
		if err := p.publisher.PublishJobCompleted(ctx, events.JobCompletedParams{
			JobID:         jobID,
			TenantID:      p.cfg.TenantID,
			SourceTag:     p.cfg.SourceTag,
			TotalFiles:    result.TotalFiles,
			ImportedCount: result.ImportedCount,
			SkippedCount:  result.SkippedCount,
			FailedCount:   result.FailedCount,
			StartedAt:     result.StartedAt,
			CompletedAt:   result.CompletedAt,
			Success:       result.Success,
			FinalStatus:   string(status),
		}); err != nil {
			p.logger.Warn().Err(err).Msg("Failed to publish completion event")
		}
	}

	p.progress.Complete(result.Success)

	return result, nil
}

// Progress returns the current progress tracker.
func (p *Processor) Progress() *Progress {
	return p.progress
}

// discoverFiles finds all .eml files at the given path.
func (p *Processor) discoverFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		// Single file
		if strings.HasSuffix(strings.ToLower(path), ".eml") {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			return []string{absPath}, nil
		}
		return nil, fmt.Errorf("file is not an .eml file: %s", path)
	}

	// Directory - walk recursively
	var files []string
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".eml") {
			absPath, err := filepath.Abs(p)
			if err != nil {
				return err
			}
			files = append(files, absPath)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// processSequential processes files one at a time.
func (p *Processor) processSequential(ctx context.Context, jobID string, files []string, result *ProcessResult) {
	for _, file := range files {
		if ctx.Err() != nil {
			p.progress.Cancel()
			return
		}

		p.progress.SetCurrentFile(file)
		outcome := p.processFile(ctx, jobID, file)
		p.recordOutcome(ctx, jobID, file, outcome, result)
	}
}

// processParallel processes files using a worker pool.
func (p *Processor) processParallel(ctx context.Context, jobID string, files []string, result *ProcessResult) {
	filesCh := make(chan string, len(files))
	resultsCh := make(chan fileOutcome, len(files))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < p.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range filesCh {
				if ctx.Err() != nil {
					resultsCh <- fileOutcome{file: file, outcome: outcomeSkipped}
					continue
				}
				p.progress.SetCurrentFile(file)
				outcome := p.processFile(ctx, jobID, file)
				resultsCh <- fileOutcome{file: file, outcome: outcome}
			}
		}()
	}

	// Send files to workers
	for _, file := range files {
		filesCh <- file
	}
	close(filesCh)

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	for fo := range resultsCh {
		p.recordOutcome(ctx, jobID, fo.file, fo.outcome, result)
	}
}

type fileOutcome struct {
	file    string
	outcome outcome
}

type outcome struct {
	status   string // "imported", "skipped", "failed"
	sourceID int64
	err      error
}

var (
	outcomeSkipped = outcome{status: "skipped"}
)

// processFile processes a single file and returns the outcome.
func (p *Processor) processFile(ctx context.Context, jobID, filePath string) outcome {
	// Parse the email
	parseResult, err := p.parser.ParseFile(filePath)
	if err != nil {
		p.logger.Error().Err(err).Str("file", filePath).Msg("Failed to parse email")
		return outcome{status: "failed", err: err}
	}

	email := parseResult.Email

	// Log warnings
	for _, w := range parseResult.Warnings {
		p.logger.Warn().Str("file", filePath).Str("warning", w).Msg("Parse warning")
	}

	// Check for duplicates
	isDup, existingID, reason, err := p.repo.CheckDuplicate(ctx, p.cfg.TenantID, email.MessageID, email.ContentHash)
	if err != nil {
		p.logger.Error().Err(err).Str("file", filePath).Msg("Failed to check duplicate")
		return outcome{status: "failed", err: err}
	}

	if isDup {
		p.logger.Debug().
			Str("file", filePath).
			Str("reason", reason).
			Int64("existing_id", existingID).
			Msg("Duplicate email skipped")
		return outcome{status: "skipped"}
	}

	// Dry run - stop here
	if p.cfg.DryRun {
		p.logger.Info().Str("file", filePath).Msg("Dry run: would import")
		return outcome{status: "imported"}
	}

	// Create source record with body text (not full raw content with base64 attachments)
	// This avoids PostgreSQL tsvector size limits for large attachments
	bodyContent := email.GetBody()
	if bodyContent == "" {
		bodyContent = email.Subject // Fallback to subject if no body
	}

	metadata := map[string]interface{}{
		"file_path":             filePath,
		"message_id_synthetic":  email.MessageIDSynthetic,
		"date_fallback":         email.DateFallback,
		"has_attachments":       email.HasAttachments(),
		"attachment_count":      email.AttachmentCount(),
		"source_tag":            p.cfg.SourceTag,
		"job_id":                jobID,
		"labels":                p.cfg.Labels,
		"from":                  email.From.Email,
		"to":                    email.ToAddresses(),
		"cc":                    email.CcAddresses(),
		"subject":               email.Subject,
	}
	if email.InReplyTo != "" {
		metadata["in_reply_to"] = email.InReplyTo
	}
	if len(email.References) > 0 {
		metadata["references"] = email.References
	}

	source := &storage.EmailSource{
		TenantID:        p.cfg.TenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      email.MessageID,
		ContentHash:     email.ContentHash,
		RawContent:      bodyContent,
		ContentType:     "message/rfc822",
		ContentSize:     int32(len(bodyContent)),
		Metadata:        metadata,
		SourceTimestamp: email.Date,
	}

	created, err := p.repo.CreateSource(ctx, source)
	if err != nil {
		p.logger.Error().Err(err).Str("file", filePath).Msg("Failed to create source")
		return outcome{status: "failed", err: err}
	}

	// Extract and store attachments
	var extractResult *attachments.ExtractionResult
	if p.extractor != nil && email.HasAttachments() {
		extractResult, err = p.extractor.ExtractAndStore(ctx, attachments.ExtractParams{
			TenantID:        p.cfg.TenantID,
			ParentSourceID:  created.ID,
			Email:           email,
			SourceTimestamp: email.Date,
		})
		if err != nil {
			p.logger.Warn().
				Err(err).
				Str("file", filePath).
				Int64("source_id", created.ID).
				Msg("Failed to extract attachments, email still imported")
			// Don't fail the email import, just log the warning
		}
	}

	// Publish email event
	if err := p.publisher.PublishManualEmailIngested(ctx, events.EmailIngestedParams{
		SourceID:  created.ID,
		TenantID:  p.cfg.TenantID,
		JobID:     jobID,
		Email:     email,
		SourceTag: p.cfg.SourceTag,
		Labels:    p.cfg.Labels,
	}); err != nil {
		p.logger.Warn().Err(err).Str("file", filePath).Msg("Failed to publish email event")
		// Don't fail the file, the source is already created
	}

	// Publish attachment events for processable attachments
	if extractResult != nil {
		for _, att := range extractResult.Attachments {
			if att.SourceID != nil && att.Classification != nil && att.Classification.Tier.IsProcessable() {
				if err := p.publisher.PublishAttachmentIngested(ctx, events.AttachmentIngestedParams{
					SourceID:       *att.SourceID,
					ParentSourceID: created.ID,
					TenantID:       p.cfg.TenantID,
					Filename:       att.Attachment.Filename,
					MimeType:       att.Attachment.MimeType,
					SizeBytes:      att.Attachment.SizeBytes,
					IsEmbeddedEmail: att.Attachment.IsEmbeddedEmail,
				}); err != nil {
					p.logger.Warn().
						Err(err).
						Str("filename", att.Attachment.Filename).
						Msg("Failed to publish attachment event")
				}
			}
		}
	}

	p.logger.Debug().
		Str("file", filePath).
		Int64("source_id", created.ID).
		Int("attachments_processed", func() int {
			if extractResult != nil {
				return extractResult.Processed
			}
			return 0
		}()).
		Msg("Email imported successfully")

	return outcome{status: "imported", sourceID: created.ID}
}

// recordOutcome updates progress and result based on the processing outcome.
func (p *Processor) recordOutcome(ctx context.Context, jobID, filePath string, o outcome, result *ProcessResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch o.status {
	case "imported":
		result.ImportedCount++
		p.progress.RecordImported()

	case "skipped":
		result.SkippedCount++
		p.progress.RecordSkipped()

	case "failed":
		result.FailedCount++
		result.Errors = append(result.Errors, FileError{
			FilePath: filePath,
			Error:    o.err.Error(),
		})
		p.progress.RecordFailed()

		// Record error in database
		if !p.cfg.DryRun {
			errorType := storage.ErrorTypeUnexpected
			errMsg := o.err.Error()
			if strings.Contains(errMsg, "parse") || strings.Contains(errMsg, "Parse") {
				errorType = storage.ErrorTypeParse
			} else if strings.Contains(errMsg, "encoding") {
				errorType = storage.ErrorTypeEncoding
			} else if strings.Contains(errMsg, "io") || strings.Contains(errMsg, "read") || strings.Contains(errMsg, "open") {
				errorType = storage.ErrorTypeIO
			} else if strings.Contains(errMsg, "validation") {
				errorType = storage.ErrorTypeValidation
			} else if strings.Contains(errMsg, "storage") || strings.Contains(errMsg, "database") {
				errorType = storage.ErrorTypeStorage
			}
			if err := p.repo.RecordError(ctx, jobID, filePath, errorType, errMsg, nil); err != nil {
				p.logger.Warn().Err(err).Msg("Failed to record error")
			}
		}
	}

	// Update job progress
	if !p.cfg.DryRun {
		// Collect processed files from progress tracker
		processedFiles := p.progress.ProcessedFiles()
		if err := p.repo.UpdateJobProgress(ctx, jobID,
			result.ImportedCount+result.SkippedCount+result.FailedCount,
			result.ImportedCount,
			result.SkippedCount,
			result.FailedCount,
			processedFiles,
		); err != nil {
			p.logger.Warn().Err(err).Msg("Failed to update job progress")
		}
	}
}

// HandleEmbeddedEmail implements attachments.EmbeddedEmailHandler for recursive email processing.
func (p *Processor) HandleEmbeddedEmail(ctx context.Context, params attachments.EmbeddedEmailParams) (*attachments.EmbeddedEmailResult, error) {
	email := params.Email

	// Check for duplicates
	isDup, existingID, reason, err := p.repo.CheckDuplicate(ctx, params.TenantID, email.MessageID, email.ContentHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate: %w", err)
	}

	if isDup {
		p.logger.Debug().
			Str("message_id", email.MessageID).
			Str("reason", reason).
			Int64("existing_id", existingID).
			Int("depth", params.Depth).
			Msg("Duplicate embedded email skipped")

		return &attachments.EmbeddedEmailResult{
			SourceID:   existingID,
			MessageID:  email.MessageID,
			WasSkipped: true,
			SkipReason: fmt.Sprintf("duplicate: %s", reason),
		}, nil
	}

	// Create source record with body text only
	bodyContent := email.GetBody()
	if bodyContent == "" {
		bodyContent = email.Subject
	}

	metadata := map[string]interface{}{
		"embedded_email":        true,
		"parent_source_id":      params.ParentSourceID,
		"embedded_filename":     params.Filename,
		"embedded_depth":        params.Depth,
		"message_id_synthetic":  email.MessageIDSynthetic,
		"date_fallback":         email.DateFallback,
		"has_attachments":       email.HasAttachments(),
		"attachment_count":      email.AttachmentCount(),
		"from":                  email.From.Email,
		"to":                    email.ToAddresses(),
		"cc":                    email.CcAddresses(),
		"subject":               email.Subject,
	}
	if email.InReplyTo != "" {
		metadata["in_reply_to"] = email.InReplyTo
	}
	if len(email.References) > 0 {
		metadata["references"] = email.References
	}

	source := &storage.EmailSource{
		TenantID:        params.TenantID,
		SourceSystem:    "embedded_email",
		ExternalID:      email.MessageID,
		ContentHash:     email.ContentHash,
		RawContent:      bodyContent,
		ContentType:     "message/rfc822",
		ContentSize:     int32(len(bodyContent)),
		Metadata:        metadata,
		SourceTimestamp: email.Date,
	}

	created, err := p.repo.CreateSource(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded email source: %w", err)
	}

	// Extract attachments from the embedded email (recursive)
	var attachmentCount int
	if p.extractor != nil && email.HasAttachments() {
		extractResult, err := p.extractor.ExtractAndStore(ctx, attachments.ExtractParams{
			TenantID:        params.TenantID,
			ParentSourceID:  created.ID,
			Email:           email,
			SourceTimestamp: email.Date,
			Depth:           params.Depth,
			SeenMessageIDs:  params.SeenMessageIDs,
		})
		if err != nil {
			p.logger.Warn().
				Err(err).
				Int64("source_id", created.ID).
				Int("depth", params.Depth).
				Msg("Failed to extract attachments from embedded email")
		} else {
			attachmentCount = extractResult.TotalCount
		}
	}

	// Publish event for the embedded email
	if err := p.publisher.PublishManualEmailIngested(ctx, events.EmailIngestedParams{
		SourceID:  created.ID,
		TenantID:  params.TenantID,
		JobID:     "", // No job ID for embedded emails
		Email:     email,
		SourceTag: "embedded",
		Labels:    []string{"embedded_email"},
	}); err != nil {
		p.logger.Warn().
			Err(err).
			Int64("source_id", created.ID).
			Msg("Failed to publish embedded email event")
	}

	p.logger.Debug().
		Int64("source_id", created.ID).
		Str("message_id", email.MessageID).
		Int("depth", params.Depth).
		Int("attachments", attachmentCount).
		Msg("Embedded email processed")

	return &attachments.EmbeddedEmailResult{
		SourceID:        created.ID,
		MessageID:       email.MessageID,
		AttachmentCount: attachmentCount,
		WasSkipped:      false,
	}, nil
}
