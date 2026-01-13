# Agent Lifecycle Management

**Purpose**: How we create, maintain, and evolve development agents throughout the Penfold project lifecycle.

---

## Agent Creation Strategy

### **Spec → Agent Transition**

Each major specification goes through this lifecycle:

```
1. Specification Phase (SpecKit)
   ├─ Research and design
   ├─ Requirements definition
   ├─ Implementation planning
   └─ Task breakdown

2. Implementation Phase
   ├─ Following spec tasks
   ├─ Real-world learning
   ├─ Architecture decisions
   └─ Pattern identification

3. Agent Creation Phase ← **Critical Transition**
   ├─ Distill specification knowledge
   ├─ Extract implementation patterns
   ├─ Create agent context
   └─ Transfer ownership from spec to agent

4. Agent Management Phase
   ├─ Agent maintains domain
   ├─ Context stays current
   ├─ Patterns evolve with code
   └─ Handoff coordination
```

---

## When to Create Agents

### **Immediate Agent Creation** (Upon Spec Completion)
- **database-dev** - Foundation for all other features
- **ai-dev** - Core AI processing capabilities
- **testing-dev** - Essential for quality assurance

### **Progressive Agent Creation** (As Specs Complete)
- **integration-dev** - After Gmail/meeting pipeline specs (004-005)
- **search-dev** - After search interface spec (007)
- **automation-dev** - After automation engine spec (008)

### **Agent Creation Criteria**
✅ Create agent when:
- Specification implementation is >80% complete
- Domain patterns are well-established
- Handoff boundaries are clear
- Future work is expected in the domain

❌ Don't create agent when:
- Specification is exploratory/research phase
- Implementation patterns are still changing rapidly
- Domain boundaries are unclear
- Feature is likely to be deprecated/replaced

---

## Knowledge Distillation Process

### **From Specification to Agent Context**

#### 1. Extract Core Principles
**Source**: `specs/XXX/spec.md` → **Target**: `context/XXX-dev/agents.md`

```markdown
# Example: Database Spec → Database Agent
## From specs/001-database-schema/spec.md:
- FR-001: Multi-tenant architecture with RLS policies
- SC-005: CRUD operations <100ms for 10K records
- Multi-model processing with event coordination

## To context/database-dev/agents.md:
- **Rule**: All entities MUST have tenant_id
- **Pattern**: SQLAlchemy async with RLS policies
- **Contract**: <100ms CRUD performance target
```

#### 2. Capture Implementation Patterns
**Source**: Actual code + task learnings → **Target**: Agent patterns

```python
# From implementation experience
# Real SQLAlchemy patterns that work
# Performance optimizations discovered
# Error handling that proved necessary
```

#### 3. Document Decision Context
**Source**: Implementation decisions → **Target**: Agent troubleshooting

```markdown
# Why PostgreSQL + pgvector vs alternatives
# Why HNSW indexing parameters (M=16, ef_construction=200)
# Why RLS over application-level multi-tenancy
# Common failure modes and solutions
```

#### 4. Define Handoff Boundaries
**Source**: Cross-spec dependencies → **Target**: Agent domain boundaries

```markdown
# Database agent handles storage optimization
# AI agent handles model selection and processing
# Integration agent handles external system coordination
```

---

## Agent Context Structure

### **Standard Context Layout**
```
context/
├─ agents.md                    # Shared rules for all agents
├─ beads.md                     # Beads workflow and commands
├─ database-dev/
│  ├─ agents.md                 # Database domain rules and patterns
│  ├─ architecture.md           # Storage design decisions
│  ├─ patterns.md               # SQLAlchemy, migration, testing patterns
│  ├─ performance.md            # Targets, monitoring, optimization
│  └─ troubleshooting.md        # Common issues and solutions
├─ ai-dev/
│  ├─ agents.md                 # AI domain rules and patterns
│  ├─ models.md                 # Model selection and integration
│  ├─ events.md                 # Pub-sub processing patterns
│  └─ cost-management.md        # Budget and performance control
└─ [other-agent-contexts]/
```

### **Context File Purposes**

| File | Purpose | Source |
|------|---------|--------|
| `agents.md` | Domain rules, boundaries, handoffs | Spec requirements + implementation learnings |
| `architecture.md` | Design decisions and rationale | Spec design + architecture decisions |
| `patterns.md` | Code patterns and best practices | Implementation experience |
| `performance.md` | Targets, monitoring, optimization | Success criteria + real performance data |
| `troubleshooting.md` | Common issues and solutions | Bug fixes and operational experience |

