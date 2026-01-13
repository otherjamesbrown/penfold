# Implementation Plan: Gmail Integration with Event Publishing

**Branch**: `004-gmail-integration` | **Date**: 2026-01-13 | **Spec**: [Gmail Integration Spec](./spec.md)
**Input**: Feature specification from `/specs/004-gmail-integration/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Gmail Integration enables secure OAuth2 connection to Gmail accounts, real-time email synchronization, and historical email import with intelligent attachment processing. The system publishes email content as processing events to the event framework, supporting multiple accounts with privacy controls and rate limiting compliance. Core technical approach uses Gmail API with Push notifications for real-time sync, encrypted credential storage, and hybrid attachment processing with background queuing for complex formats.

## Technical Context

**Language/Version**: Python 3.12 (project standard)
**Primary Dependencies**:
- Google API Python Client (OAuth2, Gmail API)
- cryptography (AES-256 encryption for token storage)
- aiohttp/httpx (async HTTP for webhook endpoints)
- SQLAlchemy 2.0 (async ORM for credential/sync state storage)
- Redis (pub/sub for event publishing)
- Celery (background task processing for attachments)
- PyPDF2/python-docx (attachment content extraction)

**Storage**: PostgreSQL with pgvector (credentials, sync state, email metadata)
**Testing**: pytest with asyncio support, async fixtures for database/API testing
**Target Platform**: Mac Mini M4 with 32GB RAM (local development), Linux server deployment
**Project Type**: Single project - CLI tool with library components integrating into existing penf_lib structure
**Performance Goals**:
- OAuth2 connection: <2 minutes complete flow
- Historical import: 100+ emails/minute with API rate limit compliance
- Real-time sync: <60 seconds detection latency
- Attachment processing: 90% success rate for common formats <10MB

**Constraints**:
- Gmail API rate limits (250 quota units/user/second, 1 billion quota units/day)
- OAuth2 security requirements (encrypted storage, secure refresh flow)
- Real-time processing: <60 seconds for new message detection
- Privacy compliance: configurable exclusion filters
- Multi-account support: up to 5 accounts per user without degradation

**Scale/Scope**:
- Target: 1-10 Gmail accounts per user
- Volume: 10,000 emails per account typical, up to 100,000 historical
- Attachments: Common formats under 25MB (Gmail limit)
- Real-time: Continuous monitoring across multiple accounts
- Background processing: Queue management for deferred attachment extraction

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Value Validation ✅
- [x] **Time Savings**: Eliminates manual email checking/searching across accounts (transforms 3+ hours context reconstruction to 15 minutes)
- [x] **Pain Relief**: Addresses email fragmentation pain point where related business communications are scattered across accounts
- [x] **Frequency**: Email processing occurs daily, multiple times per day
- [x] **Criticality**: Email context is essential for business decision-making workflows

### Principle Alignment ✅
- [x] **Source Truth**: All email content preserved with metadata, attachments, and thread relationships intact
- [x] **Local-First**: OAuth2 and sync processing local, only cloud service is Gmail API itself (required)
- [x] **User Control**: Privacy filters, account prioritization, and categorization review workflows
- [x] **Evidence-Based**: Thread relationships, participant normalization, timestamp preservation

### ADHD-Friendly UX ✅
- [x] **Context Switching**: Event-driven architecture enables flexible integration with review workflows
- [x] **Cognitive Load**: Automated ingestion reduces manual email management burden
- [x] **Structured Browsing**: Thread preservation and temporal organization
- [x] **Clear Hierarchy**: Account prioritization and privacy filtering

### Technical Robustness ✅
- [x] **Scalability**: Handles projected 200+ emails/week per account, up to 5 accounts
- [x] **Performance**: Real-time sync <60 seconds, historical import 100+ emails/minute
- [x] **Reliability**: Push notification fallback to polling, encrypted credential recovery
- [x] **Maintainability**: Single project structure, established patterns, clear separation of concerns

### Learning Laboratory Criteria ✅
- [x] **Experimentation**: Email content provides rich dataset for AI model comparison
- [x] **Improvement**: Immutable storage allows reprocessing as AI capabilities advance
- [x] **Local Development**: All processing except Gmail API calls handled locally
- [x] **Real-World Testing**: Actual business email communication as training/test data

**CONSTITUTION PASSED** - No violations identified. Gmail integration aligns with core mission of contextual archaeology by providing complete email timeline reconstruction with audit trails.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
penf_lib/
├── connectors/
│   └── gmail/
│       ├── __init__.py
│       ├── auth.py           # OAuth2 flow and credential management
│       ├── client.py         # Gmail API client wrapper
│       ├── sync.py           # Sync operations and state management
│       ├── webhook.py        # Push notification handler
│       ├── attachments.py    # Attachment processing
│       └── privacy.py        # Privacy filtering implementation
├── models/
│   ├── gmail_connection.py   # SQLAlchemy models
│   ├── email_message.py
│   ├── email_thread.py
│   └── sync_operation.py
├── events/
│   ├── publishers.py         # Event publishing framework
│   └── schemas.py           # Event schema definitions
└── storage/
    ├── encryption.py         # Credential encryption utilities
    └── migrations/          # Database schema migrations

scripts/
├── gmail_setup.py           # Initial OAuth2 setup script
└── gmail_monitor.py         # Background monitoring daemon

tests/
├── unit/
│   ├── test_gmail_auth.py
│   ├── test_gmail_sync.py
│   ├── test_attachments.py
│   └── test_privacy.py
├── integration/
│   ├── test_gmail_api.py    # Full API integration tests
│   ├── test_event_publishing.py
│   └── test_multi_account.py
└── fixtures/
    ├── sample_emails.json
    └── test_credentials.py
```

**Structure Decision**: Single project structure integrating into existing penf_lib architecture. Gmail connector follows established patterns with separation of concerns: authentication, sync operations, event publishing, and privacy controls. Database models extend existing SQLAlchemy patterns, and testing follows async fixtures approach used elsewhere in the project.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
