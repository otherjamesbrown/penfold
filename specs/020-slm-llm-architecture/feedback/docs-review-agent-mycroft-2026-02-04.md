# Architecture Review: SLM/LLM Split & Human-AI Collaboration

**Date:** 2026-02-04
**Reviewer:** Agent Mycroft
**Scope:** `specs/020-slm-llm-architecture`

## Executive Summary

The proposed architecture is **sound, pragmatic, and well-reasoned**. The shift from a monolithic "analyze everything with one prompt" approach to a multi-stage pipeline is the correct engineering decision for the constraints (Apple Silicon + single Linux server).

The "Radar Model" for Human-AI collaboration addresses the core weakness of most "chat with your data" systems: the lack of state and proactivity.

However, there are significant risks in **Stage 4.5 (The Feedback Loop)** and **Stage 0.5 (Topic Segmentation)** that require attention.

---

## 1. SLM/LLM Split Analysis

**Verdict:** Mostly Sound, with two specific risks.

The philosophy of "SLM for extraction/classification, LLM for reasoning" is the right approach.

### Risk 1: Topic Segmentation (Stage 0.5) on 7B
The design assumes a 7B model can reliably segment meeting transcripts by topic.
*   **The Problem:** Identifying topic boundaries often requires deep semantic understanding of *what* is being discussed, not just surface-level cues. A 7B model often hallucinates boundaries or misses subtle shifts.
*   **Recommendation:** Do not rely *solely* on the SLM for this. Implement a hybrid approach: use **TextTiling** or a similar lexical cohesion algorithm (fast, code-based) to propose candidate boundaries, then use the SLM to verify/label them. This is cheaper and more robust.

### Risk 2: Cross-Chunk Context in Extraction (Stage 2)
The map-reduce strategy for long content (chunking) risks losing context.
*   **The Problem:** If Chunk 1 introduces "The CLIC staffing issue" and Chunk 2 discusses "It is blocking us," an SLM analyzing Chunk 2 in isolation cannot extract "CLIC staffing" as the subject.
*   **Recommendation:** Increase the overlap window significantly (e.g., 500 chars) *or* pass a "running summary" context from the previous chunk to the next chunk's SLM call.

---

## 2. Human-AI Collaboration Model

**Verdict:** Practical and Strong.

The "Radar Model" is the strongest conceptual part of this design. Treating human signals (trust, watch lists) as first-class data is excellent.

### The "Trust Score" Friction
*   **Critique:** The design proposes a 0-5 numeric trust scale. Humans are notoriously bad at consistent numerical scoring of subjective feelings.
*   **Recommendation:** Simplify to a 3-state system for the MVP: **Trusted**, **Neutral**, **Untrusted**. This removes cognitive load. You can add "Domain Specific" tags (e.g., "Trusted for: Technical") as proposed, but keep the base weight simple.

### The Review Queue Bottleneck
*   **Critique:** Stage 3 flags "unknown entities" for review. If the glossary feedback loop is "aggressive" (as recommended), the user will be bombarded with review items for every new acronym or project code.
*   **Recommendation:** Implement **Auto-Dismiss Rules**. If an acronym appears only once and never again for 7 days, auto-dismiss it. Only surface items that appear >X times or are flagged by High Importance content.

---

## 3. Data Model & Golden Thread

**Verdict:** Viable on PostgreSQL.

The decision to avoid a graph database is correct. The `assertion_root_id` and `assertion_references` schema changes are sufficient to model the lifecycle.

### Critical Failure Point: Stage 4.5 Deduplication
The logic for deciding "Is this new Risk #312 or an update to Risk #101?" is the most fragile part of the system.
*   **The Risk:** If the LLM fails to match an update to an existing risk, you get duplicate risks. If it aggressively matches dissimilar risks, you lose data.
*   **Recommendation:**
    1.  **Conservative Matching:** If the LLM is not >90% confident it's a match, create a NEW risk but link it as a "Potential Duplicate" (new relation).
    2.  **Human Resolution:** Surface these "Potential Duplicates" in the Daily Review. "The AI thinks this new risk might be an update to VxLAN. Merge them?"
    3.  **Don't automate the merge** without high confidence or human confirmation.

---

## 4. Session Bootstrap

**Verdict:** Viable, but watch latency.

Loading context from the DB is the right pattern for stateless LLMs.

*   **Concern:** As the "Watch List" and "Recent Changes" grow, the `penf context morning` payload might exceed the context window or add significant latency to the first interaction.
*   **Mitigation:** Enforce strict token limits on the morning briefing. Summarize the summaries if necessary.

---

## 5. Missing Elements

1.  **The "Forgot" Function:** The design focuses on adding knowledge. How does knowledge expire? If a risk hasn't been mentioned in 6 months, does it stay in the context forever?
    *   *Recommendation:* Add an `archived` state for assertions/projects that auto-triggers based on inactivity, removing them from the default context window.

2.  **Conflict Resolution:** What happens when two trusted sources contradict each other? (e.g., VP says "Go", Principal Engineer says "No").
    *   *Recommendation:* The AI should explicitly surface the conflict in the briefing. "Conflict detected: Sarah says X, Mike says Y."

## Final Word

This is a high-quality architecture. It avoids over-engineering (no graph DB, no massive clusters) while addressing the real capabilities of the available hardware. The shift to a pipeline approach is necessary and well-designed.
