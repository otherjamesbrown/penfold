# Requirements Checklist: Manual Content Ingest Framework

## Ingest Framework Requirements

- [ ] **FR-001**: `penf ingest <type>` CLI pattern supporting multiple content types
- [ ] **FR-002**: Required `--source` tag for all manual uploads
- [ ] **FR-003**: Single file and directory/glob batch upload support
- [ ] **FR-004**: Progress bar with file count and percentage for batches
- [ ] **FR-005**: Summary report on completion (imported, skipped, failed)
- [ ] **FR-006**: Skip malformed files, continue batch, log errors
- [ ] **FR-007**: Respect active tenant context
- [ ] **FR-008**: `--dry-run` flag for preview

## Email Type Requirements

- [ ] **FR-100**: Parse .eml files (RFC 822/RFC 2822)
- [ ] **FR-101**: Extract metadata: From, To, Cc, Bcc, Subject, Date, Message-ID
- [ ] **FR-102**: Extract body (plain text and HTML)
- [ ] **FR-103**: Duplicate detection via Message-ID (silent skip)
- [ ] **FR-104**: Use original Date header for timeline
- [ ] **FR-105**: Thread reconstruction via In-Reply-To/References headers
- [ ] **FR-106**: Preserve folder structure as labels
- [ ] **FR-107**: Extract attachments as document type with email link
- [ ] **FR-108**: Archive original .eml file
- [ ] **FR-109**: Normalize participant email addresses
- [ ] **FR-110**: Use same email model as Gmail (source_system differentiator)
- [ ] **FR-111**: Support re-analysis (same as Gmail emails)
- [ ] **FR-112**: Trigger full AI extraction pipeline (assertions, entity resolution, summarization)
- [ ] **FR-113**: Generate embeddings for semantic search
- [ ] **FR-114**: Generate brief summary (1-2 sentences) for all content
- [ ] **FR-115**: Generate full summary based on complexity thresholds per content type

## Success Criteria

- [ ] **SC-001**: Single file upload < 5 seconds
- [ ] **SC-002**: 100 file batch < 2 minutes
- [ ] **SC-003**: 100% duplicate detection via Message-ID
- [ ] **SC-004**: 95% thread reconstruction accuracy
- [ ] **SC-005**: Unified search < 3 seconds
- [ ] **SC-006**: 90% attachment extraction (common formats)
- [ ] **SC-007**: Original .eml retrieval intact
- [ ] **SC-008**: Malformed files don't block batch
- [ ] **SC-009**: 100% folder structure label accuracy
- [ ] **SC-010**: Re-analysis treats manual same as Gmail

## User Stories

- [ ] **US-1**: Single email file upload (P1)
- [ ] **US-2**: Bulk email upload (P1)
- [ ] **US-3**: Duplicate handling (P1)
- [ ] **US-4**: Attachment extraction (P2)
- [ ] **US-5**: Unified search (P1)
- [ ] **US-6**: Original file archive (P2)

## Edge Cases

- [ ] Malformed/corrupt .eml files
- [ ] Missing Message-ID header
- [ ] Future/invalid dates
- [ ] Non-UTF8 encodings
- [ ] Attachment extraction failures
- [ ] Large attachments (>25MB)
- [ ] Interrupted batch uploads
