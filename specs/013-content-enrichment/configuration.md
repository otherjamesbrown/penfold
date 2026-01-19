# Tenant Configuration

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

Configuration is database-backed per tenant, managed via CLI. Designed for single-tenant now, multi-tenant later.

---

## Data Model

```
┌─────────────────────────┐
│       tenants           │
├─────────────────────────┤
│ id                      │
│ name                    │  ← "personal", "work-akamai", "side-project"
│ slug                    │  ← URL-safe identifier
│ created_at              │
│ is_active               │
└───────────┬─────────────┘
            │
┌───────────┴─────────────┐
│    tenant_domains       │
├─────────────────────────┤
│ tenant_id               │
│ domain                  │  ← "akamai.com"
│ domain_type             │  ← internal | external_known | external_unknown
│ notes                   │  ← "Primary work domain"
│ created_at              │
└─────────────────────────┘

┌─────────────────────────┐
│  tenant_email_patterns  │
├─────────────────────────┤
│ tenant_id               │
│ pattern                 │  ← "*-jira@*", "Prb-Facilitator@*"
│ pattern_type            │  ← bot | distribution_list | role_account | ignore
│ notes                   │
│ created_at              │
└─────────────────────────┘

┌─────────────────────────┐
│ tenant_integrations     │
├─────────────────────────┤
│ id                      │
│ tenant_id               │
│ integration_type        │  ← jira | google_workspace | slack | confluence
│ instance_url            │  ← "akamai.atlassian.net"
│ config_json             │  ← Non-sensitive config
│ credentials_key         │  ← Reference to secrets store
│ enabled                 │
│ last_sync_at            │
│ sync_status             │  ← healthy | error | never_synced
│ created_at              │
└───────────┬─────────────┘
            │
┌───────────┴─────────────┐
│ tenant_jira_mappings    │
├─────────────────────────┤
│ integration_id          │
│ jira_project_key        │  ← "OUT"
│ penfold_project_id      │  ← Links to projects table
│ sync_tickets            │  ← true/false - fetch ticket details?
│ created_at              │
└─────────────────────────┘

┌─────────────────────────┐
│tenant_processing_rules  │
├─────────────────────────┤
│ tenant_id               │
│ rule_name               │  ← "skip-internal-alerts"
│ priority                │  ← Lower = higher priority
│ match_conditions        │  ← JSON: {from_contains, subject_contains, etc.}
│ classification_override │  ← JSON: {subtype, profile}
│ enabled                 │
│ created_at              │
└─────────────────────────┘
```

---

## CLI Commands

### Tenant Management

```bash
# Tenant management
penf tenant list
penf tenant create --name "work-akamai" --slug "akamai"
penf tenant switch akamai
```

### Domain Configuration

```bash
# Domain configuration
penf config domains list
penf config domains add akamai.com --type internal
penf config domains add gmail.com --type external_known --notes "Personal accounts"
```

### Email Patterns (bots, distribution lists)

```bash
# Email patterns
penf config patterns list
penf config patterns add "*-jira@*" --type bot
penf config patterns add "team-*@*" --type distribution_list
penf config patterns add "Prb-Facilitator@*" --type role_account
```

### Integrations

```bash
# Integrations
penf integrations list
penf integrations add jira \
  --url "akamai.atlassian.net" \
  --auth  # Prompts for API token, stores securely

penf integrations jira map OUT --project tiktok-fy26
penf integrations jira map MTC --project mtc-2026
```

### Processing Rules

```bash
# Processing rules (JSON conditions)
penf config rules list
penf config rules add "skip-internal-alerts" \
  --match '{"from_contains": "alerts@akamai.com"}' \
  --classify '{"subtype": "notification/internal", "profile": "metadata_only"}' \
  --priority 10
```

---

## Processing Rule Match Conditions

Starting with simple JSON conditions (can extend later):

```json
{
  "from_contains": "string",      // From address contains
  "from_matches": "glob",         // From matches glob pattern
  "to_contains": "string",        // Any To/Cc contains
  "subject_contains": "string",   // Subject contains
  "subject_starts_with": "string",// Subject prefix
  "has_header": "header_name",    // Header exists
  "header_contains": {"name": "X-Custom", "value": "string"}
}
```

Multiple conditions = AND logic. For OR, create multiple rules.

---

## Secrets Storage

Credentials stored in secrets file (not database):

```yaml
# ~/.config/penfold/secrets.yaml (chmod 600)
akamai:
  jira_token: "xxx"
  google_oauth:
    client_id: "..."
    client_secret: "..."
    refresh_token: "..."

personal:
  google_oauth:
    client_id: "..."
    # ...
```

`tenant_integrations.credentials_key` references the tenant slug. Code resolves:
```go
secrets.Get(tenant.Slug, "jira_token")
```

Future: Can swap to external secrets manager (Vault, AWS Secrets Manager) by changing the secrets backend.

---

## Configuration Resolution

```go
type TenantConfig struct {
    TenantID             string
    InternalDomains      []string
    BotPatterns          []string
    DistributionPatterns []string
    RoleAccountPatterns  []string
    ProcessingRules      []ProcessingRule
    Integrations         map[string]Integration
}

// Cached for 5 minutes - config doesn't change often
func (r *ConfigResolver) GetConfig(tenantID string) (*TenantConfig, error)
```

---

## Functional Requirements

- **FR-600**: System MUST support multiple tenants with isolated configuration
- **FR-601**: System MUST store tenant domains with type classification (internal/external)
- **FR-602**: System MUST support glob patterns for bot/distribution list detection
- **FR-603**: System MUST support custom processing rules with JSON match conditions
- **FR-604**: System MUST store integration credentials securely outside database
- **FR-605**: System MUST cache tenant configuration to avoid repeated DB queries
- **FR-606**: System MUST support CLI-based configuration management
