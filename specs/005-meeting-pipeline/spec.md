# Feature Specification: Meeting Upload and Processing Pipeline

> **Status:** PARTIALLY IMPLEMENTED
> **Current state:** See Context Palace `penfold-arch-ingest`
> **This spec covers:** Transcription with speaker ID, AI-driven summary generation, quality scoring, large file resumable upload — basic transcript ingest works

**Feature Branch**: `005-meeting-pipeline`
**Created**: 2026-01-13
**Input**: User description: "Meeting Upload and Processing Pipeline"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Meeting File Upload and Intake (Priority: P1)

As a business user, I need to upload meeting recordings and documents to the system so that I can extract insights, summaries, and searchable content from my meetings without manual transcription work.

**Why this priority**: Foundation requirement - without file upload capability, no meeting content can be processed. This is the entry point for all meeting-based functionality.

**Independent Test**: Can be fully tested by uploading various meeting file types, verifying successful intake, and confirming files are queued for processing.

**Acceptance Scenarios**:

1. **Given** user has a meeting recording file, **When** they upload it through the interface, **Then** system accepts the file and queues it for processing with a tracking ID
2. **Given** user uploads multiple file formats (MP4, MP3, WAV, PDF, DOCX), **When** upload completes, **Then** system validates format compatibility and provides appropriate feedback
3. **Given** user uploads large meeting files, **When** processing begins, **Then** system provides progress tracking and estimated completion time

---

### User Story 2 - Meeting Transcription and Content Extraction (Priority: P1)

As the content processing system, I need to automatically transcribe audio/video meetings and extract structured information so that meeting content becomes searchable and analyzable like other business communications.

**Why this priority**: Core value delivery - transcription transforms audio/video into searchable, processable text content that integrates with the broader knowledge system.

**Independent Test**: Can be fully tested by processing uploaded audio/video files and verifying accurate transcription output with speaker identification and timestamp alignment.

**Acceptance Scenarios**:

1. **Given** audio/video meeting file is uploaded, **When** transcription processing runs, **Then** system produces accurate text with speaker identification and timestamps
2. **Given** meeting has multiple participants, **When** transcription completes, **Then** system identifies different speakers and segments dialogue appropriately
3. **Given** meeting contains technical terms or business jargon, **When** transcription processes, **Then** system maintains accuracy for domain-specific vocabulary

---

### User Story 3 - Meeting Summary and Insight Generation (Priority: P1)

As a busy professional, I need AI-generated summaries and key insights from meetings so that I can quickly understand decisions, action items, and important discussions without re-listening to entire recordings.

**Why this priority**: Primary user value - summaries and insights are the main reason users want meeting processing, enabling quick review and decision-making.

**Independent Test**: Can be fully tested by processing meetings with known outcomes and validating that summaries capture key decisions, action items, and participant contributions.

**Acceptance Scenarios**:

1. **Given** meeting transcription is complete, **When** summary generation runs, **Then** system produces concise summary highlighting decisions, action items, and key discussion points
2. **Given** meeting contains multiple topics, **When** insight generation processes, **Then** system organizes content by topic and identifies relationships to other business contexts
3. **Given** meeting includes follow-up commitments, **When** processing completes, **Then** system extracts actionable items with responsible parties and deadlines

---

### User Story 4 - Meeting Search and Discovery (Priority: P2)

As a knowledge worker, I need to search across all processed meetings to find specific discussions, decisions, or topics so that I can leverage historical meeting context for current work and decision-making.

**Why this priority**: Enhances the value of processed meetings by making them discoverable and useful for ongoing work, but basic processing can function without advanced search initially.

**Independent Test**: Can be fully tested by searching for known content across multiple processed meetings and verifying accurate, relevant results with proper ranking.

**Acceptance Scenarios**:

1. **Given** multiple meetings have been processed, **When** user searches for specific topics or decisions, **Then** system returns relevant meetings ranked by relevance with highlighted excerpts
2. **Given** user searches for participants or speakers, **When** search executes, **Then** system finds meetings featuring those individuals with context about their contributions
3. **Given** user needs to find follow-up discussions, **When** searching related topics, **Then** system identifies meeting threads and sequences on related subjects

---

### User Story 5 - Meeting Integration with Project Context (Priority: P2)

As a project manager, I need meeting insights automatically linked to relevant projects and team contexts so that meeting content enriches project knowledge and supports better project coordination.

**Why this priority**: Provides significant value by connecting meeting content to broader business context, but system can operate effectively with meetings as standalone content initially.

**Independent Test**: Can be fully tested by processing project-related meetings and verifying appropriate categorization and linking to existing project contexts.

**Acceptance Scenarios**:

1. **Given** meeting discusses specific projects, **When** processing completes, **Then** system links meeting content to relevant project contexts and updates project timelines
2. **Given** meeting involves team members from multiple projects, **When** insights are generated, **Then** system identifies cross-project implications and potential dependencies
3. **Given** meeting contains project decisions, **When** content is processed, **Then** system updates project status and flags changes requiring stakeholder attention

---

### Edge Cases

