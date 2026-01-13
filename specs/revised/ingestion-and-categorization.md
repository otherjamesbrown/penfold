# Ingestion and Categorization Design

**Database Schema**: See [001-database-schema](../001-database-schema/spec.md) for storage layer supporting pub-sub processing framework.

## Three-Channel Ingestion System

### 1. Manual Tagging (High Control)
**Use Cases**: Important meetings, customer calls, critical documents
- User explicitly assigns project(s) + adds context notes
- Confidence: 100%
- Multiple project tagging supported

**Examples**:
```bash
penfold ingest manual --source "zoom_transcript.txt" --projects "Atlas,SOC2" --notes "Customer feedback on deployment security"
penfold ingest manual --source "https://docs.google.com/doc/xyz" --projects "People Management" --notes "Performance review template"
```

### 2. AI Suggested with Human Review (Learning Loop)
**Use Cases**: Email, general Slack messages, documents
- AI analyzes content and suggests project categorization
- Daily review queue for user confirmation/correction
- System learns from feedback to improve over time
- Full AI semantic understanding (not just keywords)

**Review Workflow**:
```bash
penfold review daily
> 15 items need categorization review
> Email from bob@company.com: "deployment timeline concerns"
> AI suggests: [Atlas: 85%] [SOC2: 30%]
> Reasoning: Technical deployment discussion with Atlas team member
> Actions: [Accept] [Add SOC2] [Different Projects] [Add Learning Rule]
```

### 3. Pre-Tagged (Automation)
**Use Cases**: Project-specific channels, labeled folders
- One-time configuration maps sources to projects
- 100% automatic assignment
- Can still be multi-project

**Examples**:
```bash
penfold config set-channel "#atlas-project" --projects "Atlas"
penfold config set-folder "Gmail:SOC2 Compliance" --projects "SOC2,Operations"
```

## Key Design Decisions

### Multi-Project Support
- Any information entity can belong to multiple projects
- Atlas deployment email can be tagged: Atlas + Operations + SOC2
- No forced single categorization

### Progressive Automation
- **Phase 1**: Review 100% of AI suggestions (learning mode)
- **Phase 2**: Auto-accept high-confidence suggestions (>95%), review medium (70-95%)
- **Phase 3**: Only review low-confidence or new patterns
- User controls automation level per project

### AI Learning Framework
- Full semantic understanding of content, participants, context
- Learns from user corrections and additions
- Builds sophisticated rules beyond simple keywords
- Understands context: "HR deployment" vs "technical deployment"

## Learning Rules Evolution

### Initial Rules (Simple)
```yaml
Atlas Project:
  keywords: [deployment, kubernetes, infrastructure]
  participants: [bob@company.com, alice@company.com]
  confidence_threshold: 70%
```

### Advanced Rules (After Learning)
```yaml
Atlas Project:
  semantic_patterns:
    - technical_deployment AND (infrastructure OR kubernetes)
    - participant_overlap_with_atlas_team > 50%
    - NOT (hr_context OR hiring_context)
  negative_indicators:
    - "HR deployment" (learned from user correction)
    - hiring_process AND deployment (learned pattern)
  multi_project_indicators:
    - security + deployment = [Atlas, SOC2]
  confidence_threshold: 85% (adjusted based on accuracy)
```

## Daily Review Workflow

### Morning Triage (9am)
1. **New categorizations needed**: AI suggestions from overnight ingestion
2. **Confidence levels**: High/Medium/Low confidence items
3. **Rule learning**: When user corrects, system asks why
4. **Multi-project decisions**: AI suggests additional project tags

### Review Interface
```bash
penfold review daily --queue
> 12 items need review (8 high confidence, 3 medium, 1 low)
>
> [Item 1/12] HIGH CONFIDENCE (92%)
> Email: "Atlas deployment security checklist"
> From: bob@company.com To: alice@company.com, security@company.com
> AI suggests: [Atlas: 92%] [SOC2: 78%]
>
> Actions:
> [a] Accept both    [1] Atlas only    [2] SOC2 only    [d] Different projects
> [r] Add rule       [s] Skip          [n] Next
```

## Technical Implementation Notes

### Event-Driven Processing Pipeline
1. **Content Ingestion Event**: Published when new content arrives
2. **Parallel AI Processing**: Multiple subscribers process simultaneously
   - Content Analysis: Full semantic understanding of text
   - Participant Mapping: Cross-reference with org chart
   - Context Understanding: Meeting vs email vs document type
   - Project Scoring: Semantic similarity to existing project content
   - Multi-Project Detection: Identify overlapping themes
3. **Result Aggregation**: Combine outputs from multiple processors
4. **Quality Validation**: Optional cloud model validation for low-confidence results

### Learning Storage
**Note**: Full database schema in [001-database-schema](../001-database-schema/spec.md) including event-driven processing tables.

```sql
learning_rules:
  project_id: UUID
  rule_type: enum (keyword, semantic, participant, negative, multi_project)
  rule_definition: jsonb
  confidence_adjustment: float
  created_from_correction: boolean
  effectiveness_score: float

processing_events:
  event_id: UUID
  event_type: text
  payload: jsonb
  created_at: timestamp

processing_results:
  result_id: UUID
  event_id: UUID (references processing_events)
  processor_id: text
  result_data: jsonb
  confidence_score: float
  processing_time_ms: integer
```

### Feedback Loop
- Track accuracy of AI suggestions over time
- Adjust confidence thresholds based on performance
- Surface rules that aren't working well
- Continuous improvement of semantic understanding

## Open Questions for Discussion
1. How should conflicting rules be resolved?
2. What happens when projects end - archive rules or keep for historical context?
3. Should AI suggest new projects when it sees unknown patterns?