# Architecture Comparison: v1.0 vs v2.0

## Key Changes from Original Spec

### Mental Model Shift

**v1.0: Knowledge Management System**
- Complex assertion taxonomy (8 types)
- Rigid project hierarchies
- Formal entity relationships
- Decision support focus

**v2.0: Contextual Time Machine**
- Temporal spine + project contexts
- Emergent relationships
- Retroactive context reconstruction
- Search and discovery focus

### Data Model Simplification

**v1.0 Entity Model** (Over-engineered)
```
Source → Assertion (8 types) → Person (team/role) → Project (hierarchical)
+ Complex assertion linking
+ Formal team structures
+ Predetermined relationship types
```

**v2.0 Entity Model** (Emergent)
```
Information Entity (timestamped) → Project Contexts (flat)
+ People (AI discovered aliases/attributes)
+ Relationships (AI discovered, not predetermined)
+ Topics/Artifacts (emergent clustering)
```

### Use Case Evolution

**v1.0 Primary Use Cases:**
- Daily morning review workflow
- Weekly project health summaries
- Decision archaeology
- Entity graph management

**v2.0 Primary Use Cases:**
- "Find Bob's email about deployment"
- "Rewind 2 weeks, show Atlas timeline"
- "Who said they'd do X by when?"
- "What led to this decision?"

### MVP Scope Reduction

**v1.0 MVP:**
- Gmail integration
- Basic extraction pipeline
- Minimal entity graph
- Single project space
- 5 CLI commands
- Still quite complex!

**v2.0 MVP:**
- Email + meetings only
- Entity discovery (not predefined types)
- Project context assignment
- 3 core query patterns
- Focus on search speed over extraction accuracy

## Technical Architecture Changes

### Processing Pipeline

**v1.0: Tiered LLM Processing**
```
Local LLM (classification) → Cloud LLM (extraction) → Structured storage
Focus: Extract 8 assertion types with high accuracy
```

**v2.0: Discovery-Oriented Processing**
```
Ingestion → Entity Discovery → Relationship Discovery → Query Engine
Focus: Find entities and connections, optimize for retrieval speed
```

### Storage Strategy

**v1.0:** Highly normalized relational model with pgvector
**v2.0:** Simpler temporal model with emergent graph relationships

### Query Interface

**v1.0:** Complex CLI with many subcommands and formal review workflows
**v2.0:** Simple search patterns optimized for "time travel" queries

## Why These Changes Matter

1. **Achievability**: v2.0 MVP is actually buildable in reasonable time
2. **User Fit**: Matches actual COO workflow (retroactive context vs proactive insights)
3. **Technical Alignment**: Plays to LLM strengths (discovery) vs weaknesses (rigid classification)
4. **Scalability**: Emergent model adapts to new patterns vs rigid schema
5. **ADHD-Friendly**: Structured temporal browsing vs complex workflow management

## Migration Path

The v1.0 spec provided valuable thinking, but v2.0 represents a fundamental reframe based on:
- User research (our conversation)
- Technical reality (what's actually achievable)
- Product focus (archaeology vs knowledge management)

Phase 2 of v2.0 can incorporate many v1.0 ideas (weekly summaries, risk detection, etc.) but built on the solid foundation of temporal search and emergent relationships.