---

## Agent Evolution Process

### **Keeping Agent Context Current**

#### During Implementation
1. **Agent updates own context** when making architectural changes
2. **Document pattern discoveries** in agent context files
3. **Update performance data** with actual measurements
4. **Add troubleshooting entries** from resolved issues

#### Quarterly Reviews
1. **Context audit** - Are agent rules still accurate?
2. **Boundary review** - Do handoff patterns still work?
3. **Performance validation** - Are targets still realistic?
4. **Pattern updates** - New best practices from implementation?

#### Specification Retirement
1. **Mark specs as archived** when agent context is complete
2. **Add reference links** from archived specs to agent context
3. **Preserve research artifacts** but point to agent for current info

---

## Bead Management for Agent Creation

### **Agent Creation Beads** (To Be Created)

For each specification with >80% implementation:

```bash
# Database Agent (Ready Now)
bd create --title="Create database-dev agent from specs/001" --type=feature --priority=2
bd create --title="Distill database patterns into agent context" --type=task --priority=2
bd create --title="Document database performance contracts" --type=task --priority=2

# AI Agent (After AI Coordination Spec)
bd create --title="Create ai-dev agent from specs/002-003" --type=feature --priority=3
bd create --title="Extract AI processing patterns into agent context" --type=task --priority=3
bd create --title="Document model selection and cost management" --type=task --priority=3

# Integration Agent (After Gmail/Meeting Specs)
bd create --title="Create integration-dev agent from specs/004-005" --type=feature --priority=3
bd create --title="Capture external system integration patterns" --type=task --priority=3

# Search Agent (After Search Spec)
bd create --title="Create search-dev agent from spec/007" --type=feature --priority=3
bd create --title="Document search and retrieval patterns" --type=task --priority=3

# Testing Agent (After Testing Framework)
bd create --title="Create testing-dev agent from specs/010" --type=feature --priority=2
bd create --title="Document AI mocking and test patterns" --type=task --priority=2
```

### **Agent Maintenance Beads**

```bash
# Quarterly maintenance
bd create --title="Q1 Agent context audit and update" --type=maintenance --priority=3
bd create --title="Review agent handoff boundaries" --type=review --priority=3
bd create --title="Update performance contracts with real data" --type=task --priority=3
```

---

## Success Criteria

### **Agent Creation Complete When:**
- [ ] Agent context documents created in `context/XXX-dev/`
- [ ] Domain boundaries clearly defined with handoff rules
- [ ] Implementation patterns documented from real code
- [ ] Performance contracts established with measurable targets
- [ ] Troubleshooting guide includes common issues and solutions
- [ ] Handoff beads created for cross-agent dependencies

### **Agent Health Metrics**
- **Context Freshness**: Last updated <30 days ago
- **Pattern Accuracy**: Agent patterns match actual codebase
- **Boundary Clarity**: <5% of beads require cross-agent clarification
- **Performance Tracking**: Agent meets documented performance contracts

---

## Migration from Specs to Agents

### **Specification Retirement Process**

1. **Agent context complete** - All patterns documented
2. **Cross-reference update** - Spec points to agent context
3. **Archive specification** - Mark as "implemented, see agent"
4. **Preserve research** - Keep design rationale accessible
5. **Update documentation map** - ARCHITECTURE.md points to agents

### **Example Retirement**
```markdown
# specs/001-database-schema/README.md
## Status: IMPLEMENTED ✅
**Implementation Complete**: 2026-01-20
**Agent**: See `context/database-dev/` for current patterns and rules
**Purpose**: This spec served its purpose during implementation. All current database knowledge is maintained by the database-dev agent.

**Historical Value**: Preserved for architecture decisions and requirements traceability.
```

---

## Long-term Vision

### **Agent-Driven Development**
- **Specifications become research artifacts** - Valuable for decisions but not operational
- **Agents become living documentation** - Always current with implementation
- **Cross-agent coordination** - Clear handoff patterns prevent scope creep
- **Knowledge preservation** - Agent context captures "why" behind decisions

### **Agent Network Evolution**
```
Year 1: Core agents (database, ai, integration, search)
Year 2: Specialized agents (analytics, security, performance)
Year 3: Product agents (customer-facing features, integrations)
```

Each agent maintains institutional memory while enabling confident evolution of their domain.

---

## References

- **Agent Rules**: `context/agents.md`
- **Current Agents**: `context/*/agents.md`
- **Specification Status**: `specs/*/README.md`
- **Architecture Map**: `ARCHITECTURE.md`