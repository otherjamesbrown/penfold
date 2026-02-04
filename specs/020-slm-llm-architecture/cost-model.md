# Cost Model and Performance Expectations

> Reference document. For the core pipeline design, see `design.md`.

## Per-Email Cost Breakdown

| Content type | SLM calls | Remote LLM calls | Approximate cost |
|-------------|-----------|-------------------|-----------------|
| Personal/social | 1 (triage) | 0 | Free |
| Internal comms (low) | 1 (triage) | 0 | Free |
| Internal comms (action) | 2 (triage + extract) | 0 | Free |
| Project update (medium) | 2 + embeddings | 1 Flash | ~$0.001-0.005 |
| Project update (high) | 2 + embeddings | 1 Pro | ~$0.01-0.05 |
| Risk/escalation | 2 + embeddings | 1 Pro | ~$0.01-0.05 |
| Long thread (high) | 2 + N extractions + embeddings | 1 Pro | ~$0.02-0.10 |

## Batch Processing of 100 Emails

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

## SLM Throughput on Apple Silicon

On the M4 Mac Mini (32GB) running a 7B 4-bit quantised model:

- Triage call (~500 chars input, ~50 chars output): ~1-3 seconds
- Extraction call (~3,000 chars input, ~200 chars output): ~3-8 seconds
- Embedding generation: ~0.5-1 second per text

For 100 emails with ~200 SLM calls, that's roughly 5-15 minutes of SLM processing. This can be parallelised across multiple requests since the MLX server handles concurrent connections (up to memory limits).

## Cost Comparison: Current vs Proposed

| Approach | Cost per 100 emails | Cost per 100 transcripts |
|----------|-------------------|------------------------|
| Current (single LLM call, truncated) | Free (local 7B) but low quality | Free but 85-90% content lost |
| Everything to Gemini Pro | $3-10 | $6-75 |
| Proposed pipeline (SLM + selective LLM) | $0.35-1.60 | $1-15 |

The proposed pipeline is 5-10x cheaper than sending everything to a remote LLM, while producing better results because it processes the full content (no truncation) and provides entity context.