- What happens when audio quality is poor and transcription accuracy is low? (Resolved: System attempts transcription with quality warnings and manual correction options)
- How does the system handle meetings in multiple languages or with heavy accents?
- What occurs when meeting files are corrupted or in unsupported formats?
- How are extremely long meetings (4+ hours) processed efficiently?
- What happens when multiple speakers talk simultaneously or interrupt frequently?
- How does the system handle confidential meetings requiring special security treatment? (Resolved: User-specified privacy levels during upload with inheritance from defaults)
- What occurs when meeting content references documents or systems not available to the AI?

## Clarifications

### Session 2026-01-13

- Q: Transcription fallback strategy for poor audio quality → A: Attempt automated transcription with quality warnings and manual correction options
- Q: Speaker identity resolution for unknown voices → A: Create provisional speaker entities for unknown voices, allow later manual linking
- Q: Privacy control implementation for confidential meetings → A: User-specified privacy level during upload with inheritance from user/team defaults
- Q: Project context linking for failed automatic matching cases → A: Queue failed cases for manual review with AI-suggested project matches
- Q: Batch processing resource management when capacity is exceeded → A: First-in-first-out queue with configurable concurrent processing limits

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept multiple meeting file formats including MP4, MP3, WAV, MOV for audio/video and PDF, DOCX for supporting documents
- **FR-002**: System MUST provide automated transcription of audio/video content with speaker identification and timestamp alignment
- **FR-003**: System MUST generate structured summaries including key decisions, action items, and discussion topics
- **FR-004**: System MUST extract participant information and map to existing person entities in the system, creating provisional entities for unknown speakers that can be linked manually later
- **FR-005**: System MUST publish meeting content as processing events to the event framework for AI analysis
- **FR-006**: System MUST provide progress tracking for file processing with estimated completion times
- **FR-007**: System MUST support semantic search across meeting transcripts and summaries
- **FR-008**: System MUST link meeting insights to relevant project contexts automatically, with failed cases queued for manual review with AI-suggested matches
- **FR-009**: System MUST handle large file uploads (up to 2GB) with resumable upload capability
- **FR-010**: System MUST preserve meeting metadata including date, participants, duration, and file source information
- **FR-011**: System MUST support privacy controls for confidential meetings with user-specified privacy levels during upload and inheritance from user/team defaults
- **FR-012**: System MUST provide quality scores for transcription accuracy and summary completeness with user warnings when confidence falls below acceptable thresholds
- **FR-013**: System MUST enable manual correction of transcription errors and speaker identification
- **FR-014**: System MUST integrate with existing person and project entity resolution systems
- **FR-015**: System MUST support batch processing of multiple meeting files uploaded simultaneously using FIFO queue with configurable concurrent processing limits

### Key Entities

- **MeetingFile**: Uploaded audio/video or document file with processing status and metadata
- **MeetingTranscript**: Text transcription with speaker segments, timestamps, and confidence scores
- **MeetingSummary**: AI-generated summary with key points, decisions, and action items
- **MeetingParticipant**: Person entity participating in meeting with role and contribution tracking, including provisional entities for unknown speakers
- **MeetingTopic**: Identified discussion topic with relevance scoring and project linkage
- **ProcessingJob**: Background job tracking upload, transcription, and analysis progress with FIFO queue position
- **ReviewQueue**: Manual review tasks for failed automatic linking with AI-suggested matches
- **MeetingInsight**: Extracted business insight linking to broader organizational knowledge

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Meeting file upload completes successfully for files up to 2GB within 10 minutes including validation
- **SC-002**: Audio transcription achieves 95%+ accuracy for clear audio with identified speakers
- **SC-003**: Meeting summary generation completes within 30 minutes for 2-hour meetings
- **SC-004**: Search across meeting transcripts returns relevant results within 3 seconds for 1000+ meetings
- **SC-005**: Speaker identification accuracy reaches 90%+ for meetings with up to 8 participants
- **SC-006**: Meeting insights are automatically linked to correct projects in 85%+ of cases
- **SC-007**: System processes 20+ concurrent meeting uploads without performance degradation
- **SC-008**: Meeting content integration with existing knowledge base enables 40%+ faster information retrieval
- **SC-009**: Privacy controls successfully restrict access to confidential meetings for 100% of protected content
- **SC-010**: User satisfaction with meeting summary quality reaches 4.0/5.0 rating in usability testing

## Dependencies

- Event processing framework from [002-event-processing](../002-event-processing/spec.md) for meeting content publishing
- Database storage system from [001-database-schema](../001-database-schema/spec.md) for meeting metadata and transcripts
- AI coordination framework from [003-ai-coordination](../003-ai-coordination/spec.md) for multi-model processing
- Person and project entity resolution systems for context linking
- Speech-to-text transcription service (cloud-based for quality, local for privacy)
- File storage system for uploaded meeting files and processed outputs

## Assumptions

- Meeting files will typically be under 1GB with 2GB as maximum supported size
- Audio quality will be sufficient for transcription in 80%+ of uploaded meetings
- Users will have appropriate permissions to upload meetings they participated in
- Network bandwidth will support large file uploads in reasonable timeframes
- Speech-to-text services will remain available and cost-effective for expected meeting volume
- Meeting participants will speak clearly enough for speaker identification in most cases
- Business context and project relationships will be identifiable from meeting content
- Privacy requirements will be manageable through access controls and encryption