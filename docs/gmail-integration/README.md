# Gmail Integration Documentation

This directory contains comprehensive documentation for Penfold's Gmail integration feature, providing everything needed to set up, use, develop, and troubleshoot Gmail connectivity.

## Documentation Overview

### User Documentation
- **[Setup Guide](./setup-guide.md)** - Complete walkthrough for connecting Gmail accounts, configuring OAuth2, and setting up real-time monitoring
- **[Troubleshooting Guide](./troubleshooting.md)** - Comprehensive solutions for common issues and diagnostic procedures

### Developer Documentation
- **[Architecture Guide](./architecture.md)** - Technical overview of Gmail integration architecture, components, and design patterns
- **[API Reference](./api-reference.md)** - Complete API documentation for all Gmail integration components with examples

### Context Documentation
- **[Integration Patterns](../context/integration-dev/agents.md)** - Proven patterns for developing Penfold integrations
- **[System Architecture](../ARCHITECTURE.md)** - Overall Penfold system architecture including Gmail integration patterns

## Quick Start

### For End Users
1. **Setup**: Follow the [Setup Guide](./setup-guide.md) to connect your Gmail account
2. **Configuration**: Configure privacy filters and sync preferences
3. **Verification**: Test the integration with real-time monitoring
4. **Usage**: Start using `penf ask` to query your email data

### For Developers
1. **Architecture**: Read the [Architecture Guide](./architecture.md) to understand the system design
2. **API Reference**: Use the [API Reference](./api-reference.md) for implementation details
3. **Patterns**: Follow [Integration Patterns](../context/integration-dev/agents.md) for consistent development
4. **Testing**: Implement comprehensive tests using the provided patterns

## Feature Overview

Gmail integration enables Penfold to:

- **Secure Connection**: OAuth2 PKCE authentication with AES-256-GCM encrypted credential storage
- **Historical Import**: Batch import of existing emails with configurable date ranges
- **Real-time Sync**: Live monitoring of new emails using Gmail Push notifications via Cloud Pub/Sub
- **Privacy Controls**: Configurable filtering based on labels, senders, domains, and PII detection
- **Attachment Processing**: Automatic download and content extraction from common file types
- **Multi-Account Support**: Handle multiple Gmail accounts with intelligent prioritization
- **gRPC Service**: Integration with Penfold's microservice architecture

## Key Components

### Go Service Architecture

| Component | Location | Description |
|-----------|----------|-------------|
| Main Entry | `services/gmail/main.go` | gRPC service entry point |
| OAuth2 PKCE | `services/gmail/oauth/` | Authentication with encrypted token storage |
| Sync Engine | `services/gmail/sync/` | Full and incremental sync with History API |
| Push Handler | `services/gmail/push/` | Cloud Pub/Sub notification processing |
| Attachments | `services/gmail/attachment/` | Processing and text extraction |
| Privacy Filter | `services/gmail/privacy/` | PII detection and content filtering |
| Scheduler | `services/gmail/scheduler/` | Multi-account priority scheduling |
| Config | `services/gmail/config/` | Service configuration |
| Server | `services/gmail/server/` | gRPC service implementation |

### Authentication System
- OAuth2 PKCE authorization code flow
- AES-256-GCM encrypted credential storage
- Automatic token refresh with configurable margin
- Multi-tenant credential isolation

### Synchronization Engine
- Incremental sync with Gmail History API
- Full sync with resumable state
- Priority-based multi-account scheduling
- Rate limiting compliance (250 quota units/second)
- Robust error recovery and exponential backoff

### Real-time Monitoring
- Gmail Push notification handling via Cloud Pub/Sub
- Webhook signature verification
- Polling fallback for reliability
- Notification deduplication

### Privacy and Security
- Configurable sensitivity levels (Low/Medium/High)
- PII detection with regex-based rules
- Sender/domain blocklists and allowlists
- Content redaction with audit logging

### Performance Features
- Concurrent batch processing
- Connection pooling
- Token bucket rate limiting
- Background attachment processing

## Configuration Examples

### Environment Variables
```bash
# Service configuration
export GMAIL_GRPC_PORT=50051
export GMAIL_HTTP_PORT=8081
export GMAIL_OAUTH_CREDENTIALS_PATH=/path/to/credentials.json
export GMAIL_TOKEN_STORE_PATH=/path/to/tokens
export GMAIL_MAX_SYNC_BATCH_SIZE=500
export GMAIL_SYNC_TIMEOUT_SECONDS=300
```

