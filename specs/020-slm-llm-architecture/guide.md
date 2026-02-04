# Splitting the Work: SLMs and LLMs in Penfold

A practical guide to deciding what runs locally on a 7B model and what gets sent to a remote LLM, why, and how the pieces fit together.

---

## The Problem We're Solving

Penfold ingests content from multiple sources: emails, meeting transcripts, Slack threads, documents. It needs to extract entities, understand sentiment, identify action items, map content to known projects and risks, and make all of this searchable.

Right now, the entire analysis happens in a single LLM call (see `services/gateway/modelservice/service.go:752`). The content gets truncated to 8,000 characters, fed to a local 7B model with one comprehensive prompt, and the model is asked to do everything at once: sentiment, entities, topics, action items, insights.

This has three problems:

1. **The 8,000 character limit throws away content.** A long email thread or meeting transcript can easily be 30,000+ characters. We're losing information before we even start.

2. **A 7B model struggles with complex reasoning.** Mapping an email discussion to Risk #4 from last Tuesday's MTC meeting requires connecting information across contexts. That's not what small models are good at.

3. **Everything gets the same treatment.** A "free for lunch?" email gets the same full analysis pipeline as a critical project risk escalation. That's wasteful even when using a free local model, because it's slow.

---

## The Foundation: Human + AI Collaboration

Before we get into model architectures and pipeline stages, we need to establish what this system is actually for. **Penfold is not a fully automated system. It is not a manual one either. It is a collaboration between a human and an AI assistant.**

### The Radar Model

Think of a radar screen. The AI paints the full picture — every risk, every mention, every escalation, tracked consistently. The human points the spotlight — "these 3 risks are what I'm watching right now." The periphery is still monitored — when something starts moving (more mentions, higher severity language, senior people getting involved), the AI notices and surfaces it.

The spotlight moves. Things that were background become urgent. When something enters the spotlight, the human needs instant context: When was this first raised? By whom? Give me a summary. What's the progress? Who's been escalated to? Who are the key people?

This means the AI must track **everything** — not just what seems important today. Completeness first, curation second. The human decides what matters; the AI makes sure nothing is lost.

### The Human Adds Value the AI Cannot See

The human is not a passive observer. They add context that no amount of AI processing can discover:

- **Trust signals**: "When Sarah says it's an issue, I believe her." Trust is personal and subjective — a staff engineer you've worked with for years may carry more weight than an unfamiliar VP. The system captures this.
- **Offline context**: "I talked to Mike yesterday, he says it's handled." Conversations that happen outside email and meetings are invisible to the AI but critical to understanding the real state of things.
- **Gut feel**: "I have a bad feeling about this one." Sometimes you don't have data, but you have experience. The system should capture and weight that.
- **Priority decisions**: "Elevate this to critical." The human decides what enters the spotlight.

These are not overrides of the AI's judgment. They are **inputs** the AI doesn't have access to any other way. Human signals are first-class data.

### Seniority and Trust: Two Different Axes

**Seniority** is organizational hierarchy. When a VP flags something, it carries organizational weight. A VP attending a meeting they weren't previously involved in is itself a strong signal. The seniority profile of people discussing a topic tells you about escalation patterns.

**Trust** is personal. There are specific people whose judgment you rely on. This might not correlate with seniority at all. Trust can be domain-specific: "I trust Sarah on technical risks but not on timeline estimates."

Both factor into how the system weights assertions:
- An assertion from a trusted or senior person gets higher initial visibility
- A change in the seniority profile of people involved triggers an alert
- The human can always override by watching anything regardless of who raised it

### Bidirectional Prompting

The human asks Claude questions. But Claude also prompts the human:

- "3 new risks surfaced this week — what are your thoughts?"
- "VxLAN hasn't been mentioned in 2 weeks. Is it resolved, or has everyone just forgotten?"
- "You've been watching this risk for 3 weeks with no change. Should we close it or escalate?"
- "A VP just entered the VxLAN discussion for the first time. Want to add this to your watch list?"

The AI should know when to ask for input — not for every minor update, but when something has changed character.

### What This Means for the Pipeline

Every design decision in this guide follows from this collaboration model:

1. **Track everything** — every assertion extracted, every mention logged. No filtering. The "whole."
2. **Weight by who said it** — seniority and trust factor into initial assertion visibility.
3. **Human curates through conversation** — watch lists, annotations, priority overrides via Claude Code.
4. **Proactive change detection** — the AI monitors the periphery and alerts when patterns change.
5. **Briefings on demand** — when the spotlight moves, full context assembles instantly because we've been tracking everything.

The SLM/LLM split serves this model: SLMs handle the comprehensive tracking (cheap, fast, every piece of content). LLMs handle the deep analysis and synthesis (expensive, slow, only when warranted — including when the human asks for it).

### How Claude Gets Its Memory

There's a fundamental asymmetry: the human wakes up with their memory intact. Claude wakes up with amnesia and a CLAUDE.md file. Every session starts from zero context.

The solution is a clean separation between **personality** and **memory**:

**CLAUDE.md = Personality**
How to think. How to behave. What the system is. What commands exist. When to prompt the human. How to interpret trust and seniority signals. This is static — it changes when the system changes, not between sessions.

```markdown
# CLAUDE.md (simplified)
You are working on Penfold. On session start, run `penf context morning` to load your working memory.
When the human mentions risks, people, or projects, query penf for current state.
When you detect a change worth surfacing, prompt the human.
Trust and seniority scores are on a 1-5 and 1-7 scale respectively.
At session end, persist anything the human said that isn't already captured.
```

**penf context morning = Memory**
What to think about. What's happening right now. What the human cares about. This is dynamic — it changes every day, every session.

```
$ penf context morning --format json
```

Returns a structured briefing — not everything, but enough to start working:

```json
{
  "session": {
    "last_session": "2025-12-16T18:30:00Z",
    "last_session_summary": "Focused on VxLAN risk. User expressed concern about timeline. Added note: Mike says CDN capacity handled. Dropped CDN from watch list."
  },
  "watch_list": [
    {
      "assertion_id": 101,
      "root_id": 101,
      "type": "risk",
      "description": "VxLAN injection vulnerability in CLIC PLT",
      "severity": "critical",
      "status": "open",
      "owner": "Dan Spataro",
      "your_notes": "Dan thinks this is worse than reported",
      "last_change": "2025-12-16 - VP Sarah Chen requested mitigation plan by EOM",
      "changes_since_last_session": 1
    },
    {
      "assertion_id": 205,
      "root_id": 205,
      "type": "risk",
      "description": "CLIC staffing gap",
      "severity": "high",
      "status": "open",
      "owner": "Dan Spataro",
      "your_notes": null,
      "last_change": "2025-12-15 - escalated from medium to high",
      "changes_since_last_session": 0
    }
  ],
  "recent_changes": {
    "since": "2025-12-16T18:30:00Z",
    "new_risks": 1,
    "escalations": 0,
    "decisions": 2,
    "new_content_processed": 14,
    "items_needing_attention": [
      {
        "type": "new_risk",
        "description": "Missing SLOs for API gateway",
        "raised_by": "Melissa General (Director)",
        "seniority": 5,
        "source": "MTC Status Update email"
      }
    ]
  },
  "active_projects": [
    {"name": "MTC", "open_risks": 4, "open_actions": 7},
    {"name": "CLIC", "open_risks": 2, "open_actions": 3}
  ],
  "trusted_people": [
    {"name": "Dan Spataro", "trust": 5, "domains": ["technical", "risk"]},
    {"name": "Sarah Chen", "trust": 4, "domains": ["strategy"]}
  ]
}
```

Claude reads this and now has a working memory: what you're tracking, what changed, who matters, what needs attention. It's not everything — but it's enough to start the conversation intelligently.

**Going deeper on demand.** If you say "tell me about the VxLAN risk", Claude doesn't rely on the bootstrap summary. It queries:

```
penf assertion history --root-id 101 --format json
penf assertion briefing --root-id 101 --format json
```

That returns the full golden thread: origin, every lifecycle event, key people, escalation chain, linked content, current state. The bootstrap gave Claude enough to know this matters; the query gives it full depth.

**Session end: persisting what Claude learned.** During the session, most human context is captured through penf commands (watch list changes, annotations, priority overrides). But at session end, Claude should persist a session summary:

```
penf context session-end --summary "User focused on VxLAN. Concerned about January deadline. Wants to review CLIC staffing next session. Mentioned offline that budget approval for contractor is pending."
```

Next morning, this loads as part of `penf context morning`. The memory persists across the context gap.

**The key insight:** Claude's memory is not in Claude's context window. It's in the database, accessible through penf. The context window is a working scratchpad for the current conversation. The database is the long-term memory. Claude reconstructs what it needs at session start and queries for depth as the conversation requires it.

---

## Understanding What SLMs Can and Cannot Do

Before we design anything, we need to be honest about what a 7B model running on Apple Silicon actually does well.

### What a 7B model does reliably

**Classification into a small set of known categories.** If you give it an email and ask "is this one of: project-update, customer-communication, risk-escalation, internal-comms, personal, action-request?" it will get this right the vast majority of the time. The answer is almost always obvious from the first few lines, and the model only needs to pattern-match against a handful of categories.

**Extraction of explicitly stated information.** If the email says "Meeting with Dan Spataro on Friday at 3pm about the CLIC staffing issue", a 7B model can extract: person=Dan Spataro, date=Friday 3pm, topic=CLIC staffing. The information is right there in the text. The model is doing structured extraction, not reasoning.

**Short summarisation.** Compressing a single email or a single page of text into 2-3 sentences. This is essentially "what are the most important sentences?" which small models handle well.

**Structured output from clear instructions.** If you tell it "output JSON with these exact fields" and the task is simple, it follows the format. This works because the task is mechanical.

### What a 7B model does unreliably

**Connecting information across contexts.** "Does this email relate to the noisy neighbour risk we discussed in the MTC meeting?" requires the model to hold two separate contexts (the email and the meeting notes) and reason about their relationship. A 7B model will either hallucinate a connection or miss an obvious one.

**Nuanced business sentiment.** "We're making good progress but there are some areas we need to keep an eye on" is diplomatically negative. A 7B model will likely score this as neutral or slightly positive. Understanding corporate euphemism requires exposure to a lot of business language and the reasoning to decode it.

**Following complex multi-step instructions.** The current FULL analysis prompt asks for sentiment analysis, entity extraction, topic identification, action items, and strategic insights all in one call. That's five different tasks packed into one prompt. A 7B model will do some of them adequately and some of them poorly, and you won't know which is which.

**Handling long inputs.** Even if the context window technically fits the text, model quality degrades with input length. A 7B model paying attention to 6,000 tokens is not doing as good a job as it does at 500 tokens.

### The rule of thumb

> If the answer is already in the text and you just need to pull it out or label it, use the SLM.
> If the answer requires reasoning, connecting dots, or understanding subtext, use the LLM.

---

## The Pipeline Architecture

Instead of one call that does everything, we split the work into stages. Each stage is matched to the right model for the job.

```
Content arrives (email / transcript / slack)
       |
       v
   Stage 0: Parse
   (no AI - libraries only)
   Strip HTML, extract headers, detect format
       |
       v
   Stage 1: Triage
   (SLM - every piece of content)
   Classify, rate importance, detect relevance
       |
       |--- LOW importance, PERSONAL/INTERNAL_COMMS ---> Store and stop
       |
       v
   Stage 2: Extract
   (SLM - content that passes triage)
   Pull out entities, dates, action items, project names
       |
       v
   Stage 3: Enrich with Context
   (code logic - database lookups)
   Match extracted entities to known people, glossary, products
       |
       v
   Stage 4: Deep Analysis
   (Remote LLM - only for content that warrants it)
   Sentiment, strategic insights, risk mapping, synthesis
       |
       v
   Stage 5: Embed and Index
   (SLM for embeddings, database for storage)
   Generate vector embeddings, update search index
```

Let's walk through each stage in detail.

---

## Stage 0: Parse (No AI)

**What it does:** Converts raw content into clean text. Extracts metadata that's already structured.

**Why no AI:** This is a solved engineering problem. Using an LLM to strip HTML tags is like using a microscope to read a road sign.

### For emails

- Strip HTML to plain text using a library like `jaytaylor/html2text` in Go, or the `html` standard library. Do not use a model for this.
- Extract headers: From, To, CC, Subject, Date, Message-ID, In-Reply-To, References.
- Detect thread structure from the References header chain.
- Extract quoted reply sections (lines starting with `>` or `On ... wrote:` blocks). Separate the new content from the quoted content. This matters - we don't want to re-analyse quoted text we've already seen.
- Detect attachments by MIME type. Flag them for separate processing if needed.

### For meeting transcripts

- Detect format: WebVTT (.vtt), plain text, SRT. Penfold already handles this in `cmd/penf/cmd/ingest_meeting.go`.
- Parse speaker labels and timestamps from VTT/SRT format.
- Remove timing markers and formatting artefacts that don't carry meaning.
- Detect platform-specific patterns (Webex auto-transcription artefacts, Teams speaker identification format, etc.).

### For Slack messages

- Parse JSON export format (Slack exports as JSON with message arrays).
- Extract: sender, timestamp, channel, thread_ts (identifies threads), reactions, mentions.
- Expand user IDs to names if a user mapping is available.
- Handle message types: regular messages, thread replies, bot messages, file shares, channel joins.
- Reconstruct thread structure using thread_ts grouping.

### What you get out of Stage 0

A clean text body, structured metadata, and thread/conversation structure. All deterministic, all fast, no model required.

---

## Stage 1: Triage (SLM)

**What it does:** Classifies content and decides how much processing it deserves.

**Why the SLM:** This is a simple classification task. The answer is usually obvious from the subject line and first paragraph. A 7B model handles this well because the output is constrained to a few labels and a one-sentence reason.

### The triage prompt

```
You are a content classifier for a business knowledge management system.

Classify this content into exactly ONE category:
- PROJECT_UPDATE: project status, meeting notes, deliverables, milestones
- CUSTOMER: customer communications, sales, deals, account management
- RISK_ISSUE: risks, problems, escalations, blockers, vulnerabilities
- ACTION_REQUEST: someone asking for specific action to be done
- DECISION: a decision has been made or is being requested
- INTERNAL_COMMS: HR, training, company announcements, policy changes
- PERSONAL: lunch, social, casual conversation
- OTHER: does not fit any category above

Rate importance: HIGH, MEDIUM, LOW

Respond ONLY with JSON:
{"category": "...", "importance": "...", "reason": "one sentence"}

---
Subject: {subject}
From: {sender}

{first 500 characters of body}
```

