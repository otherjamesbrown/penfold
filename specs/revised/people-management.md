# People Management - Manual First Approach

## Design Philosophy
**Manual control over automated complexity**: Build people database manually for accuracy and simplicity, avoiding HRIS integration complexity.

## People Database Structure

### Core Person Record
```yaml
Person:
  canonical_name: "Robert Smith"
  aliases: ["Bob", "Rob", "Bobby", "Robert"]
  email: "robert.smith@company.com"
  status: enum (current, former, external)
  type: enum (employee, contractor, customer, vendor, other)
  role: "Lead Architect"
  team: "Engineering"
  start_date: "2023-01-15"
  end_date: null (for current employees)
  notes: "Joined from TechCorp, AI/ML expertise"
  created_at: datetime
  updated_at: datetime
```

## Manual People Management Workflow

### Adding New People
```bash
# Interactive person creation
penfold person create
> Canonical name: Robert Smith
> Aliases (comma-separated): Bob, Rob, Bobby
> Email: robert.smith@company.com
> Status: [current/former/external]: current
> Type: [employee/contractor/customer/vendor]: employee
> Role: Lead Architect
> Team: Engineering
> Start date (YYYY-MM-DD): 2023-01-15
> Notes: Joined from TechCorp, AI/ML expertise

Person created: Robert Smith (ID: uuid-12345)
```

### Bulk Import Option
```bash
# CSV import for initial setup
penfold person import --file people.csv

# CSV format:
# canonical_name,aliases,email,status,type,role,team,start_date,notes
# "Robert Smith","Bob,Rob,Bobby",robert.smith@company.com,current,employee,"Lead Architect",Engineering,2023-01-15,"AI/ML expert"
```

### Managing Aliases and Resolution
```bash
# When AI encounters unknown name during processing
penfold person resolve
> Found unresolved name: "Bobby" in email from 2025-01-10
>
> Possible matches:
> [1] Robert Smith (Bob, Rob, Bobby) - Engineering
> [2] Robert Johnson (Bob, Bobby) - Sales
> [3] Create new person
>
> Select option: 1
>
> Added "Bobby" as alias for Robert Smith ✓
```

## People Categories and Handling

### Internal People (Employees/Contractors)
**Current Employees**:
- Full information (role, team, start date)
- Active email addresses
- Regular alias resolution as new nicknames appear

**Former Employees** (Keep for Historical Context):
- Mark status as "former" with end_date
- Preserve all historical information and aliases
- Include departure notes if relevant
- Continue to resolve historical references

**Example Former Employee**:
```yaml
Person:
  canonical_name: "Sarah Johnson"
  status: former
  role: "Former VP Engineering"
  team: "Engineering"
  end_date: "2024-06-30"
  notes: "Left for startup, led Atlas project genesis"
```

### External People
**Customers**:
- Company affiliation in role field
- Contact information if available
- Relationship notes

**Vendors/Partners**:
- Company and role information
- Business relationship context

**Example External Person**:
```yaml
Person:
  canonical_name: "Mike Chen"
  status: external
  type: customer
  role: "CTO at AcmeCorp"
  team: "AcmeCorp"
  email: "mike.chen@acmecorp.com"
  notes: "Primary technical contact for Atlas deployment"
```

## AI-Assisted Name Resolution

### Smart Suggestions
When processing content, AI suggests person matches:
```bash
# During email/meeting processing
> Email mentions "Mike from Acme"
> AI suggests: Mike Chen (CTO at AcmeCorp) - 85% confidence
> [Accept] [Reject] [Create new person] [Add as alias]
```

### Context-Based Resolution
AI uses multiple signals:
- Email domains matching known people
- Role/team context in conversation
- Time proximity to known interactions
- Conversation participants

### Learning from Corrections
```bash
# When correcting AI suggestions
> AI suggested: Mike Chen for "Mike from Acme"
> You selected: Create new person "Mike Davis"
>
> System learns:
> - "Mike from Acme" ≠ Mike Chen
> - New pattern: Multiple Mikes at AcmeCorp
> - Improve future suggestions for "Mike" + "Acme" context
```

## Maintenance Workflows

### Regular Review and Updates
```bash
# Weekly people management tasks
penfold person review --unresolved
> 5 unresolved names found this week
> Review and resolve? [y/n]

penfold person update --recent
> Recent changes to review:
> - Sarah Johnson marked as former employee
> - 3 new aliases added
> - 2 external contacts created
```

### Duplicate Detection
```bash
# Find potential duplicates
penfold person duplicates
> Potential duplicates found:
> - "Rob Smith" and "Robert Smith" (similar names, different emails)
> - "Mike" (customer) and "Mike Chen" (same email domain)
>
> Review and merge? [y/n]
```

## Integration with Content Processing

### Email Processing
- Extract "From" and "To" addresses
- Map to known people or flag for resolution
- Learn email signature patterns for name/role extraction

### Meeting Processing
- Parse attendee lists from meeting metadata
- Cross-reference with known people
- Extract speaker identification from transcripts

### Timeline and Search Integration
```bash
# People-centric queries
penfold timeline --person "Robert Smith" --last-month
penfold search "what did Sarah say about Atlas" --include-former
penfold people --team Engineering --status current
```

## Data Migration and Backup

### Export Options
```bash
# Backup people database
penfold person export --format csv --file people-backup.csv
penfold person export --format json --file people-backup.json
```

### Import/Sync Options for Future
When ready for automation:
- HRIS integration: Import employee data periodically
- Email signature parsing: Auto-detect role changes
- Calendar integration: Extract external meeting attendees

## Success Metrics

### People Database Health
- **Coverage**: Percentage of email/meeting participants resolved
- **Accuracy**: Correction rate for AI name suggestions
- **Completeness**: Average data fields populated per person

### User Experience
- **Resolution time**: Average time to resolve unknown names
- **Search effectiveness**: Success rate finding people in queries
- **Maintenance burden**: Time spent on weekly people management

This manual approach gives you complete control while building the foundation for future automation.