### Privacy Configuration
```go
config := &privacy.FilterConfig{
    SensitivityLevel:     privacy.SensitivityMedium,
    RedactionPlaceholder: "[REDACTED]",
    BlockedSenders:       []string{"spam@example.com"},
    BlockedDomains:       []string{"spam.example.com"},
    AllowedDomains:       []string{"company.com"},
}
```

## Common Use Cases

### Personal Knowledge Management
- Import and analyze personal email history
- Query for specific conversations or topics
- Track project communications over time
- Discover forgotten context and decisions

### Business Communication Analysis
- Multiple account management (work, personal, clients)
- Priority-based synchronization
- Privacy filtering for sensitive content
- Integration with project management workflows

### Compliance and Audit
- Complete communication history preservation
- Audit trail for all privacy filtering decisions
- Data retention policy enforcement
- Export capabilities for compliance reporting

## Performance Characteristics

### Throughput
- **Historical Import**: 100+ emails/minute
- **Real-time Detection**: <60 seconds average latency
- **Attachment Processing**: 90% success rate for files <25MB
- **Vector Search**: <500ms for 100K embeddings

### Scalability
- **Multi-Account**: Up to 5 accounts without degradation
- **Concurrent Processing**: 10+ simultaneous batch operations
- **Database**: Optimized for 100K+ emails per account
- **Rate Limiting**: Intelligent quota distribution across accounts

## Security and Privacy

### Data Protection
- OAuth2 tokens encrypted with AES-256-GCM
- No plaintext password storage
- Configurable data retention periods
- Local processing for sensitive content

### Privacy Controls
- Three sensitivity levels for PII detection
- Sender/domain blocklists and allowlists
- Content redaction with configurable placeholder
- Comprehensive audit logging

### Compliance Features
- GDPR-compliant data deletion
- SOX-compliant audit logging
- Complete data export capabilities
- Configurable retention policies

## Development Standards

### Code Quality
- Go 1.22+ with standard formatting (`gofmt`)
- 80%+ test coverage requirement
- Zero warnings from `go vet` and `staticcheck`
- Comprehensive error handling

### Testing Strategy
- Unit tests for all components
- Integration tests with mock Gmail API
- Performance benchmarks with clear targets
- Security tests for authentication and encryption

### Documentation Requirements
- API documentation with usage examples
- Architecture documentation with diagrams
- User guides with step-by-step instructions
- Troubleshooting guides with diagnostic commands

## Support and Maintenance

### Monitoring
- Prometheus metrics for all operations
- Health check endpoints (`/health`, `/ready`, `/live`)
- Performance metrics collection
- API quota usage tracking

### Diagnostics
- Built-in diagnostic command (`penf gmail diagnostic`)
- Structured logging with correlation IDs
- Network connectivity testing
- Configuration validation tools

### Maintenance Procedures
- OAuth2 token rotation
- Database optimization and cleanup
- Performance tuning based on usage patterns
- Security updates and vulnerability patching

## Contributing

When extending Gmail integration:

1. **Follow Patterns**: Use established patterns from [Integration Patterns](../context/integration-dev/agents.md)
2. **Test Thoroughly**: Implement comprehensive tests including error scenarios
3. **Document Changes**: Update relevant documentation with new features
4. **Security Review**: Ensure all changes maintain security standards
5. **Performance Testing**: Verify changes meet performance requirements

## Related Documentation

- **[Penfold System Architecture](../ARCHITECTURE.md)** - Overall system design and integration points
- **[Event Processing Framework](../specs/002-event-processing/)** - Event-driven architecture details
- **[Database Schema](../specs/001-database-schema/)** - Storage layer implementation
- **[AI Architecture](../specs/revised/ai-architecture.md)** - AI processing pipeline integration

## Version Information

- **Current Version**: 2.0.0 (Go Migration Complete)
- **Go Compatibility**: 1.22+
- **Database Support**: PostgreSQL 16+ with pgvector
- **Gmail API Version**: v1
- **OAuth2 Specification**: RFC 6749 with PKCE (RFC 7636)

This documentation provides everything needed to successfully implement, deploy, and maintain Gmail integration in the Penfold system. For additional support, see the troubleshooting guide or consult the development patterns documentation.