### Key design decisions

**Only send the first ~500 characters.** For triage, you don't need the whole email. The subject line and opening paragraph tell you what the email is about. This keeps the SLM call fast and well within context limits.

**The category list is fixed and small.** Classification works best with 5-10 clear categories. If you need more granularity, add it in Stage 2 where you have more context.

**Include the subject line and sender.** "Mandatory Compliance Training" from hr-noreply@ is immediately classifiable without reading the body. "Re: MTC Risk Register Update" from a project lead is immediately high-importance.

### What happens after triage

| Category | Importance | Next step |
|----------|-----------|-----------|
| PERSONAL | any | Store metadata only. No further processing. |
| INTERNAL_COMMS | LOW | Store with basic metadata. Maybe generate embedding for search. |
| INTERNAL_COMMS | MEDIUM/HIGH | Stage 2 (extract action items - "complete your training by Friday"). |
| PROJECT_UPDATE | any | Stage 2 + Stage 4. |
| CUSTOMER | any | Stage 2 + Stage 4. |
| RISK_ISSUE | any | Stage 2 + Stage 4 (always use Gemini Pro). |
| ACTION_REQUEST | any | Stage 2 + Stage 4. |
| DECISION | any | Stage 2 + Stage 4. |

This means roughly 50-70% of incoming content never goes past the SLM. That's the cost saving.

---

## Stage 2: Extract (SLM)

**What it does:** Pulls out structured data that's explicitly stated in the content.

**Why the SLM:** Extraction is about finding things that are already there, not reasoning about what they mean. "Dan Spataro" is a person name. "January 15th" is a date. "CLIC" is a project name. The model is doing pattern recognition, not inference.

### The extraction prompt

```
Extract the following from this content. Only include information that is explicitly stated - do not infer or guess.

1. People mentioned (name and role/title if stated)
2. Dates and deadlines mentioned
3. Projects, products, or codenames mentioned
4. Organisations or teams mentioned
5. Explicit action items (who should do what, by when)
6. Key decisions stated
7. Risks or issues mentioned

Respond ONLY with JSON:
{
  "people": [{"name": "...", "role": "..."}],
  "dates": [{"date": "...", "context": "..."}],
  "projects": ["..."],
  "organisations": ["..."],
  "action_items": [{"assignee": "...", "action": "...", "due": "..."}],
  "decisions": ["..."],
  "risks": ["..."]
}

If a field has no matches, use an empty array.

---
{content}
```

### Handling content that's too long for the SLM context window

This is where the existing `splitIntoChunks()` function in `services/worker/activities/content_activities.go` becomes relevant. Penfold already has chunking logic with sentence-boundary detection and configurable overlap. We use it here.

**For emails under ~3,000 characters** (the vast majority): Send the full text. One SLM call. Done.

**For emails between 3,000-6,000 characters:** Send the full text but with a more focused extraction prompt. The SLM will handle it, but quality drops slightly at the edges.

**For emails over 6,000 characters (long threads, lengthy updates):**

Split the content into chunks using the existing chunker (1,500 character chunks with 200 character overlap). Run extraction on each chunk independently. Then merge the results:

```
Email (12,000 chars)
    |
    +---> Chunk 1 (chars 0-1500)     ---> SLM extract ---> {people: [...], dates: [...]}
    +---> Chunk 2 (chars 1300-2800)  ---> SLM extract ---> {people: [...], dates: [...]}
    +---> Chunk 3 (chars 2600-4100)  ---> SLM extract ---> {people: [...], dates: [...]}
    ...
    |
    v
  Merge results (deduplicate people, union of all entities)
```

The merge step is **code, not AI.** Deduplicating "Dan Spataro" appearing in three chunks is string matching. The overlap between chunks ensures entities that span a chunk boundary aren't missed.

This is the **map-reduce pattern for extraction.** Each chunk gets mapped independently, then results are reduced by merging. Because extraction is pulling out facts, not reasoning about relationships, the chunks are independent - the model doesn't need to see the whole document to know that "Dan Spataro" is a person name.

### What you get out of Stage 2

A structured JSON document of extracted entities, dates, action items, and risks. All explicitly stated in the source content. Ready for database matching in Stage 3.

---

## Stage 3: Enrich with Context (Code Logic + Database)

**What it does:** Matches the raw extracted data from Stage 2 against Penfold's knowledge base - the people, products, glossary, projects, and teams that are already known.

**Why no AI:** This is database lookups and string matching. Penfold already has the machinery for this.

### How it works

**People resolution.** Take each extracted person name and run it through the existing mention resolution pipeline (documented in `context/client/concepts/mentions.md`). This means:

1. Exact email match against the `people` table
2. Alias lookup (the `person_aliases` table covers email, slack_id, name variants)
3. Name similarity matching with confidence scoring
4. If confidence is 0.7-0.9, flag for review. If < 0.7, don't resolve.

This is the same pipeline that already exists for email header resolution. We're just applying it to names extracted from the body text.

**Glossary expansion.** Take each extracted project name, acronym, or unfamiliar term and look it up in the glossary. This is what `services/gateway/glossaryservice/service.go` already does:

- "TER" resolves to "Technical Execution Review"
- "CLIC" resolves to the CLIC project entity
- "VIP" resolves context-dependently (the glossary supports context tags for disambiguation, as described in `context/client/concepts/glossary.md`)

**Product matching.** Match extracted project/product names against the product hierarchy. The `products` table supports aliases and hierarchical relationships (product -> sub-product -> feature), so "DBaaS" matches even if the full name is "Database as a Service."

**Team matching.** Same for teams - match against known teams and their aliases.

**Unknown entity detection.** If Stage 2 extracted an acronym or project name that doesn't match anything in the glossary or product tables, flag it. This feeds the "unknown acronym workflow" described in `context/client/concepts/glossary.md` - creating review queue items for undefined acronyms that a human can later define.

### What you get out of Stage 3

An enriched version of the Stage 2 output where raw strings have been resolved to entity IDs where possible:

```json
{
  "people": [
    {"name": "Dan Spataro", "person_id": "p-abc123", "confidence": 0.95},
    {"name": "Mike", "person_id": null, "candidates": ["p-def456", "p-ghi789"], "confidence": 0.65}
  ],
  "projects": [
    {"name": "CLIC", "product_id": "prod-xyz", "expansion": "Cloud Infrastructure Compute"},
    {"name": "OSL", "product_id": "prod-uvw", "expansion": "Operational Service Layer"}
  ],
  "unresolved_terms": ["PLT"]
}
```

No model was used. This was all database lookups. But now the remote LLM in Stage 4 has **resolved context** to work with, not raw text.

---

## Stage 4: Deep Analysis (Remote LLM)

**What it does:** The reasoning work. Connecting information, understanding nuance, generating insights.

**Why a remote LLM:** This is where intelligence matters. Understanding that "we're making progress but need to watch a few areas" is diplomatically negative. Recognising that an email about VxLAN injection vulnerabilities relates to Risk #3 from the MTC meeting. Identifying that three separate action items from different people actually represent a disagreement about approach. These require a model with genuine reasoning capability.

### What gets sent to the LLM

Not the raw email. Instead, the LLM receives:

