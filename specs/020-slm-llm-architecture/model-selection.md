# Model Selection: 7B vs 14B vs 32B on Apple Silicon

> Reference document. For the core pipeline design, see `design.md`.

The Mac Mini (M4, 32GB unified memory) can run either a 7B or 32B quantised model, but not both simultaneously, and the tradeoffs are significant. This isn't just a speed question - it's a question about what's possible at each pipeline stage.

## The Hardware Reality

| Model | Quantisation | Memory footprint | Tokens/sec (M4) | Tokens/sec (M4 Pro) | Context window |
|-------|-------------|-----------------|-----------------|---------------------|----------------|
| Qwen 2.5 7B | 4-bit (Q4_K_M) | ~5 GB | ~40-60 tok/s | ~60-80 tok/s | 32K tokens |
| Qwen 2.5 14B | 4-bit (Q4_K_M) | ~9 GB | ~20-35 tok/s | ~35-50 tok/s | 32K tokens |
| Qwen 2.5 32B | 4-bit (Q4_K_M) | ~19 GB | ~8-15 tok/s | ~15-25 tok/s | 32K tokens |
| Qwen 2.5 32B | 3-bit (Q3_K_M) | ~15 GB | ~10-18 tok/s | ~18-30 tok/s | 32K tokens |

With 32GB unified memory, the 32B 4-bit model fits (~19GB) with reasonable headroom (~13GB for system, MLX server, and other services). But you're still 3-5x slower per token than the 7B, and running the 32B leaves less memory for concurrent operations (Worker service, Temporal client, database connections).

## What the Speed Difference Actually Means

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

## Where the 32B Is Actually Worth It

The 32B model isn't uniformly better. It's specifically better at tasks where the 7B falls down:

**Complex extraction from ambiguous text.** When an email says "Mike mentioned the issue to Dan's team last week and they're looking into it", a 7B model might extract Mike and Dan as people but miss that "Dan's team" is a team reference, or that "the issue" refers to something specific. A 32B model is better at this kind of coreference resolution.

**Triage of forwarded or multi-topic emails.** An email that looks like internal comms but contains a forwarded customer escalation. The 7B model reads the first 500 chars (the forwarding note) and classifies as INTERNAL_COMMS/LOW. The 32B model is more likely to look at the forwarded content and classify correctly.

**Extraction from noisy transcripts.** Auto-transcription produces garbled text. "We need to, um, you know, get the, the VxLAN thing sorted before, what was it, January?" A 32B model handles this better because it can resolve the disfluencies.

**Structured output reliability.** The 32B model produces valid JSON more consistently. On a 7B model, you might see 5-10% malformed JSON responses. On a 32B, it's closer to 1-2%.

## Where the 7B Is Sufficient (or Better)

**Triage of clear-cut content.** "Mandatory compliance training" from hr@company.com doesn't need a 32B model to classify as INTERNAL_COMMS/LOW. The subject line says it all.

**Extraction from clean, well-structured text.** Business emails that clearly state "Action: Dan to complete the review by Friday" are easy extraction targets for any model.

**Embeddings.** The embedding model (mxbai-embed-large-v1) is separate from the LLM and runs fine on its own. Model size for the LLM doesn't affect embedding quality.

**Any task where speed matters more than marginal quality improvement.** If you're processing 300 Slack messages, the 3-5x speed penalty of the 32B model isn't worth a small improvement in extraction accuracy.

## Recommended Strategies

### Strategy 1: Time-Based Model Swapping

The MLX server can't load two models simultaneously (memory constraint), but it can swap models between tasks. Model loading on MLX takes 5-15 seconds depending on model size.

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

### Strategy 2: Complexity-Based Routing (Single Model Loaded)

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

### Strategy 3: 7B for Everything, Remote LLM for Quality

The most pragmatic approach if you want operational simplicity: accept the 7B's limitations and use the remote LLM (Gemini Flash, which is cheap and fast) to cover the gap.

```
Stage 1: Triage with 7B (fast, good enough for classification)
Stage 2: Extract with 7B (fast, handles most content)
  -> If content is complex/long/noisy: send to Gemini Flash for extraction
     (Flash is cheap enough that this isn't a cost concern)
Stage 3: Enrich (code, no model)
Stage 4: Gemini Pro/Flash (already remote)
```

This eliminates the 32B model entirely. The cost increase from sending some extraction to Gemini Flash is marginal (~$0.001-0.003 per extraction call). The operational simplicity of running one local model is significant.

## The Honest Recommendation

**With the M4 32GB Mac Mini (current hardware):** The 32B model fits but leaves ~13GB for everything else. Strategy 1 (time-based swapping) or Strategy 2 (7B default with 32B for hard cases) are both viable. Strategy 3 (7B + Gemini Flash fallback) is simpler operationally and avoids the memory pressure of the 32B entirely. The pipeline is designed so that SLM quality at Stages 1-2 doesn't need to be perfect; Stage 4 (remote LLM) is where the real intelligence happens.

**If you later upgrade to an M4 Pro/Max with 48-64GB:** You could keep both models loaded simultaneously (MLX supports this with model caching). Route triage to 7B and extraction to 32B without swapping. This is the ideal setup.

## The 14B Middle Ground

The Qwen 2.5 14B is an interesting middle ground:
- Fits comfortably on 32GB (9GB model + plenty of headroom)
- ~2x slower than 7B (not 5x like the 32B)
- Meaningfully better at structured extraction than 7B
- Still not as good at complex reasoning as 32B

For the pipeline, the 14B might be the sweet spot: good enough extraction quality that you rarely need to fall back to remote LLM, fast enough that batch processing completes in reasonable time. Worth benchmarking against the 7B on your actual content to see if the quality improvement justifies the 2x speed penalty.

## How to Decide: Benchmark on Your Data

The right choice depends on your actual content. Run a benchmark:

1. Take 50 representative emails (mix of types from your inbox).
2. Run Stage 1 triage on all 50 with both 7B and 32B. Compare classifications.
3. Run Stage 2 extraction on 20 non-trivial emails with both. Compare extracted entities against a manually-created ground truth.
4. Measure: how many extractions does the 32B get right that the 7B gets wrong?
5. Multiply by the speed difference. Is the quality improvement worth 3-5x slower processing?

If the 7B gets triage right 95% of the time and the 32B gets it right 98%, the 3% improvement probably isn't worth 5x slower processing. If the 7B gets extraction right 70% and the 32B gets 90%, that's a different calculation.
