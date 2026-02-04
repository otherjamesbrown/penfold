# Implementation Mapping

> Reference document. For the core pipeline design, see `design.md`.

## What Already Exists

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

## What Needs to Be Built

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
| **Context morning command** | New `penf context morning` CLI command for session bootstrap (see `design.md` — How Claude Gets Its Memory). |
| **Watch list management** | CRUD operations for human-curated watch list with annotations. |
| **Session persistence** | `penf context session-end` for persisting session summaries. |
| **Seniority/trust fields** | Database migration to add seniority_tier, trust_level, trust_note, trust_domains to people table. |

## What Gets Modified

| Component | Change |
|-----------|--------|
| `buildAnalysisPrompt()` in `service.go` | Replace with pipeline orchestration. The prompt becomes the Stage 4 prompt only, receiving pre-processed input. |
| `truncateText(content, 8000)` | Remove. The pipeline handles content size at each stage. |
| `DefaultModelSelector` | Extend to consider content category and importance for model selection. |
| Content enrichment tables | Add columns for triage results, pipeline stage tracking. |

## Design Principles

**1. Each stage should be independently testable.** You should be able to run Stage 2 (extraction) on a piece of text and verify the output without running the full pipeline. This means each stage is its own gRPC method.

**2. Stages should be idempotent.** Running Stage 2 twice on the same content should produce the same result. This enables retries and reprocessing.

**3. Store intermediate results.** The output of each stage is stored in the database. If Stage 4 fails (Gemini is down), you don't lose the work from Stages 1-3. When Gemini comes back, you pick up from where you left off.

**4. The SLM should never block on the LLM.** Stages 1-3 (all local) can run immediately on ingestion. Stage 4 (remote) can be queued and processed asynchronously. The content is searchable (via keyword + embeddings) as soon as Stage 2 completes, before Stage 4 finishes.

**5. The pipeline configuration should be adjustable per tenant.** Some users might want everything analysed deeply. Others might want minimal processing. The triage thresholds and Stage 4 model selection should be configurable via `ai_routing_rules`.

**6. Fail loudly.** If the SLM produces invalid JSON, if entity resolution fails, if the LLM returns an error - log it, flag it for review, don't silently produce bad data. This aligns with Penfold's engineering principles: "fail loudly, succeed quietly."

## Frequently Asked Questions

### "Why not just use Gemini Flash for everything? It's cheap."

It is cheap per-call, but the costs add up at volume. More importantly, using a remote API for every triage and extraction call introduces latency and a dependency on network availability. If Google's API is slow or down, your entire ingestion pipeline stops. With local SLMs handling the first stages, ingestion continues regardless of remote API availability. The remote LLM is only needed for the final analysis step, which can be queued and retried.

### "What if the SLM gets the triage wrong?"

It will sometimes. The most likely error is classifying something as lower importance than it deserves. Mitigations:

- **Keyword triggers:** Regardless of SLM triage, if the content contains known high-priority keywords (specific project names, risk-related terms from the glossary), override the importance to HIGH.
- **Sender-based rules:** Emails from certain senders (your manager, key customers, specific distribution lists) always get full processing regardless of triage.
- **Periodic review:** Sample triage results and check for systematic errors.
- **User correction:** If a user searches for something and the content wasn't properly analysed, flag it for reprocessing through the full pipeline.

### "Can the SLM handle non-English content?"

The Qwen 2.5 7B model has reasonable multilingual support, particularly for European languages and Chinese. For triage and basic extraction, it should work adequately. For deep analysis in non-English languages, route to Gemini Pro which has stronger multilingual capabilities. The triage stage could include language detection and use that as a routing signal.

### "How do we handle attachments (PDFs, spreadsheets)?"

Attachments need a separate pre-processing step before entering the pipeline:

1. **PDFs:** Extract text using a PDF library (not an LLM). Then the extracted text enters the pipeline at Stage 1 like any other content.
2. **Spreadsheets:** Extract cell data, convert to a text representation.
3. **Images:** These need a multimodal model (Gemini Pro Vision). Route directly to Stage 4.
4. **Large documents (50+ pages):** Chunk at the page or section level, process each chunk through Stages 2-3, then synthesise in Stage 4.

### "What about privacy? Are we sending sensitive content to Google?"

Only content that reaches Stage 4 goes to a remote API. The pipeline's triage stage (Stage 1) means most content stays entirely local. For content that does reach Stage 4:

- All Penfold data stays within your control up to Stage 3.
- Stage 4 sends pre-processed extracts and summaries, not always the raw content (depending on configuration).
- Gemini API data is not used for training by default under Google's API ToS.
- For highly sensitive content, you could add a sensitivity classification in Stage 1 and route sensitive content to a more capable local model rather than the cloud.

### "What happens when we upgrade to a larger local model?"

The pipeline design is model-agnostic. If you later run a 14B or 32B model locally:

- Stage 2 extraction quality improves, especially for long or complex content.
- Some content that currently requires Stage 4 (Gemini) could be handled locally.
- The triage thresholds in the routing rules adjust: more categories get processed locally.
- The pipeline structure doesn't change - just the routing decisions.

This is why the `ai_routing_rules` table and `ModelSelector` exist. You change the routing, not the pipeline.