1. **The clean text** from Stage 0 (HTML stripped, quoted replies separated)
2. **The extracted entities** from Stage 2 (so the LLM doesn't have to re-extract them)
3. **The resolved context** from Stage 3 (entity IDs, glossary expansions, product mappings)
4. **Relevant background** from the knowledge base (more on this below)

### Building the context package

This is where Penfold's entity system pays off. Because Stage 3 resolved "CLIC" to a product ID and "Dan Spataro" to a person ID, we can pull relevant context from the database:

```
For product CLIC:
  - Recent product timeline events (decisions, risks, milestones)
  - Associated team and roles
  - Recent meeting summaries that mention CLIC

For person Dan Spataro:
  - Role and department
  - Recent action items assigned to them
  - Active projects they're involved in
```

This gives the LLM **background knowledge** that it wouldn't have from the email alone. Now when the email says "Dan needs to resolve the staffing issue", the LLM can see from the context that Dan was assigned an action item about CLIC staffing in last week's meeting, and this is a follow-up.

### The analysis prompt

```
You are analysing business content for a knowledge management system.

## Content
{clean text from Stage 0}

## Already Extracted (verified)
{Stage 2 + Stage 3 output - entities, dates, action items}

## Background Context
{relevant context pulled from Penfold's knowledge base}

## Analysis Required

1. SENTIMENT: Overall sentiment score (-1.0 to 1.0) with confidence.
   Consider business communication norms - diplomatic language often
   masks negative sentiment. "Areas to watch" often means problems.

2. TOPIC MAPPING: How does this content relate to the known projects
   and products listed in the background context? Identify specific
   connections, not general themes.

3. RISK & ISSUE IDENTIFICATION: Are any new risks or issues raised
   that aren't in the background context? Are any existing risks
   being updated or escalated?

4. IMPLICIT ACTION ITEMS: Beyond the explicitly stated action items
   (already extracted above), are there implied actions? Things that
   need to happen but weren't directly assigned?

5. STRATEGIC INSIGHTS: What should the reader take away from this
   content? What's the significance in the context of the active
   projects and known risks?

Respond as JSON.
```

### Model selection

Not every email that reaches Stage 4 needs the most expensive model:

| Triage result | Recommended model | Reasoning |
|--------------|-------------------|-----------|
| RISK_ISSUE, any importance | Gemini Pro | Risk analysis needs the best reasoning |
| CUSTOMER + HIGH importance | Gemini Pro | Customer-facing content needs accuracy |
| PROJECT_UPDATE + HIGH | Gemini Pro | Major project updates need good synthesis |
| PROJECT_UPDATE + MEDIUM | Gemini Flash | Standard updates, Flash is sufficient |
| ACTION_REQUEST + MEDIUM | Gemini Flash | Action extraction is relatively simple |
| Anything + LOW that reached Stage 4 | Gemini Flash | Lower stakes, save cost |

The `ai_routing_rules` table and `ModelRouter` in `services/ai/router/router.go` already support this kind of task-based routing. The `DefaultModelSelector` has `ModelMappings` by request type. We extend this to include the triage metadata (category + importance) in the routing decision.

---

## Stage 5: Embed and Index (SLM + Database)

**What it does:** Generates vector embeddings for semantic search and stores everything.

**Why the SLM:** Embedding generation is a mechanical transformation, not a reasoning task. The local `mxbai-embed-large-v1` model (1024 dimensions) running on Apple Silicon does this well. It's the same model already configured in `services/ai/backend/mlx.go`.

### What gets embedded

Not just the raw content. We embed **multiple representations** of the same content:

1. **The raw text** (for general semantic search)
2. **The summary** from Stage 4 (for high-level search queries)
3. **Extracted action items** (so "what did I need to do?" queries work)

Each embedding is stored in the `embeddings` table with a reference back to the source content and a label indicating which representation it is.

### Connecting to the glossary for search

When someone later searches "TER meeting notes about CLIC staffing", the search pipeline (in `services/gateway/searchservice/service.go`) expands the query using the glossary:

```
"TER meeting notes about CLIC staffing"
  -> "Technical Execution Review meeting notes about Cloud Infrastructure Compute staffing"
```

This expanded query produces better results from both keyword search (more matching terms) and vector search (the embedding of the expanded query is closer to the embedding of content that uses the full terms).

---

## The Feedback Loop: How the Knowledge Base Grows

The pipeline stages described above have a problem. They describe a one-way flow: content comes in, gets analysed, and the analysis is stored. But where does the **context** come from that makes Stage 4 useful? If the LLM is supposed to compare an email against "known risks from the MTC risk register," someone had to put those risks into the knowledge base first.

This is the chicken-and-egg problem. And getting it right is arguably more important than any individual pipeline stage.

### The current gap

Stages 1-5 describe extraction and analysis. But they don't describe what happens to the **output**. When Stage 4 identifies a new risk ("VxLAN injection vulnerability affects CLIC PLT"), where does that risk go? When it extracts a decision ("team agreed to defer the fix to the January maintenance window"), where is that decision stored? When it finds an action item ("Dan to resolve staffing by end of month"), does that become a trackable item?

Without a feedback loop, the pipeline is a read-only analysis engine. It produces good reports, but each new piece of content is analysed in isolation. The next email about the same topic doesn't know what was learned from the last one.

### What Penfold already has

The database schema already supports storing structured findings. The infrastructure exists; it's just not connected to the pipeline:

| What | Where it's stored | Schema |
|------|-------------------|--------|
| Risks, issues | `assertions` table (type='risk', 'issue') | severity, owner_person_id, project_id, is_current, superseded_by |
| Decisions | `assertions` table (type='decision') | rationale, decision_maker_person_id |
| Action items | `assertions` table (type='action') | assignee_person_id, due_date, status (open/in_progress/completed) |
| Commitments | `assertions` table (type='commitment') | committed_to_person_id, due_date |
| Product timeline events | `product_events` table | event_type (risk/decision/milestone), product_id, occurred_at |
| Content insights | `content_insights` table | insight_type, data (JSONB), per content_id |
| Mention patterns | `mention_patterns` table | pattern text, resolved entity, usage_count |

The `assertions` table even has versioning: `is_current` and `superseded_by` fields, so when a risk gets updated, the old version is preserved and the new one takes over.

### Stage 4.5: Persist Findings

After Stage 4 (Deep Analysis) produces its output, a new stage stores structured findings back into the knowledge base:

```
Stage 4 output (from Gemini Pro):
{
  "risks": [
    {"title": "VxLAN injection in CLIC PLT", "severity": "critical",
     "owner": "Dan Spataro", "status": "open"}
  ],
  "decisions": [
    {"title": "Defer fix to January maintenance window",
     "made_by": "Michael Merideth", "rationale": "Too close to holiday freeze"}
  ],
  "action_items": [
    {"assignee": "Melissa General", "action": "Pull together tiger teams for SLO gaps",
     "due": "Before OSL revenue start", "status": "open"}
  ]
}
       |
       v
Stage 4.5: Persist Findings
       |
       +---> Risks/Issues → assertions table (type='risk'/'issue')
       |     - Link to resolved person_id (owner) from Stage 3
       |     - Link to resolved product_id from Stage 3
       |     - Link to source_id (the content that raised this)
       |     - Check for existing assertions: is this a new risk or an
       |       update to one we've seen before?
       |     - If update: mark old assertion superseded_by new one
       |     - Also create product_event (type='risk') linked to product
       |
       +---> Decisions → assertions table (type='decision')
       |     - Link to decision_maker_person_id
       |     - Create product_event (type='decision')
       |     - Link product_event to source content via product_event_links
       |
       +---> Action Items → assertions table (type='action')
       |     - Link to assignee_person_id
       |     - Set status='open', due_date if specified
       |     - Check for duplicate/existing actions (same assignee, similar text)
       |
       +---> New Entities → review queue
       |     - Unknown acronyms → glossary review items
       |     - Unknown people → person review items (needs_review=true)
       |     - Unknown projects → flagged for manual creation
       |
       +---> Mention Patterns → mention_patterns table
             - "Dan" resolved to person_id p-abc123 with high confidence
             - Store pattern so future "Dan" in same project context
               auto-resolves without LLM
```

### Deduplication: is this a new risk or an update?

This is the hardest part of Stage 4.5. When the pipeline processes an email that mentions "the VxLAN vulnerability," is it a new risk or an update to the one we found in last week's meeting?

**For the SLM (can't do this):** Comparing a new risk description against a database of existing risks and deciding "this is the same risk, updated" requires semantic matching. A 7B model can't reliably do this.

**For the remote LLM (can do this):** Include existing assertions in the Stage 4 context package. The LLM can then say "this is an update to existing Risk #3" rather than "this is a new risk." This is why the context package matters.

**For code logic (partial):** If the same project, same risk keyword, and same people appear in an existing assertion and the new extraction, it's likely the same risk. Exact string matching won't work, but TF-IDF or embedding similarity between the new risk description and existing assertions can flag likely matches. Present those matches to the LLM in Stage 4 for confirmation.

The approach:

```
1. Stage 2 extracts raw risk text: "VxLAN injection vulnerability in CLIC PLT"
2. Stage 3 resolves CLIC to product_id prod-xyz
3. Stage 3 queries: SELECT * FROM assertions
     WHERE type='risk' AND project_id IN (projects linked to prod-xyz)
     AND is_current = true
   Result: 3 existing risks for CLIC-related projects
4. Stage 4 receives these existing risks as context
5. Stage 4 output: {"risk_id": "existing-assertion-42", "update": "severity escalated from high to critical, new info: affects PLT specifically"}
6. Stage 4.5: UPDATE assertions SET is_current=false, superseded_by=NEW_ID WHERE id=42
   INSERT INTO assertions (type, severity, ..., superseded_by) VALUES ('risk', 'critical', ...)
```

### The knowledge base lifecycle

This creates a virtuous cycle:

```
Week 1, Meeting 1:
  - No context exists yet (cold start)
  - Pipeline extracts: 5 risks, 3 decisions, 8 action items
  - Stage 4.5 creates all as NEW assertions
  - Product events created for each risk and decision

Week 1, Email about same project:
  - Stage 3 finds: 5 existing risks, 3 decisions for this project
  - Stage 4 receives these as context
  - LLM: "This email updates Risk #2 (severity now critical) and
    adds one new risk. Action Item #3 appears completed."
  - Stage 4.5: supersede old Risk #2, add new risk, close Action #3

Week 2, Meeting 2:
  - Stage 3 finds: 5 risks (1 superseded, 1 new since last meeting),
    3 decisions, 5 open action items
  - Stage 4 receives ALL of this as context
  - LLM can now say: "Risk #2 was escalated in email last week and
    discussed at length in this meeting. Team decided to..."
  - The analysis has continuity - it knows what came before
```

**This is how the system gets smarter over time.** The first meeting produces shallow analysis because there's no context. The fifth meeting about the same project produces deep analysis because the knowledge base now contains weeks of accumulated risks, decisions, and action items.

### What gets loaded as context for each content type

The context package for Stage 4 should be different depending on what we're processing:

**For a meeting transcript (most context-heavy):**
```
Context package:
1. Meeting series info (if this is a recurring meeting)
   - Previous meetings in the series
   - Previous meeting summaries (from content_insights)
2. Active assertions for the project
   - Open risks (is_current=true, type='risk')
   - Open action items (status='open', type='action')
   - Recent decisions (last 30 days, type='decision')
3. Product timeline events (last 60 days)
   - Milestones, releases, changes
4. Participant context
   - Role of each speaker (from people table)
   - What they own (action items assigned to them)
   - Their recent activity (mentions in other content)
5. Glossary terms relevant to the project
```

**For an email:**
```
Context package:
1. Thread history (previous emails in this thread, already processed)
   - Their summaries and extracted data
2. Active assertions for resolved products/projects
   - Focus on risks and open actions
3. Sender context
   - Who is this person, what do they own, what projects are they on
4. Glossary terms
```

**For Slack messages:**
```
Context package:
1. Channel context (what project/team is this channel for)
2. Active assertions for that project
3. Recent thread history (if this is a continuing thread)
4. Participant context (lightweight - just roles)
```

### People: where we need to be careful

The guide currently describes people resolution as a Stage 3 activity: match extracted names to known people. This is correct but incomplete.

**What's missing: relationship discovery.** When a meeting transcript says "Dan reports to Michael on the CLIC programme," that's a relationship fact. The `relationships` table (with types like `reports_to`, `works_on`, `member_of`) should be updated. Currently this would only happen if someone manually runs a CLI command.

**What's missing: expertise tracking.** When Dan is consistently extracted as the person who speaks about VxLAN and networking topics across multiple meetings, that's an expertise signal. The system should track this so that future queries like "who knows about VxLAN?" can answer from accumulated evidence, not just from a person's job title.

**What's missing: activity tracking.** "When did we last hear from Dan about CLIC?" should be answerable from the content_mentions table, but only if Stage 4.5 stores the mentions properly linked to person_ids.

The good news: the `entity_project_affinity` table already tracks mention counts and affinity scores per person-project pair. Stage 4.5 should update these scores as it processes content.

### Products and projects: closing the loop

The guide describes matching extracted project names to known products. But projects have active lifecycles:

**Status transitions.** If a meeting says "we've completed the GTC launch," the product event should be created (type='milestone') and the project status should be reviewed. Currently this would require a manual `penf product event add` command.

**Hierarchy discovery.** If emails keep mentioning "CLIC PLT" and "CLIC Core" and "CLIC Networking," the system should recognise these as sub-products of CLIC. The products table supports a three-level hierarchy (product -> sub_product -> feature), and the pipeline should be capable of suggesting hierarchy relationships when it sees consistent patterns.

**Keyword learning.** The projects table has a `keywords` field used for matching. If content consistently uses a term in the context of a project ("PLT" always appears near "CLIC"), that term should be added to the project's keywords for better future matching.

### Glossary: the quiet foundation

The glossary is arguably the most important entity type for the pipeline, because it affects every stage:

- **Stage 1 (Triage):** If the SLM sees "TER" and doesn't know what it means, triage quality suffers.
- **Stage 2 (Extract):** Extracted project names are only as good as the glossary that disambiguates them.
- **Stage 3 (Enrich):** Glossary expansion is the primary mechanism for matching extracted terms to entities.
- **Stage 5 (Search):** Query expansion depends entirely on the glossary.

**The feedback loop for glossary should be aggressive:**

1. Stage 2 extracts an unknown acronym (not in glossary)
2. Stage 3 flags it as unresolved
3. If Stage 4 (LLM) can infer the meaning from context, it should suggest an expansion
4. Stage 4.5 creates a glossary entry with `source='discovered'` and `needs_review=true`
5. A human confirms or corrects it during review
6. Once confirmed, all future content processing benefits from the expanded term

The glossary already supports this workflow: the `source` field distinguishes manual from discovered entries, and there's a review queue for unresolved items. What's missing is the automated discovery in Stage 4.

### Risks: should they be a first-class entity?

Currently, risks live in three places:
1. `assertions` table (type='risk') - structured, linked to people and projects
2. `product_events` table (type='risk') - linked to product timeline
3. `content_insights` table (type='risks') - JSONB blob per content item

This is confusing. The question is: should "Risk" become a first-class entity like People or Products?

**Arguments for:** Risks have identity (Risk #3), they persist across content, they get updated, they have owners, they have severity levels, they relate to projects and products. These are entity characteristics.

**Arguments against:** Risks are temporal. They're raised, tracked, and resolved. They don't have the permanence of a person or product. They're more like "events with lifecycle" than "entities."

**The recommendation:** Don't make risks a separate entity type. Instead, make the `assertions` table the single source of truth for tracked risks, with the `product_events` table as a denormalised view for timeline display. Drop the `content_insights` duplication - it's just adding confusion.

The assertion model already has the right structure:
- `is_current` / `superseded_by` handles versioning
- `severity` handles priority
- `owner_person_id` handles accountability
- `source_id` handles provenance
- `project_id` handles scope

What it needs is better **retrieval for context building.** A query like "give me all current risks for projects related to product CLIC" should be fast and standard. This is a view or a dedicated query in the gateway service, not a schema change.

### Bringing it all together: the context assembly query

When the pipeline reaches Stage 3 and needs to build a context package, here's what the query looks like:

```sql
-- Active risks for the products/projects mentioned in this content
SELECT a.description, a.severity, a.source_quote,
       p.canonical_name AS owner_name,
       pr.name AS project_name
FROM assertions a
LEFT JOIN people p ON a.owner_person_id = p.id
LEFT JOIN projects pr ON a.project_id = pr.id
WHERE a.type IN ('risk', 'issue')
  AND a.is_current = true
  AND (a.project_id IN (:resolved_project_ids)
       OR a.owner_person_id IN (:resolved_person_ids))
ORDER BY
  CASE a.severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2
       WHEN 'medium' THEN 3 ELSE 4 END;

-- Open action items for the people mentioned
SELECT a.description, a.due_date, a.status,
       p.canonical_name AS assignee_name
FROM assertions a
JOIN people p ON a.assignee_person_id = p.id
WHERE a.type = 'action'
  AND a.status = 'open'
  AND a.assignee_person_id IN (:resolved_person_ids);

-- Recent decisions for the project
SELECT a.description, a.rationale,
       p.canonical_name AS decision_maker
FROM assertions a
LEFT JOIN people p ON a.decision_maker_person_id = p.id
WHERE a.type = 'decision'
  AND a.project_id IN (:resolved_project_ids)
  AND a.created_at > now() - interval '60 days'
ORDER BY a.created_at DESC
LIMIT 10;

-- Recent product timeline events
SELECT pe.title, pe.description, pe.event_type, pe.occurred_at
FROM product_events pe
WHERE pe.product_id IN (:resolved_product_ids)
  AND pe.occurred_at > now() - interval '90 days'
ORDER BY pe.occurred_at DESC
LIMIT 15;

-- Glossary terms for context
SELECT g.term, g.expansion, g.definition
FROM glossary g
WHERE g.term IN (:extracted_acronyms)
  OR g.linked_entity_id IN (:resolved_product_ids);
```

This is what gets assembled into the "Background Context" section of the Stage 4 prompt. The richer this context is, the better the LLM's analysis. And the context gets richer over time because Stage 4.5 keeps feeding findings back in.

### The cold start problem

When you first deploy Penfold, the knowledge base is empty. No people, no glossary, no products, no risks. The first meeting or email batch has no context to work with.

**The bootstrap sequence:**

1. **Seed the glossary first.** Before processing any content, manually add the 20-30 most common acronyms and terms for your domain. `penf glossary add TER "Technical Execution Review"`. This takes 15 minutes and massively improves Stage 2 and Stage 3 quality from day one.

2. **Seed key people.** Add the 10-20 people you interact with most. Include aliases. This gives Stage 3 a head start on person resolution.

3. **Seed products/projects.** Add the 5-10 main products and projects. Include known aliases and keywords.

4. **Process a batch of recent content.** Run the pipeline on the last month of emails and recent meeting transcripts. Accept that the first pass will produce shallow analysis (no context), but it will populate the assertions table with risks, decisions, and action items.

5. **Review and correct.** Go through the review queue. Confirm or correct entity resolutions, glossary discoveries, and assertion extractions. This feedback improves future processing.

6. **Process again.** With a populated knowledge base, re-run Stage 4 on the important content. The analysis will be significantly better because the LLM now has context.

Step 6 is optional but valuable. The design principle "store intermediate results" means Stages 0-3 don't need to be re-run. Only Stage 4 (the expensive remote LLM call) gets re-run with better context.

---

## Tracking the Golden Thread: How Risks (and Decisions, Actions) Have a Lifecycle

The previous section described what gets stored. This section addresses a harder question: how do you follow the story of a risk from the moment it was raised through every escalation, decision, and eventual resolution? And how do you distinguish the three meetings where critical decisions were made from the seventeen meetings where someone said "oh, and the VxLAN thing is still open"?

### What we have vs what we need

The `assertions` table already has a versioning chain: `is_current` and `superseded_by`. Each time a risk is updated, a new assertion is created, the old one is marked `is_current=false` and points to the new one via `superseded_by`. This gives us a linked list of versions.

But it's missing three things:

1. **A root identity.** To find all versions of "the VxLAN risk," you need to walk backward from the current version through the superseded_by chain. There's no field that says "all of these assertions are versions of the same underlying risk." If you want to query "show me the history of Risk X," you need a recursive CTE every time.

2. **Lifecycle classification.** Each version is "a new assertion that supersedes an old one." But *why* was it superseded? Was the risk escalated? Was a decision made? Was it just mentioned and the description got slightly updated? The chain doesn't tell you what *kind* of change each version represents.

3. **Multi-content references.** Each assertion has a single `source_id` - the content that created it. But a risk might be discussed in a meeting for 30 minutes, mentioned in a follow-up email, and then resolved in another meeting. The assertion only points to one of these. You lose the connection to the other content.

### The data model additions

Two changes to the schema:

**1. Add a root identity and lifecycle event type to assertions:**

```sql
ALTER TABLE assertions ADD COLUMN assertion_root_id BIGINT
    REFERENCES assertions(id) ON DELETE SET NULL;

ALTER TABLE assertions ADD COLUMN lifecycle_event VARCHAR(50);
-- Values: raised, updated, escalated, de_escalated,
--         assigned, decided, deferred, resolved, reopened
```

`assertion_root_id` points to the *first* assertion in the chain - the one where the risk was originally raised. Every subsequent version of this risk shares the same root ID. This means "show me the history of Risk X" is:

```sql
SELECT * FROM assertions
WHERE assertion_root_id = :root_id
ORDER BY created_at;
```

No recursive CTE needed. And "show me all current risks" is still:

```sql
SELECT * FROM assertions
WHERE type = 'risk' AND is_current = true;
```

The `lifecycle_event` field classifies each version. The original assertion has lifecycle_event='raised'. An escalation is lifecycle_event='escalated'. A decision about the risk is lifecycle_event='decided'. Resolution is lifecycle_event='resolved'.

**2. Add a content reference table for multi-content linking:**

```sql
CREATE TABLE assertion_references (
    id BIGSERIAL PRIMARY KEY,
    assertion_id BIGINT NOT NULL REFERENCES assertions(id) ON DELETE CASCADE,
    assertion_root_id BIGINT NOT NULL REFERENCES assertions(id) ON DELETE CASCADE,
    source_id BIGINT NOT NULL,  -- the content item

    -- What role did this content play?
    reference_type VARCHAR(50) NOT NULL,
    -- origination:   this is where the risk was first raised
    -- escalation:    this content escalated the risk
    -- decision:      a decision about the risk was made here
    -- discussion:    substantive discussion (> passing mention)
    -- resolution:    the risk was closed/resolved here
    -- mention:       passing reference, no new information

    significance VARCHAR(20) NOT NULL,
    -- primary:       this content is a key moment in the lifecycle
    -- supporting:    provides additional evidence or context
    -- passing:       mentioned but no material change

    context_excerpt TEXT,       -- the relevant quote from the content
    occurred_at TIMESTAMPTZ,    -- when this reference happened
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_assertion_refs_root ON assertion_references(assertion_root_id);
CREATE INDEX idx_assertion_refs_source ON assertion_references(source_id);
CREATE INDEX idx_assertion_refs_significance ON assertion_references(significance);
```

### How this works in practice: the VxLAN story

Let's trace how the VxLAN vulnerability risk would be tracked across content:

**Meeting 1 (December 2):** TER weekly. Security team raises the VxLAN injection vulnerability for the first time.

```
assertions:
  id: 101, type: 'risk', assertion_root_id: 101, lifecycle_event: 'raised'
  description: "VxLAN injection vulnerability identified in CLIC PLT"
  severity: 'medium', owner_person_id: NULL, is_current: true
  source_id: <meeting-1-source-id>

assertion_references:
  assertion_id: 101, assertion_root_id: 101
  source_id: <meeting-1-source-id>
  reference_type: 'origination', significance: 'primary'
  context_excerpt: "We've found a VxLAN injection vulnerability in the PLT..."
```

**Email (December 5):** Dan sends an email about it, escalates severity.

```
assertions:
  id: 102, type: 'risk', assertion_root_id: 101, lifecycle_event: 'escalated'
  description: "VxLAN injection vulnerability - affects production, needs urgent fix"
  severity: 'critical', owner_person_id: <dan>, is_current: true
  superseded_by: NULL
  source_id: <email-source-id>

  -- AND update assertion 101:
  id: 101 -> is_current: false, superseded_by: 102

assertion_references:
  assertion_id: 102, assertion_root_id: 101
  source_id: <email-source-id>
  reference_type: 'escalation', significance: 'primary'
  context_excerpt: "This is more serious than initially thought. Affects production..."
```

**Meeting 2 (December 9):** TER weekly. VxLAN is one of 8 agenda items. Briefly discussed, no decisions made. Someone says "Dan's looking into it."

```
-- No new assertion version created (no material change to the risk)

assertion_references:
  assertion_id: 102, assertion_root_id: 101
  source_id: <meeting-2-source-id>
  reference_type: 'mention', significance: 'passing'
  context_excerpt: "Dan's still looking into the VxLAN issue"
```

**Meeting 3 (December 16):** TER weekly. VxLAN is the main topic for 25 minutes. Decision made to defer fix to January maintenance window.

```
assertions:
  id: 103, type: 'risk', assertion_root_id: 101, lifecycle_event: 'decided'
  description: "VxLAN injection - fix deferred to January maintenance window"
  severity: 'critical', owner_person_id: <dan>, is_current: true
  source_id: <meeting-3-source-id>

  -- AND: id: 102 -> is_current: false, superseded_by: 103

assertion_references:
  assertion_id: 103, assertion_root_id: 101
  source_id: <meeting-3-source-id>
  reference_type: 'decision', significance: 'primary'
  context_excerpt: "Michael decided to defer the VxLAN fix to the January maintenance window because..."

-- Also create a separate decision assertion:
assertions:
  id: 104, type: 'decision', assertion_root_id: 104, lifecycle_event: 'raised'
  description: "Defer VxLAN fix to January maintenance window"
  decision_maker_person_id: <michael>, rationale: "Too close to holiday freeze"
  source_id: <meeting-3-source-id>
```

**Meetings 4-7 (December-January):** Various meetings mention VxLAN in status updates. No material changes.

```
-- No new assertion versions. Just passing references:
assertion_references (multiple):
  reference_type: 'mention', significance: 'passing'
```

**Meeting 8 (January 20):** Fix deployed and verified. Risk resolved.

```
assertions:
  id: 108, type: 'risk', assertion_root_id: 101, lifecycle_event: 'resolved'
  description: "VxLAN injection vulnerability - fix deployed and verified"
  severity: 'critical', status: 'completed', is_current: true
  source_id: <meeting-8-source-id>

  -- AND: id: 103 -> is_current: false, superseded_by: 108

assertion_references:
  assertion_id: 108, assertion_root_id: 101
  source_id: <meeting-8-source-id>
  reference_type: 'resolution', significance: 'primary'
  context_excerpt: "The VxLAN fix was deployed last week and passed verification..."
```

### Querying the golden thread

**"Show me the lifecycle of the VxLAN risk":**

```sql
SELECT a.lifecycle_event, a.description, a.severity, a.status,
       p.canonical_name AS owner,
       a.created_at,
       s.subject AS source_content
FROM assertions a
LEFT JOIN people p ON a.owner_person_id = p.id
LEFT JOIN sources s ON a.source_id = s.id
WHERE a.assertion_root_id = 101
ORDER BY a.created_at;
```

Returns:
| lifecycle_event | severity | description | owner | date |
|----------------|----------|-------------|-------|------|
| raised | medium | VxLAN injection vulnerability identified in CLIC PLT | - | Dec 2 |
| escalated | critical | Affects production, needs urgent fix | Dan Spataro | Dec 5 |
| decided | critical | Fix deferred to January maintenance window | Dan Spataro | Dec 16 |
| resolved | critical | Fix deployed and verified | Dan Spataro | Jan 20 |

That's the golden thread. Four entries. Not the seventeen meetings where it was mentioned.

**"Where were the key decisions made about this risk?":**

```sql
SELECT ar.reference_type, ar.context_excerpt, ar.occurred_at,
       s.subject AS content_title,
       s.content_type AS content_type
FROM assertion_references ar
JOIN sources s ON ar.source_id = s.id
WHERE ar.assertion_root_id = 101
  AND ar.significance = 'primary'
ORDER BY ar.occurred_at;
```

Returns the 4 key moments: origination meeting, escalation email, decision meeting, resolution meeting. Not the noise.

**"Show me all content where this risk was discussed" (the full picture, if you want it):**

```sql
SELECT ar.reference_type, ar.significance, ar.context_excerpt,
       s.subject, s.content_type, ar.occurred_at
FROM assertion_references ar
JOIN sources s ON ar.source_id = s.id
WHERE ar.assertion_root_id = 101
ORDER BY ar.occurred_at;
```

This returns all 8+ references, including the passing mentions. But you can filter by significance.

### How Stage 4.5 observes what happened (and who said it)

> **Important:** The LLM observes and classifies what happened. It does not decide what's significant to the human. Significance depends on who said it (seniority + trust), whether it's on the human's watch list, and context the AI can't see. See "The Foundation: Human + AI Collaboration" at the top of this guide.

This is where the remote LLM earns its cost. When Stage 4 processes a meeting transcript, it receives the existing risk assertions as context. Its job includes classifying how the content relates to each risk, **and who was involved**:

```
Stage 4 prompt addition:

## Active Risks for Context
The following risks are currently tracked:

Risk #101 (root): "VxLAN injection vulnerability in CLIC PLT"
  Current status: severity=critical, owner=Dan Spataro
  Last update: Dec 5 - escalated from medium to critical

For each risk mentioned in the content, classify the mention:
- LIFECYCLE_CHANGE: The risk's status, severity, or owner changed
  -> specify: escalated | de_escalated | assigned | decided | deferred | resolved
  -> who triggered this change? (name and role)
- DISCUSSION: Substantive discussion (new information, debate, analysis)
  -> who were the key speakers? (names and roles)
- MENTION: Referenced in passing, no new information

For each person involved, include their role/title if known.
If a risk is not mentioned, omit it.
```

The LLM output:
```json
{
  "risk_references": [
    {
      "root_id": 101,
      "lifecycle_change": "decided",
      "significance": "primary",
      "new_description": "Fix deferred to January maintenance window",
      "context_excerpt": "Michael decided we can't risk a fix this close to...",
      "severity_change": null,
      "owner_change": null
    }
  ]
}
```

Stage 4.5 then:
1. Creates a new assertion version (id: 103) with `lifecycle_event='decided'`
2. Supersedes the previous version (id: 102)
3. Creates an `assertion_reference` with `reference_type='decision'`, `significance='primary'`

For a meeting where VxLAN is only mentioned in passing, the LLM output would be:
```json
{
  "risk_references": [
    {
      "root_id": 101,
      "lifecycle_change": null,
      "significance": "passing",
      "context_excerpt": "Dan mentioned the VxLAN thing is still in progress"
    }
  ]
}
```

Stage 4.5 then:
1. Does NOT create a new assertion version (no material change)
2. Creates an `assertion_reference` with `reference_type='mention'`, `significance='passing'`

### The SLM's role in risk tracking

The SLM doesn't decide significance - that requires reasoning. But it does contribute:

**Stage 1 (Triage):** If the SLM classifies content as RISK_ISSUE, this is a signal that the content may contain lifecycle events for existing risks. Ensure it reaches Stage 4.

**Stage 2 (Extract):** The SLM extracts raw risk text: "VxLAN injection vulnerability." This gets matched to the existing assertion in Stage 3 (by keyword overlap with existing risk descriptions + project match). The SLM doesn't need to know this is assertion #101; it just extracts the text.

**Stage 3 (Enrich):** Code logic matches the extracted risk text against existing assertions:

```sql
-- Find existing risks that might match the extracted text
SELECT id, assertion_root_id, description, severity
FROM assertions
WHERE type = 'risk' AND is_current = true
  AND project_id IN (:resolved_project_ids)
ORDER BY created_at DESC;
```

Then compute text similarity (TF-IDF or embedding distance) between the extracted risk text and each existing risk description. High similarity → this is a reference to an existing risk. Low similarity → this might be a new risk.

This matching is done by code (embedding similarity), not by the SLM. The result is passed to Stage 4 so the LLM can confirm and classify significance.

### Do we need a graph database?

No. Here's why:

The relationships we need to traverse are:
1. Assertion version chain (assertion → superseded_by → next version)
2. Assertion to content (assertion → source_id → sources, assertion_references → source_id → sources)
3. Assertion to entities (assertion → owner_person_id → people, assertion → project_id → projects)
4. Product hierarchy (product → parent_id → product)

These are all simple foreign key relationships. PostgreSQL handles them with standard joins. The assertion lifecycle query doesn't need a graph traversal; it's a simple `WHERE assertion_root_id = X ORDER BY created_at`. Even the "find all content connected to a risk" query is a single join through `assertion_references`.

Graph databases are valuable when you need to traverse unbounded-depth relationships ("find all people connected to Dan through no more than 3 degrees of separation"). Penfold's relationships are bounded: an assertion has one owner, belongs to one project, was created from one source, and references a known set of content items. Standard SQL handles this well.

The one place where a graph-like query might be useful is relationship discovery: "who works with whom, based on co-occurrence in meetings and email threads?" But even this is better modelled as a materialised view or a computed affinity score in the `entity_project_affinity` table (which already exists) than as a graph traversal.

### How this connects to the daily review and watch list

When you ask "what happened yesterday that I need to know about?", Claude assembles a daily review. But it prioritises differently based on your watch list and trust signals:

```
YOUR WATCHED RISKS:
  ★ Risk #101 (VxLAN vulnerability): DECIDED - fix deferred to January
    Source: TER Weekly Meeting, Dec 16
    Decision maker: Michael Merideth (VP Engineering - seniority: 6)
    ⚠ Note: This was on your watch list. A VP made the decision.

  ★ Risk #205 (CLIC staffing gap): ESCALATED - severity changed to high
    Source: Email from Dan Spataro (trusted: 5/5)
    Your note from last week: "Dan thinks this is worse than reported"

OTHER CHANGES:
  - Risk #312 (Missing SLOs): raised (NEW)
    Source: MTC Status Update email
    Raised by: Melissa General (Director - seniority: 5)
    → New risk from a Director. Want to add to your watch list?

  - Risk #415 (CDN capacity): quiet (no mentions in 8 days)
    Your note: "Mike says handled" (added Jan 28)
    → Close this one?

Open action items:
  - [Dan Spataro] Resolve CLIC staffing - due: end of month (open)
  - [Melissa General] Pull together tiger teams for SLO gaps - due: before OSL revenue (open)
```

Notice what's happening here:
- **Watched risks come first** with full context — because you told the system these matter
- **Trust and seniority are visible** — Dan is trusted (5/5), so his escalation is highlighted; a VP made a decision on a watched item
- **Claude prompts you** — "Want to add to your watch list?" for new risks from senior people; "Close this one?" for stale items with human notes suggesting resolution
- **Your own annotations surface** — the note you left last week about Dan's view is right there when you need it
- **Passing mentions are absent** — you don't see the 5 meetings where risks were mentioned in passing

This is the radar model in action: the AI tracked everything, the human focused the spotlight, and the daily review reflects both.

### Timeline queries for products

The `product_events` table now becomes a view into the assertion lifecycle, filtered to a specific product:

```sql
-- Complete timeline for CLIC product
SELECT
  COALESCE(pe.occurred_at, a.created_at) AS event_date,
  COALESCE(pe.event_type, a.type) AS event_type,
  COALESCE(pe.title, a.description) AS title,
  a.lifecycle_event,
  a.severity,
  p.canonical_name AS owner
FROM product_events pe
LEFT JOIN assertions a ON a.project_id IN (
    SELECT pr.id FROM projects pr WHERE pr.product_id = :product_id
  ) AND a.is_current = false  -- historical versions show the arc
JOIN people p ON COALESCE(a.owner_person_id, pe.recorded_by) IS NOT NULL
WHERE pe.product_id = :product_id
ORDER BY event_date;
```

This gives you a unified timeline: milestones, risks raised and resolved, decisions made, all in chronological order for a single product.

---

## Applying This to Different Content Types

### Short Emails (under 2,000 characters)

"Hi James, can you confirm you're free for the CLIC review on Thursday? Dan"

```
Stage 0: Extract headers (From: Dan, To: James, Subject: CLIC review)
Stage 1: Triage -> ACTION_REQUEST, LOW importance
          Reason: "Simple scheduling request for known project"
Stage 2: Extract -> people: [Dan, James], dates: [Thursday], projects: [CLIC]
Stage 3: Resolve -> Dan=p-abc, James=p-def, CLIC=prod-xyz
Stage 4: SKIP (low importance action request, no deep analysis needed)
Stage 5: Generate embedding, store
```

**Total model calls:** 2 SLM calls (triage + extract), 1 SLM embedding call.
**Remote LLM calls:** 0.

### Standard Company Comms

"Reminder: All employees must complete mandatory security awareness training by January 31st."

```
Stage 0: Extract headers
Stage 1: Triage -> INTERNAL_COMMS, LOW importance
          Reason: "Standard mandatory training reminder"
Stage 2: SKIP (low importance internal comms)
Stage 3: SKIP
Stage 4: SKIP
Stage 5: Generate embedding (so it's searchable), store
```

**Total model calls:** 1 SLM call (triage), 1 SLM embedding call.
**Remote LLM calls:** 0.

### Project Update Email (medium length)

"Great meeting with the MTC customer last night. Here's a summary of where we stand on the 2026 roadmap..."

```
Stage 0: Extract headers, strip HTML
Stage 1: Triage -> PROJECT_UPDATE, HIGH importance
          Reason: "Customer meeting summary about active project roadmap"
Stage 2: Extract people, dates, projects, action items, risks
Stage 3: Resolve all entities against knowledge base
         Pull background: recent MTC meeting notes, active risks, product timeline
Stage 4: Gemini Pro analysis (HIGH importance project update)
         -> sentiment, risk mapping, strategic insights
Stage 5: Embed raw text + summary + action items
```

**Total model calls:** 2 SLM calls + 1 Gemini Pro call + 3 embeddings.
**Remote LLM calls:** 1.

### Long Email Thread (debate between teams)

A 15-email thread about VxLAN injection vulnerabilities. Multiple participants, disagreements about approach, risk assessments.

```
Stage 0: Parse thread structure. Separate into individual messages.
         Identify quoted replies and deduplicate (don't re-process
         content already seen in earlier emails of the thread).
         Result: 15 individual messages, 8 of which contain new content.

Stage 1: Triage the THREAD (not individual emails).
         Use subject line + first email + last email (most recent).
         -> RISK_ISSUE, HIGH importance
         Reason: "Technical risk discussion about security vulnerability"

Stage 2: Extract from EACH new message individually (8 SLM calls).
         Each call gets one message (~500-2000 chars), well within SLM range.
         Merge extracted entities across all messages (code, not AI).
         Deduplicate: "Michael Merideth" in messages 3, 7, and 12 becomes
         one entity, not three.

Stage 3: Resolve all entities. Pull background context.
         Because this is RISK_ISSUE + HIGH, pull extra context:
         - Known risks from MTC risk register
         - Recent security-related meeting notes
         - Product vulnerability timeline events

Stage 4: Gemini Pro receives:
         - Thread summary: "{N} messages over {date range} between {participants}"
         - Per-message summaries from Stage 2 (not the raw text of all 15 emails)
         - Merged extracted entities (resolved)
         - Background context from knowledge base

         Gemini Pro analyses:
         - Thread arc: "Discussion started with vulnerability report,
           escalated through disagreement on timeline, appears unresolved"
         - Positions: "Team A argues for immediate fix, Team B argues
           for scheduled maintenance window"
         - Risk assessment: "Maps to existing Risk #3 (VxLAN Injection),
           new information: affects CLIC PLT specifically"
         - Action items: both explicit and implied

Stage 5: Embed thread summary + individual message summaries
```

**Total model calls:** 1 SLM triage + 8 SLM extractions + 1 Gemini Pro + multiple embeddings.
**Remote LLM calls:** 1 (but it received pre-processed, compressed input).

This is the key insight for long threads: **the SLM does the per-message extraction, and the LLM does the cross-message synthesis.** The LLM never sees the raw text of all 15 emails. It sees structured summaries and extracted data. This keeps the input well within context limits even for very long threads.

---

## Handling Meeting Transcripts

Meeting transcripts are a different beast from emails. A 1-hour meeting generates roughly 8,000-15,000 words of transcript (approximately 30,000-60,000 characters). That's far beyond what any local SLM can process in one call, and even some remote LLMs would struggle with the full text.

### The problem with transcripts

Transcripts have unique challenges:

1. **They're long.** An hour of conversation is a lot of text.
2. **They're noisy.** Auto-transcription produces errors, partial sentences, filler words, crosstalk.
3. **They're non-linear.** Conversations jump between topics, circle back, go on tangents.
4. **Speaker identification is unreliable.** "Speaker 3" might be labelled differently across segments.

### The transcript pipeline

```
Raw transcript (60,000 chars)
       |
       v
   Stage 0: Parse and Clean
   - Parse VTT/SRT format (extract timestamps + speaker labels)
   - Clean transcription artefacts
   - Normalise speaker labels
   - Segment by speaker turns
       |
       v
   Stage 0.5: Segment by Topic
   (SLM - lightweight)
   - Identify topic boundaries using timestamps and content shifts
   - Group speaker turns into topical segments
   - Result: 8-15 segments of 2,000-5,000 chars each
       |
       v
   Stage 1: Triage
   (SLM - on the meeting as a whole)
   - Uses meeting title + participants + first segment
   - Already know it's a meeting, so category is predetermined
   - Importance based on participants and topic
       |
       v
   Stage 2: Extract per Segment
   (SLM - one call per segment)
   - For each topical segment: extract people, decisions,
     action items, risks, topics discussed
   - 8-15 SLM calls, each processing 2,000-5,000 chars
   - Merge results across segments
       |
       v
   Stage 3: Enrich with Context
   (database lookups)
   - Resolve speakers to known people
   - Match discussed topics to products/projects
   - Pull relevant background for Stage 4
       |
       v
   Stage 4: Synthesise
   (Remote LLM)
   - Receives: segment summaries, extracted data, resolved context
   - NOT the full 60,000 character transcript
   - Produces: meeting summary, decision log, action items,
     risk updates, follow-ups needed
       |
       v
   Stage 5: Embed and Index
   - Embed meeting summary
   - Embed each topical segment summary
   - Embed action items separately (for "what do I need to do?" queries)
```

### Topic segmentation (Stage 0.5)

This is a task uniquely suited to an SLM. You're not asking it to understand the content - you're asking it to identify where the conversation shifts topic.

```
Below is a section of a meeting transcript with timestamps and speaker labels.
Identify where the topic changes. Return the timestamp boundaries.

For each segment, provide:
- start_time
- end_time
- topic_label (2-5 words describing the topic)

---
[00:00:15] Speaker 1: Let's start with the risk register update...
[00:03:45] Speaker 2: I wanted to flag the VxLAN issue...
[00:12:30] Speaker 1: OK, moving on to the staffing discussion...
```

The SLM is good at this because topic shifts are usually signalled explicitly ("let's move on to...", "next item...", "before we wrap up...") or by obvious content changes. It doesn't need to understand the content deeply - just detect the boundaries.

### Why segment-level extraction works

Each topical segment is typically 2,000-5,000 characters. That's comfortable for a 7B model. And critically, the entities and action items within a segment are usually self-contained:

- "Dan will follow up with the customer by Friday" doesn't need context from the segment about staffing to be extractable.
- "The VxLAN vulnerability is rated critical" doesn't need the budget discussion segment.

The things that **do** need cross-segment understanding - "the staffing issue is blocking the fix for the vulnerability" - are exactly what Stage 4 (remote LLM) handles, working from the per-segment summaries.

### Dealing with speaker identification

Auto-transcription often labels speakers as "Speaker 1", "Speaker 2", etc. Sometimes it mis-labels, splitting one person's speech across two speaker labels.

Handle this in Stage 0 (code, not AI):

1. If the meeting has a participant list (from calendar invite or meeting metadata), present the SLM with the speaker labels and participant list and ask it to map them. This is a simple matching task: "Speaker 1 talks about being the project lead for CLIC, and Dan Spataro is the project lead for CLIC" -> Speaker 1 = Dan Spataro.

2. If no participant list is available, use the Stage 2 extraction output - if Speaker 2 is referred to as "Mike" by other speakers, we know Speaker 2 is Mike.

3. Once mapped, replace speaker labels in all stored content.

---

## Handling Slack Messages

Slack is different again. Instead of one long document, you have potentially hundreds of short messages, threaded conversations, reactions, and channel context.

### The Slack challenge

- **Volume:** A busy channel can have 200+ messages per day.
- **Context dependence:** "I agree" means nothing without knowing what it's responding to.
- **Threads:** Slack threads are conversations within conversations.
- **Signal-to-noise:** A lot of Slack is social, reactions, "+1", emoji responses.
- **Fragmentation:** One person's thought might be split across 5 short messages.

### The Slack pipeline

```
Slack messages (batch of messages from a channel or time period)
       |
       v
   Stage 0: Parse and Structure
   (code only)
   - Group messages into threads (using thread_ts)
   - Separate top-level messages from thread replies
   - Expand user IDs to names
   - Identify message types (regular, bot, system, file_share)
   - Filter out system messages (joins, leaves, topic changes)
   - Concatenate rapid-fire messages from same user
     (messages within 60 seconds, same person -> merge into one)
       |
       v
   Stage 0.5: Thread Assembly
   (code only)
   - Each thread becomes one "document":
     "Channel: #mtc-project, Thread started by Dan Spataro
      Dan: {first message}
      Mike: {reply 1}
      Dan: {reply 2}
      ..."
   - Standalone messages (no thread) get grouped by time window
     (e.g., messages within 30 minutes that seem related)
       |
       v
   Stage 1: Triage per Thread
   (SLM)
   - Classify each assembled thread
   - Most threads will be PERSONAL or OTHER -> skip
   - Project discussions, decisions, risk flags -> continue
       |
       v
   Stage 2-5: Same as email pipeline
   (each thread treated like a short email)
```

### Grouping unthreaded messages

Not everyone uses Slack threads. In busy channels, you'll often see:

```
10:15 Dan: Anyone looked at the VxLAN vulnerability report?
10:16 Mike: Yeah, it's concerning. Affects the PLT in CLIC
10:17 Dan: We need to prioritise a fix
10:18 Sarah: I can take a look this afternoon
10:22 Alex: Unrelated - has anyone seen the new laptop policy?
10:23 Dave: @Alex yeah it's on the intranet
```

Stage 0.5 needs to group the related messages (10:15-10:18 = one conversation about VxLAN, 10:22-10:23 = separate conversation about laptop policy). This is done by:

1. **Time gap detection:** Messages more than 5 minutes apart are likely different conversations.
2. **Participant continuity:** If the same people are talking, it's likely the same conversation.
3. **SLM verification:** For ambiguous cases, show the SLM a block of messages and ask "how many distinct conversations are in this block, and where do they split?"

This last step is a good SLM task - it's classification/segmentation, not reasoning.

### Handling high-volume channels

For channels with 200+ messages/day, you don't want to process every message individually. The batching strategy:

1. Collect all messages from a time period (e.g., one day).
2. Group into threads and conversation clusters (Stage 0).
3. Triage the threads/clusters in a single batch SLM call:

```
Below are summaries of 15 Slack threads from #mtc-project today.
For each, classify as PROJECT/RISK/ACTION/SOCIAL/OTHER and rate importance.

1. "Dan asks about VxLAN vulnerability, team discusses fix timeline" (5 messages)
2. "Lunch plans for Thursday" (3 messages)
3. "Deployment schedule for next sprint" (12 messages)
...
```

One SLM call triages all 15 threads. Only the important ones proceed to extraction.

---

## The Vector Database and Semantic Search

Penfold uses PostgreSQL with pgvector for embeddings (the `embeddings` table) and standard full-text search on the `sources` table. The search pipeline in `services/gateway/searchservice/service.go` supports hybrid search with configurable weights between keyword and vector.

### How the pipeline feeds search

Every piece of content that passes through the pipeline produces multiple searchable artefacts:

| Artefact | Source | Embedding? | Full-text indexed? |
|----------|--------|------------|-------------------|
| Raw content | Stage 0 | Yes | Yes (sources.raw_content) |
| Triage classification | Stage 1 | No | Stored as metadata |
| Extracted entities | Stage 2 | No | Stored in content_mentions |
| Summary | Stage 4 | Yes | Yes |
| Action items | Stage 4 | Yes | Yes |
| Risk assessments | Stage 4 | Yes | Yes |

### How glossary expansion improves search

The glossary isn't just for display. It actively improves search quality:

**At index time:** When content mentions "TER", the enrichment stage (Stage 3) resolves this to "Technical Execution Review" and stores both the acronym and expansion. This means the content is findable by either term.

**At query time:** When someone searches for "TER meeting notes", the search service expands the query:

```go
// services/gateway/glossaryservice/service.go already does this:
// "TER" -> "(TER OR \"Technical Execution Review\")"
```

This double-expansion (at both index and query time) means search works whether the user uses the acronym, the full term, or a mix.

**Context-aware disambiguation:** The glossary supports context tags. If "VIP" means "Very Important Person" in a sales context but "Virtual IP Address" in a networking context, the glossary can disambiguate based on surrounding terms. When searching in #networking-channel, "VIP" expands to the networking definition.

### How entity resolution improves search

Because Stage 3 resolves "Dan" to person ID `p-abc123`, a search for "Dan Spataro's action items" can:

1. Resolve "Dan Spataro" to `p-abc123`
2. Query all content where `p-abc123` is mentioned
3. Filter to content with extracted action items assigned to `p-abc123`

This is more reliable than text search for "Dan" (too common) or "Dan Spataro" (might be referred to as "Dan" or "DS" in some content).

### Multi-level embeddings

For meeting transcripts and long email threads, we generate embeddings at multiple levels:

1. **Full content embedding:** The complete summary, for high-level semantic matching.
2. **Segment/message embeddings:** Each topical segment or important message, for fine-grained matching.
3. **Entity-context embeddings:** The context around each entity mention, so "what did Dan say about CLIC?" can find the specific segment.

The search service first finds matches at the segment level, then retrieves the full content for context. This is more accurate than embedding the entire transcript as one vector (which dilutes the signal).

---

## Cost Model and Performance Expectations

### Per-email cost breakdown

| Content type | SLM calls | Remote LLM calls | Approximate cost |
|-------------|-----------|-------------------|-----------------|
| Personal/social | 1 (triage) | 0 | Free |
| Internal comms (low) | 1 (triage) | 0 | Free |
| Internal comms (action) | 2 (triage + extract) | 0 | Free |
| Project update (medium) | 2 + embeddings | 1 Flash | ~$0.001-0.005 |
| Project update (high) | 2 + embeddings | 1 Pro | ~$0.01-0.05 |
| Risk/escalation | 2 + embeddings | 1 Pro | ~$0.01-0.05 |
| Long thread (high) | 2 + N extractions + embeddings | 1 Pro | ~$0.02-0.10 |

### Batch processing of 100 emails

Typical distribution for a business user:

| Category | Count | SLM calls | Remote calls | Cost |
|----------|-------|-----------|--------------|------|
| Personal/social | 15 | 15 | 0 | Free |
| Internal comms (low) | 25 | 25 | 0 | Free |
| Internal comms (action) | 10 | 20 | 0 | Free |
| Standard project updates | 25 | 50 | 25 Flash | ~$0.05-0.10 |
| Important updates | 15 | 30 | 15 Pro | ~$0.15-0.75 |
| Risk/escalation | 5 | 10 | 5 Pro | ~$0.05-0.25 |
| Long threads | 5 | 50+ | 5 Pro | ~$0.10-0.50 |
| **Totals** | **100** | **~200** | **~50** | **~$0.35-1.60** |

Without the pipeline (sending everything to Gemini Pro): 100 calls at ~$0.03-0.10 each = **$3-10.**

The pipeline reduces remote LLM costs by roughly 70-80%.

### SLM throughput on Apple Silicon

On an M-series Mac running a 7B 4-bit quantised model:

- Triage call (~500 chars input, ~50 chars output): ~1-3 seconds
- Extraction call (~3,000 chars input, ~200 chars output): ~3-8 seconds
- Embedding generation: ~0.5-1 second per text

For 100 emails with ~200 SLM calls, that's roughly 5-15 minutes of SLM processing. This can be parallelised across multiple requests since the MLX server handles concurrent connections (up to memory limits).

---

## 7B vs 32B: Which Local Model for Which Stage?

The Mac Mini (M-series, likely 16-32GB unified memory) can run either a 7B or 32B quantised model, but not both simultaneously, and the tradeoffs are significant. This isn't just a speed question - it's a question about what's possible at each pipeline stage.

### The hardware reality

| Model | Quantisation | Memory footprint | Tokens/sec (M4) | Tokens/sec (M4 Pro) | Context window |
|-------|-------------|-----------------|-----------------|---------------------|----------------|
| Qwen 2.5 7B | 4-bit (Q4_K_M) | ~5 GB | ~40-60 tok/s | ~60-80 tok/s | 32K tokens |
| Qwen 2.5 14B | 4-bit (Q4_K_M) | ~9 GB | ~20-35 tok/s | ~35-50 tok/s | 32K tokens |
| Qwen 2.5 32B | 4-bit (Q4_K_M) | ~19 GB | ~8-15 tok/s | ~15-25 tok/s | 32K tokens |
| Qwen 2.5 32B | 3-bit (Q3_K_M) | ~15 GB | ~10-18 tok/s | ~18-30 tok/s | 32K tokens |

On a 16GB Mac Mini: the 32B 4-bit model leaves almost no headroom. The system itself needs 3-4GB, MLX server overhead takes another 1-2GB, and you're into swap. Performance collapses. The 32B 3-bit model fits, but 3-bit quantisation measurably degrades quality - you lose some of the reasoning advantage you were paying for in speed.

On a 24GB or 32GB Mac Mini: the 32B 4-bit model fits comfortably. But you're still 3-5x slower per token than the 7B.

### What the speed difference actually means in practice

Let's be concrete about what "slower" means for the pipeline:

| Task | Input | Output | 7B time | 32B time |
|------|-------|--------|---------|----------|
| Triage (Stage 1) | ~200 tokens | ~30 tokens | 1-2 sec | 3-6 sec |
| Extraction (Stage 2) | ~1000 tokens | ~150 tokens | 4-8 sec | 15-30 sec |
| Extraction from chunk | ~500 tokens | ~100 tokens | 2-5 sec | 8-15 sec |
| Topic segmentation | ~2000 tokens | ~100 tokens | 5-10 sec | 20-40 sec |
| Batch triage (10 items) | ~800 tokens | ~200 tokens | 5-10 sec | 20-40 sec |

For a batch of 100 emails:
- **7B pipeline:** ~200 SLM calls = 5-15 minutes total
- **32B pipeline:** ~200 SLM calls = 25-80 minutes total

That's the difference between "process overnight emails before the morning briefing" and "still processing when lunch arrives."

### Where the 32B is actually worth it

The 32B model isn't uniformly better. It's specifically better at tasks where the 7B falls down:

**Complex extraction from ambiguous text.** When an email says "Mike mentioned the issue to Dan's team last week and they're looking into it", a 7B model might extract Mike and Dan as people but miss that "Dan's team" is a team reference, or that "the issue" refers to something specific. A 32B model is better at this kind of coreference resolution.

**Triage of forwarded or multi-topic emails.** An email that looks like internal comms but contains a forwarded customer escalation. The 7B model reads the first 500 chars (the forwarding note) and classifies as INTERNAL_COMMS/LOW. The 32B model is more likely to look at the forwarded content and classify correctly.

**Extraction from noisy transcripts.** Auto-transcription produces garbled text. "We need to, um, you know, get the, the VxLAN thing sorted before, what was it, January?" A 32B model handles this better because it can resolve the disfluencies.

**Structured output reliability.** The 32B model produces valid JSON more consistently. On a 7B model, you might see 5-10% malformed JSON responses. On a 32B, it's closer to 1-2%.

### Where the 7B is sufficient (or better)

**Triage of clear-cut content.** "Mandatory compliance training" from hr@company.com doesn't need a 32B model to classify as INTERNAL_COMMS/LOW. The subject line says it all.

**Extraction from clean, well-structured text.** Business emails that clearly state "Action: Dan to complete the review by Friday" are easy extraction targets for any model.

**Embeddings.** The embedding model (mxbai-embed-large-v1) is separate from the LLM and runs fine on its own. Model size for the LLM doesn't affect embedding quality.

**Any task where speed matters more than marginal quality improvement.** If you're processing 300 Slack messages, the 3-5x speed penalty of the 32B model isn't worth a small improvement in extraction accuracy.

### The recommended approach: task-based model selection

Don't choose one model. Run both, and route tasks to the right one.

The MLX server can't load two models simultaneously (memory constraint), but it can **swap models** between tasks. Model loading on MLX takes 5-15 seconds depending on model size. This means swapping per-request is impractical, but swapping per-batch is viable.

**Strategy 1: Time-based model swapping**

```
Night batch (low time pressure):
  - Load 32B model
  - Process all extraction tasks (Stage 2) that accumulated during the day
  - Process transcript segmentation (Stage 0.5)
  - Swap to 7B model when done

Daytime (responsiveness matters):
  - Load 7B model
  - Handle triage (Stage 1) for incoming content in real-time
  - Handle on-demand queries and search
  - Queue extraction tasks for night batch if quality matters,
    or run them on 7B if speed matters
```

**Strategy 2: Complexity-based routing (single model loaded)**

If model swapping is too operationally complex, use the 7B model for everything but add quality gates:

```
Stage 1: Triage with 7B
  -> If triage confidence is low (generic reason, ambiguous content):
     flag for re-triage with 32B in next batch window

Stage 2: Extract with 7B
  -> If JSON is malformed: retry once, then queue for 32B
  -> If extraction yields zero entities from non-trivial content: queue for 32B
  -> If content is a noisy transcript: queue for 32B directly
```

This means 90% of content is processed quickly by the 7B, and only the hard cases wait for the 32B.

**Strategy 3: 7B for everything, remote LLM for quality**

The most pragmatic approach for a 16GB Mac Mini: accept the 7B's limitations and use the remote LLM (Gemini Flash, which is cheap and fast) to cover the gap.

```
Stage 1: Triage with 7B (fast, good enough for classification)
Stage 2: Extract with 7B (fast, handles most content)
  -> If content is complex/long/noisy: send to Gemini Flash for extraction
     (Flash is cheap enough that this isn't a cost concern)
Stage 3: Enrich (code, no model)
Stage 4: Gemini Pro/Flash (already remote)
```

This eliminates the 32B model entirely. The cost increase from sending some extraction to Gemini Flash is marginal (Flash is ~$0.001-0.003 per extraction call). The operational simplicity of running one local model is significant.

### The honest recommendation

**If you have a 16GB Mac Mini:** Use the 7B for Stages 1-2 and Gemini Flash as your "better local model" fallback. Don't run the 32B - the memory pressure and speed penalty aren't worth it. The pipeline is designed so that SLM quality at Stages 1-2 doesn't need to be perfect; Stage 4 (remote LLM) is where the real intelligence happens.

**If you have a 24-32GB Mac Mini:** The 32B becomes viable. Use Strategy 1 (time-based swapping) for batch processing, or Strategy 2 (complexity-based routing with 7B as default). The 32B is genuinely better at extraction from messy content, and with enough memory it runs at acceptable speed.

**If you later get an M4 Pro/Max with 48-64GB:** You could keep both models loaded simultaneously (MLX supports this with model caching). Route triage to 7B and extraction to 32B without swapping. This is the ideal setup but requires the hardware.

### What about the 14B model?

The Qwen 2.5 14B is an interesting middle ground:
- Fits comfortably on 16GB (9GB model + headroom)
- ~2x slower than 7B (not 5x like the 32B)
- Meaningfully better at structured extraction than 7B
- Still not as good at complex reasoning as 32B

For the pipeline, the 14B might be the sweet spot on 16GB hardware: good enough extraction quality that you rarely need to fall back to remote LLM, fast enough that batch processing completes in reasonable time. Worth benchmarking against the 7B on your actual content to see if the quality improvement justifies the 2x speed penalty.

### How to decide: benchmark on your data

The right choice depends on your actual content. Run a benchmark:

1. Take 50 representative emails (mix of types from your inbox).
2. Run Stage 1 triage on all 50 with both 7B and 32B. Compare classifications.
3. Run Stage 2 extraction on 20 non-trivial emails with both. Compare extracted entities against a manually-created ground truth.
4. Measure: how many extractions does the 32B get right that the 7B gets wrong?
5. Multiply by the speed difference. Is the quality improvement worth 3-5x slower processing?

If the 7B gets triage right 95% of the time and the 32B gets it right 98%, the 3% improvement probably isn't worth 5x slower processing. If the 7B gets extraction right 70% and the 32B gets 90%, that's a different calculation.

---

## How This Maps to Penfold's Existing Code

### What already exists

| Component | Location | Status |
|-----------|----------|--------|
| Content chunking with overlap | `services/worker/activities/content_activities.go` | Working |
| Model router with fallback | `services/ai/router/router.go` | Working |
| Multiple backends (MLX, OpenAI, Gemini) | `services/ai/backend/` | Working |
| Glossary expansion | `services/gateway/glossaryservice/` | Working |
| Entity mention resolution | `services/gateway/mentionsservice/` | Working |
| Embedding generation | `services/ai/server/server.go` | Working |
| Search with hybrid weights | `services/gateway/searchservice/` | Working (keyword), vector TODO |
| Health checks & circuit breakers | `services/ai/router/` | Working |
| Langfuse tracing | `services/ai/server/server.go` | Working |
| People, products, teams entities | `services/gateway/entityservice/` | Working |
| Review queue for unresolved items | `services/gateway/` | Working |
| Meeting transcript ingestion | `cmd/penf/cmd/ingest_meeting.go` | Working (VTT, text, chat) |

### What needs to be built

| Component | Description |
|-----------|-------------|
| **Triage RPC** | New gRPC method: `TriageContent(text, metadata) -> {category, importance}`. Routes to local SLM only. |
| **Extract RPC** | New gRPC method: `ExtractEntities(text) -> {people, dates, projects, ...}`. Routes to local SLM. Handles chunking for long content. |
| **Context Builder** | Service that takes Stage 2 output, resolves entities (Stage 3), and builds the context package for Stage 4. Orchestrates existing services. |
| **Pipeline Orchestrator** | Coordinates the full Stage 0-5 flow. Decides what skips, what continues. Could be a Temporal workflow (worker already uses Temporal). |
| **Routing rules update** | Extend `ModelSelector` to route based on triage metadata (category + importance), not just request type. |
| **Thread parser** | Stage 0 logic for email threads: detect quoted replies, separate new content, reconstruct thread order. |
| **Slack message grouper** | Stage 0 logic for Slack: thread assembly, rapid-fire merging, conversation clustering. |
| **Transcript segmenter** | Stage 0.5 for meetings: topic boundary detection using SLM. |
| **Multi-level embeddings** | Generate and store embeddings at content, summary, and segment levels. |
| **Updated Analyze RPC** | Replace the single-call `AnalyzeByID` with the full pipeline. Keep the same external API but change the internals. |

### What gets modified

| Component | Change |
|-----------|--------|
| `buildAnalysisPrompt()` in `service.go` | Replace with pipeline orchestration. The prompt becomes the Stage 4 prompt only, receiving pre-processed input. |
| `truncateText(content, 8000)` | Remove. The pipeline handles content size at each stage. |
| `DefaultModelSelector` | Extend to consider content category and importance for model selection. |
| Content enrichment tables | Add columns for triage results, pipeline stage tracking. |

---

## Design Principles for the Pipeline

**1. Each stage should be independently testable.** You should be able to run Stage 2 (extraction) on a piece of text and verify the output without running the full pipeline. This means each stage is its own gRPC method.

**2. Stages should be idempotent.** Running Stage 2 twice on the same content should produce the same result. This enables retries and reprocessing.

**3. Store intermediate results.** The output of each stage is stored in the database. If Stage 4 fails (Gemini is down), you don't lose the work from Stages 1-3. When Gemini comes back, you pick up from where you left off.

**4. The SLM should never block on the LLM.** Stages 1-3 (all local) can run immediately on ingestion. Stage 4 (remote) can be queued and processed asynchronously. The content is searchable (via keyword + embeddings) as soon as Stage 2 completes, before Stage 4 finishes.

**5. The pipeline configuration should be adjustable per tenant.** Some users might want everything analysed deeply. Others might want minimal processing. The triage thresholds and Stage 4 model selection should be configurable via `ai_routing_rules`.

**6. Fail loudly.** If the SLM produces invalid JSON, if entity resolution fails, if the LLM returns an error - log it, flag it for review, don't silently produce bad data. This aligns with Penfold's engineering principles: "fail loudly, succeed quietly."

---

## Frequently Asked Questions

### "Why not just use Gemini Flash for everything? It's cheap."

It is cheap per-call, but the costs add up at volume. More importantly, using a remote API for every triage and extraction call introduces latency and a dependency on network availability. If Google's API is slow or down, your entire ingestion pipeline stops. With local SLMs handling the first stages, ingestion continues regardless of remote API availability. The remote LLM is only needed for the final analysis step, which can be queued and retried.

### "What if the SLM gets the triage wrong?"

It will sometimes. The most likely error is classifying something as lower importance than it deserves. Mitigations:

- **Keyword triggers:** Regardless of SLM triage, if the content contains known high-priority keywords (specific project names, risk-related terms from the glossary), override the importance to HIGH.
- **Sender-based rules:** Emails from certain senders (your manager, key customers, specific distribution lists) always get full processing regardless of triage.
- **Periodic review:** Sample triage results and check for systematic errors. If the SLM consistently misclassifies a certain type of email, add it as a rule.
- **User correction:** If a user searches for something and the content wasn't properly analysed, flag it for reprocessing through the full pipeline.

### "Can the SLM handle non-English content?"

The Qwen 2.5 7B model has reasonable multilingual support, particularly for European languages and Chinese. For triage and basic extraction, it should work adequately. For deep analysis in non-English languages, route to Gemini Pro which has stronger multilingual capabilities. The triage stage could include language detection and use that as a routing signal.

### "How do we handle attachments (PDFs, spreadsheets)?"

Attachments need a separate pre-processing step before entering the pipeline:

1. **PDFs:** Extract text using a PDF library (not an LLM). Then the extracted text enters the pipeline at Stage 1 like any other content.
2. **Spreadsheets:** Extract cell data, convert to a text representation. The SLM can triage and extract from the text representation.
3. **Images:** These need a multimodal model (Gemini Pro Vision). Route directly to Stage 4 with a vision-capable model.
4. **Large documents (50+ pages):** Chunk at the page or section level, process each chunk through Stages 2-3, then synthesise in Stage 4.

### "What about privacy? Are we sending sensitive content to Google?"

Only content that reaches Stage 4 goes to a remote API. The pipeline's triage stage (Stage 1) means most content stays entirely local. For content that does reach Stage 4:

- All Penfold data stays within your control up to Stage 3.
- Stage 4 sends pre-processed extracts and summaries, not always the raw content (depending on configuration).
- Gemini API data is not used for training by default under Google's API ToS.
- For highly sensitive content, you could add a sensitivity classification in Stage 1 and route sensitive content to a more capable local model (when available) rather than the cloud.

### "What happens when we upgrade to a larger local model?"

The pipeline design is model-agnostic. If you later run a 14B or 32B model locally:

- Stage 2 extraction quality improves, especially for long or complex content.
- Some content that currently requires Stage 4 (Gemini) could be handled locally.
- The triage thresholds in the routing rules adjust: more categories get processed locally.
- The pipeline structure doesn't change - just the routing decisions.

This is why the `ai_routing_rules` table and `ModelSelector` exist. You change the routing, not the pipeline.

---

## Prompt Engineering: SLMs Need Different Prompts Than LLMs

This is easy to get wrong. The instinct is to write one prompt and use it everywhere. But SLMs and LLMs respond differently to prompt structure, and getting this right is the difference between a pipeline that works and one that produces garbage.

### Rules for SLM prompts

**1. One task per prompt.** Never ask a 7B model to do two things at once. "Classify this email AND extract entities" will produce worse results on both tasks than two separate calls. The current `buildAnalysisPrompt()` asking for five things simultaneously is exactly the anti-pattern.

**2. Show the output format explicitly.** Don't say "respond as JSON." Say:

```
Respond with ONLY this JSON, no other text:
{"category": "PROJECT_UPDATE", "importance": "HIGH", "reason": "customer meeting summary"}
```

The example values in the format template act as few-shot guidance. The model sees what a correct response looks like.

**3. Keep the system prompt short.** A 7B model doesn't benefit from elaborate role descriptions. "You are a content classifier" is enough. "You are an expert AI assistant specialised in business intelligence analysis with deep experience in enterprise communication patterns" wastes tokens and doesn't improve output.

**4. Put the content LAST.** Structure prompts as: instruction, output format, then content. If the content comes first, the model may start generating before it's "seen" the instructions (especially with longer inputs where attention degrades).

**5. Constrain the output aggressively.** "Rate importance as HIGH, MEDIUM, or LOW" is better than "Rate importance on a scale of 1-10." Fewer options = more reliable classification. If you need granularity, use two-stage classification: first coarse (HIGH/MEDIUM/LOW), then fine within the selected category.

### Rules for LLM prompts (Stage 4)

**1. Provide context, not just content.** The LLM's value is in reasoning, so give it material to reason about. The Stage 3 enrichment output (resolved entities, glossary expansions, relevant background) is as important as the content itself.

**2. Separate facts from analysis.** Tell the LLM what's already been extracted (Stage 2 output) and ask it to focus on what requires reasoning. "The following entities have already been extracted and verified. Focus your analysis on: connections between entities, implied actions, and strategic significance."

**3. Be specific about what "analysis" means.** "Analyse this email" is vague. "Identify how this email relates to the three active risks listed in the background context" is specific. The LLM produces better output when the reasoning task is clearly scoped.

**4. Ask it to show its reasoning.** For risk mapping and sentiment analysis, ask the LLM to explain why. "Score: -0.3, because the phrase 'areas we need to watch' in a business context typically indicates known problems being downplayed." This makes the output auditable and helps you judge quality.

### Example: triage prompt that works well with a 7B model

```
Classify this email. Pick ONE category and ONE importance level.

Categories: PROJECT_UPDATE, CUSTOMER, RISK_ISSUE, ACTION_REQUEST, DECISION, INTERNAL_COMMS, PERSONAL, OTHER
Importance: HIGH, MEDIUM, LOW

Respond with ONLY this JSON:
{"category": "INTERNAL_COMMS", "importance": "LOW", "reason": "routine company announcement"}

---
Subject: Re: MTC Risk Register - VxLAN Update
From: Dan Spataro <dan.spataro@company.com>

Team, following up on the discussion from yesterday's steering committee...
```

Notice: the example JSON in the format template shows INTERNAL_COMMS/LOW, which is deliberately different from what this email should be classified as. This prevents the model from just copying the example. A good 7B model will correctly output RISK_ISSUE/HIGH here because the content clearly signals it.

---

## Progressive Availability: Content Is Useful Before the Pipeline Finishes

One of the most important architectural properties of this pipeline: **content becomes searchable immediately, not after the full analysis completes.**

### Timeline of availability

```
T+0s     Content ingested, raw text stored in sources table
         -> Immediately searchable via keyword/full-text search

T+2s     Stage 1 triage complete
         -> Content is classified and tagged
         -> Filters and faceted search work ("show me all RISK_ISSUE emails")

T+10s    Stage 2 extraction complete
         -> Entities extracted and stored in content_mentions
         -> Entity-based search works ("emails mentioning Dan Spataro")
         -> Basic embedding generated
         -> Semantic search works

T+15s    Stage 3 enrichment complete
         -> Entities resolved to known people/products/glossary
         -> Relationship-based search works ("emails about CLIC project")

T+30-60s Stage 4 analysis complete (async, remote)
         -> Deep analysis, sentiment, insights available
         -> Summary embedding generated
         -> Rich search results with analysis snippets
```

This means the system is useful even when Gemini is down. Stages 0-3 all run locally. You lose deep analysis, but search, entity extraction, and classification all work. The Temporal workflow (already used by the worker service) can queue Stage 4 work and process it when the remote API is available.

This also means the `penf ai analyze` command could work in two modes:

1. **If Stage 4 is already done:** Return the stored analysis immediately.
2. **If only Stages 1-3 are done:** Return the triage + extraction results, and optionally trigger Stage 4 on-demand.

---

## Validating SLM Output Quality

Running a local 7B model means you're responsible for quality in a way you're not when using a managed API. You need to know when it's working and when it's not.

### Structured output validation

Every SLM call should validate the output before accepting it:

1. **JSON parsing.** If the model was asked for JSON and the response doesn't parse, it's a failure. Don't try to fix malformed JSON - retry the call (up to 2 retries), then flag for review if it still fails.

2. **Schema validation.** The JSON parsed, but does it have the expected fields? Is `category` one of the allowed values? Is `importance` one of HIGH/MEDIUM/LOW? Reject outputs that don't conform.

3. **Sanity checks.** An email with subject "URGENT: Production down" classified as PERSONAL/LOW is probably wrong. Simple rule-based checks can catch the most obvious SLM errors:
   - Subject contains "urgent", "critical", "production", "outage" -> importance must be HIGH
   - Sender is in the VIP list -> importance cannot be LOW
   - Content contains known project keywords -> category cannot be PERSONAL

4. **Confidence heuristic.** If the SLM's JSON reason is very short or generic ("general email"), treat the classification as low-confidence and route to Stage 2 regardless.

### Tracking quality over time

Log every triage and extraction result to Langfuse (already integrated). Periodically review a sample:

- What percentage of emails did the SLM triage as PERSONAL that were actually project-related?
- What percentage of extracted entities were successfully resolved in Stage 3? (Low resolution rate might mean extraction quality is poor.)
- What percentage of Stage 4 analyses contradicted Stage 1 triage? (If the LLM frequently upgrades importance, the SLM threshold is too conservative.)

### When to distrust the SLM

Be especially cautious with:

- **Forwarded emails.** The outer email might be "FYI" from a colleague, but the forwarded content is a critical customer escalation. The subject line and first paragraph may not reflect the actual content.
- **Newsletter-style emails.** Long emails with multiple topics. Triage based on the first 500 chars might miss important content further down.
- **Emails with minimal text.** "See attached" or "Thoughts?" with an attachment. The real content is in the attachment, not the email body. Flag these for attachment processing.

For these cases, add heuristic rules that override or supplement the SLM triage:
- If email is a forward (detected in Stage 0 from headers), always run Stage 2 on the forwarded content separately.
- If email body is under 100 characters and has attachments, classify as needs-attachment-review.
- If email is from a known distribution list for a tracked project, always route to Stage 2 minimum.

---

## How This Connects to Daily Review (UC-5)

Penfold's use case UC-5 (Daily Review & Triage, described in `context/shared/use-cases.md`) is where this pipeline delivers its highest value. The vision is a morning briefing that tells you:

> "You received 47 emails yesterday. 3 require your action. The MTC risk register was updated - VxLAN vulnerability escalated to critical. Dan Spataro is waiting for your input on CLIC staffing."

This becomes possible because:

1. **Stage 1 triage** already sorted the 47 emails into categories and importance levels.
2. **Stage 2 extraction** already identified the action items assigned to you.
3. **Stage 3 enrichment** already resolved the entities and linked them to known projects.
4. **Stage 4 analysis** (for the important ones) already identified the risk escalation and the implicit request for your input.

The daily review is a **read** operation on pipeline results, not a new processing step. The pipeline runs as content arrives (or in nightly batches). The daily review aggregates and presents the results.

### Daily review query pattern

```sql
-- Emails from yesterday requiring action
SELECT s.content_id, s.subject, e.triage_category, e.triage_importance,
       e.extracted_action_items, e.analysis_summary
FROM sources s
JOIN content_enrichment e ON s.id = e.source_id
WHERE s.source_timestamp > now() - interval '24 hours'
  AND e.triage_importance IN ('HIGH', 'MEDIUM')
  AND e.triage_category IN ('ACTION_REQUEST', 'RISK_ISSUE', 'DECISION')
ORDER BY
  CASE e.triage_importance WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END,
  CASE e.triage_category WHEN 'RISK_ISSUE' THEN 1 WHEN 'DECISION' THEN 2 ELSE 3 END;
```

The daily review itself might use a remote LLM to synthesise the results into a coherent briefing narrative, but the heavy lifting (processing 47 emails) was already done by the pipeline.

---

## Validation Against Real Test Data

The test data in `/Users/dev/penf-cli/TestData/` contains 269 real emails and 18 meeting transcripts. Before trusting any architectural design, we need to check it against what the pipeline will actually process.

### What the email data actually looks like

I parsed all 267 readable .eml files and extracted the plain text body (stripping MIME headers, HTML alternate parts, and base64-encoded attachments). Here's the distribution of **actual text content** the pipeline would process:

| Plain text size | Email count | Percentage | Cumulative |
|----------------|-------------|------------|------------|
| Under 2,000 chars | 129 | 48% | 48% |
| 2,000 - 5,000 chars | 66 | 25% | 73% |
| 5,000 - 10,000 chars | 27 | 10% | 83% |
| 10,000 - 20,000 chars | 31 | 12% | 95% |
| 20,000 - 50,000 chars | 13 | 5% | 100% |
| 50,000 - 70,000 chars | 1 | <1% | 100% |
| Over 100,000 chars | 0 | 0% | 100% |

**Key statistics:**
- Median: **2,036 characters** (about 300 words)
- Mean: **5,213 characters** (skewed by a few large ones)
- Maximum: **69,684 characters** (the CTG Status Report)
- No email exceeds 70K characters of plain text

### The file size is misleading

This is the most important finding. The raw .eml files are dramatically larger than the text content they contain:

| Email | Raw .eml size | Plain text content | Ratio |
|-------|--------------|-------------------|-------|
| TikTok FY26 discounts (largest) | 5.7 MB | 18,057 chars | 0.3% text |
| Huggingface Opportunity | 3.3 MB | 18,560 chars | 0.5% text |
| No IP ACLs for NLB | 1.4 MB | 6,377 chars | 0.4% text |
| CTG Status Report | 1.2 MB | 69,684 chars | 5.6% text |
| Billing thread | 493 KB | 3,079 chars | 0.6% text |
| Roadmap Concerns | 149 KB | 7,224 chars | 4.7% text |
| TikTok FY26 (shorter thread) | 319 KB | 21,575 chars | 6.6% text |

The massive file sizes come from:
- **Base64-encoded attachments** (Excel spreadsheets, PNG images) - accounts for most of the 5.7MB TikTok email
- **HTML duplicate of the plain text** with extensive CSS styling - the CTG report's HTML version is 1.14MB vs 73KB of plain text
- **MIME headers and boundaries** - typically 2-7KB per email
- **Quoted-printable encoding overhead** - adds ~10-15% to text size with `=20`, `=E2=80=A2` etc.

**Stage 0 (Parse) is doing most of the heavy lifting.** By stripping MIME structure, decoding quoted-printable, extracting only the plain text part, and discarding base64 attachments, we reduce the 5.7MB file to 18K of processable text.

### What this means for the pipeline design

**The current 8,000 character truncation is a problem, but a smaller one than expected.** After proper text extraction:

- **73% of emails (under 5K chars)** fit entirely in a single SLM call. No chunking needed. The 7B model handles these without any issue.
- **10% of emails (5-10K chars)** are borderline. A 7B model can process them but quality may degrade. A single SLM call is still reasonable.
- **12% of emails (10-20K chars)** need chunking for the SLM. These are the long thread emails where quoted replies haven't been stripped yet. After removing quoted replies, most of these would drop to 2-5K of new content.
- **5% of emails (20-50K chars)** definitely need chunking or the map-reduce approach.
- **1 email (the CTG report at 70K chars)** is the real outlier. This is a structured status report with 22 programme outcomes. It needs special handling.

**The thread-stripping in Stage 0 is more important than chunking.** The TikTok FY26 thread has 22 quoted replies in 18K characters of plain text. If we strip quoted replies and extract only the newest message, each individual email in that thread probably contains 500-2,000 characters of new content. That's trivial for any model.

### The CTG Status Report: the hardest case

This is the stress test for the pipeline. 69,684 characters of structured status report covering 22 programme outcomes, each with:
- Summary, schedule status, delay reasons
- Assigned person and programme manager
- Status summary, next milestone, success metrics
- Multiple bullet points of detail

**How the pipeline handles it:**

```
Stage 0: Extract plain text (69,684 chars from 1.2MB file)
         No thread stripping needed (single email, no replies)

Stage 1: Triage using first 500 chars
         -> PROJECT_UPDATE, HIGH importance
         (The subject "CTG Status Report" and sender "CTG-PMO-Ops" make this obvious)

Stage 2: This is too long for one SLM call.
         Split into chunks. But this report has natural structure -
         22 numbered programme outcomes. Better approach: split by
         programme outcome (detected by the numbered headers).
         Result: 22 chunks of ~2,000-4,000 chars each.
         Run extraction on each chunk independently.
         Each chunk yields: programme name, status, assignee, risks, milestones.

Stage 3: Resolve programme names against products/glossary.
         Resolve assignees against people table.
         "GTC Launch with NVIDIA" -> product entity
         "Shawn Michels" -> person entity
         "CSPI" -> glossary: "Compute Security Posture Improvement"

Stage 4: Gemini Pro receives:
         - 22 programme summaries with extracted status data
         - Resolved entities
         - Background context (previous CTG reports, known risks)
         Produces: trend analysis, programmes at risk, cross-cutting themes

Stage 5: Embed at programme-outcome level (22 embeddings)
         + embed overall summary
```

**The key insight:** This report has internal structure. Instead of blindly chunking at character boundaries, Stage 0 should detect the structure (numbered sections, repeating header patterns) and chunk semantically. The existing `splitIntoChunks()` with sentence-boundary detection is a reasonable fallback, but structure-aware splitting would produce better results.

### What the transcript data looks like

| Content type | File count | Size range | Character count |
|-------------|-----------|------------|-----------------|
| VTT transcripts | 4 | 51-64 KB | 51,265 - 64,144 chars |
| Text transcripts | 8 | 44-58 KB | 43,931 - 58,179 chars |
| Chat messages | 3 | 1.3-2.7 KB | 1,319 - 2,732 chars |

**Transcripts are consistent and manageable.** Every meeting transcript falls in the 44-64K character range (roughly 6,000-10,000 words). This aligns with the guide's estimate of 30,000-60,000 characters for a 1-hour meeting.

At ~50-60K characters (~12,000-15,000 tokens), a full transcript **does not fit** in the 7B model's effective working range (quality degrades well before the 32K token limit). The topic-segmentation approach from the guide is validated: split into 8-15 segments of 3,000-5,000 characters each, extract per segment, synthesise with the remote LLM.

**Chat messages are trivial.** At 1-3KB, these are well within a single SLM call. The guide's Slack pipeline (group by thread, triage, extract) applies, but these are small enough that even the grouping step is overkill - process the entire chat log in one call.

### Specific concerns from the test data

**1. Quoted reply accumulation in email threads.**

The TikTok FY26 discount thread has 10+ versions of the same email at different stages of the conversation (`[1]` through `[9]`). Each version includes all previous replies. If we ingest all versions, we're re-processing the same quoted text multiple times. Stage 0 needs to:
- Detect that `RE- Tiktok FY26 discounts[7].eml` is a later version of the same thread as `[1]` through `[6]`
- Either process only the latest version, or deduplicate content across versions using Message-ID and References headers

**2. The CTG report needs structure-aware chunking.**

Blind character-boundary chunking will split a programme outcome in the middle. The report has a clear repeating pattern (numbered sections with headers). Stage 0 should detect this and chunk by section. This is a general problem with structured reports and could be handled by:
- Regex detection of numbered section patterns
- Splitting on blank lines between sections
- Falling back to sentence-boundary chunking if no structure is detected

**3. VTT format overhead.**

A 64KB VTT file contains significant non-content overhead: entry numbers, timestamp ranges (`00:00:00.760 --> 00:00:01.280`), speaker ID numbers. After stripping VTT formatting, the actual spoken content is probably 60-70% of the file size (~38-45K chars). The guide's pipeline handles this in Stage 0 (parse and clean), but it's worth noting that VTT parsing is a prerequisite before any content size estimates.

**4. Speaker name variations in transcripts.**

The transcript files show names like `Sara Weisman (she/her)` and `Mark Holland (he,him)`. Stage 0 needs to normalise these to canonical names before Stage 2 extraction and Stage 3 entity resolution. Simple regex to strip parenthetical suffixes would handle this.

### Does the pipeline design hold up?

**Yes, with minor adjustments:**

| Design assumption | Reality from test data | Adjustment needed? |
|-------------------|----------------------|-------------------|
| Most emails are short | 73% under 5K chars | No - design is validated |
| Long threads need chunking | Threads are large files but small text after reply stripping | **Prioritise reply stripping over chunking** |
| Transcripts are 30-60K chars | Real data: 44-64K chars | No - estimate was accurate |
| Need to handle huge emails | Largest actual text: 70K chars | Add structure-aware chunking for reports |
| SLM handles most content in one call | 83% of emails under 10K chars | No - confirmed |
| Attachments are a concern | Base64 attachments dominate file size, not text | **Stage 0 must strip MIME properly; attachment text extraction is a separate concern** |

The pipeline design is sound for this data. The main additions needed:
1. **Quoted reply stripping** is more important than general chunking (handles most "large" emails)
2. **Structure-aware chunking** for formatted reports like the CTG status
3. **Thread deduplication** when multiple versions of the same thread are ingested
4. **MIME parsing in Stage 0** is the single most impactful step - it turns a 5.7MB file into 18K of processable text

---

## Worked Example: End-to-End with a Real Email

Let's trace the email from your `penf ai analyze em-Hp44vxPl` through the proposed pipeline.

### The email

An MTC 2026 product launch status update. Contains: risk register items, multiple named people with action items, project references (OSL, CLIC), location references (IAD), vulnerability mentions (VxLAN injection), and a list of open actions.

### Stage 0: Parse

- Source: Gmail
- Headers extracted: sender, recipients, date, subject, message-id
- HTML stripped to plain text
- No quoted replies detected (this is an original update, not a reply)
- Content length: ~4,000 characters (estimate based on analysis output)

### Stage 1: Triage

Input: subject line + first 500 chars
```json
{"category": "PROJECT_UPDATE", "importance": "HIGH", "reason": "MTC 2026 product launch status with active risks"}
```
Decision: proceed to Stage 2 + Stage 4 (Gemini Pro, because HIGH importance project update)

### Stage 2: Extract

Content fits in a single SLM call (under 6,000 chars). Extracted:
```json
{
  "people": [
    {"name": "Adam Weingarten", "role": null},
    {"name": "Allen Duet", "role": null},
    {"name": "Dan Spataro", "role": null},
    {"name": "James Brown", "role": null},
    {"name": "Aleksandra Lisinska-Pszyk", "role": null},
    {"name": "Michael Merideth", "role": null},
    {"name": "Melissa General", "role": null}
  ],
  "dates": [
    {"date": "January", "context": "VxLAN injection vuln resolution deadline"},
    {"date": "2026", "context": "MTC product launch year"}
  ],
  "projects": ["OSL", "CLIC", "MTC"],
  "organisations": ["SteerCo"],
  "action_items": [
    {"assignee": "Adam Weingarten and Allen Duet", "action": "Resolve scope issues for MTC 2026 Roadmap", "due": null},
    {"assignee": "James Brown", "action": "Raise CLIC Staffing for MTC in 2026 to SteerCo", "due": null},
    {"assignee": "Michael Merideth", "action": "Approve IAD Noisy Neighbor Risk mitigation plan", "due": "Before customer control plane integration start"},
    {"assignee": "Melissa General", "action": "Pull together tiger teams for missing SLOs/SLIs", "due": "Before OSL revenue start"}
  ],
  "decisions": [],
  "risks": [
    "Scope issues for MTC 2026 roadmap",
    "CLIC staffing denial",
    "IAD noisy neighbor risk",
    "Missing SLOs/SLIs for products in scope",
    "VxLAN injection vulnerability in CLIC PLT"
  ]
}
```

### Stage 3: Enrich

- "Adam Weingarten" -> person_id: p-aw001 (exact name match, confidence 0.95)
- "James Brown" -> person_id: p-jb001 (exact name match, confidence 0.98)
- "OSL" -> glossary: "Operational Service Layer", product_id: prod-osl
- "CLIC" -> glossary: "Cloud Infrastructure Compute", product_id: prod-clic
- "MTC" -> glossary: "Major Telecom Customer" (or whatever it stands for)
- "SteerCo" -> glossary: "Steering Committee"
- "SLOs/SLIs" -> glossary: "Service Level Objectives/Indicators"

Background context pulled:
- MTC product timeline: recent milestones, existing risks in database
- CLIC project status: recent meeting notes, team assignments
- OSL project status: revenue milestones, dependencies

### Stage 4: Gemini Pro Analysis

Receives the extracted + enriched data, plus background context. Produces:

- **Sentiment:** -0.30 (neutral-negative). "Status is nominally ON TRACK but five active risks with no resolution dates suggest significant underlying concern."
- **Topic mapping:** Links to MTC 2026 programme, CLIC product, OSL product. Identifies this as a SteerCo-level status update.
- **Risk assessment:** "VxLAN injection vulnerability is the most technically urgent risk (January deadline already passed?). Staffing denial for CLIC represents a strategic blocker."
- **Implicit action items:** "Someone needs to own the VxLAN fix - no assignee listed. The January deadline may have passed, requiring escalation."
- **Strategic insight:** "Five simultaneous risks on a revenue-critical programme suggest under-resourcing. The combination of scope issues + staffing denial + missing SLOs points to a programme that was launched before adequate support structures were in place."

Compare this to the current single-call output, which produced a competent but shallow analysis. The pipeline version has actual insight because it had context from the knowledge base to reason about.

### Stage 5: Embed and Index

- Embed full text (raw content vector)
- Embed summary (high-level search)
- Embed each risk separately (so "VxLAN risk" queries find this specific content)
- Embed action items (so "what does James Brown need to do?" finds these)
- Store all pipeline outputs in content_enrichment

### The result

The email is now:
- Searchable by keyword, semantic similarity, entity, and relationship
- Classified and importance-rated for daily review
- Entity-linked to people, products, and glossary terms
- Deep-analysed with sentiment, risk mapping, and strategic insights
- Traceable: every pipeline stage's output is stored and auditable via Langfuse

**Total cost:** 2 SLM calls (~5 seconds) + 1 Gemini Pro call (~$0.02-0.05) + 4 embedding calls (~2 seconds)

Compared to the current approach: 1 SLM call that truncates the content and produces a less insightful analysis.
