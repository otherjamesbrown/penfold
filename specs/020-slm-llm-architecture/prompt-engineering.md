# Prompt Engineering and Output Validation

> Reference document. For the core pipeline design, see `design.md`.

SLMs and LLMs respond differently to prompt structure. Getting this right is the difference between a pipeline that works and one that produces garbage.

## Rules for SLM Prompts

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

## Rules for LLM Prompts (Stage 4)

**1. Provide context, not just content.** The LLM's value is in reasoning, so give it material to reason about. The Stage 3 enrichment output (resolved entities, glossary expansions, relevant background) is as important as the content itself.

**2. Separate facts from analysis.** Tell the LLM what's already been extracted (Stage 2 output) and ask it to focus on what requires reasoning. "The following entities have already been extracted and verified. Focus your analysis on: connections between entities, implied actions, and strategic significance."

**3. Be specific about what "analysis" means.** "Analyse this email" is vague. "Identify how this email relates to the three active risks listed in the background context" is specific. The LLM produces better output when the reasoning task is clearly scoped.

**4. Ask it to show its reasoning.** For risk mapping and sentiment analysis, ask the LLM to explain why. "Score: -0.3, because the phrase 'areas we need to watch' in a business context typically indicates known problems being downplayed." This makes the output auditable and helps you judge quality.

## Example: Triage Prompt for a 7B Model

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

## Validating SLM Output Quality

Running a local 7B model means you're responsible for quality in a way you're not when using a managed API. You need to know when it's working and when it's not.

### Structured Output Validation

Every SLM call should validate the output before accepting it:

1. **JSON parsing.** If the model was asked for JSON and the response doesn't parse, it's a failure. Don't try to fix malformed JSON - retry the call (up to 2 retries), then flag for review if it still fails.

2. **Schema validation.** The JSON parsed, but does it have the expected fields? Is `category` one of the allowed values? Is `importance` one of HIGH/MEDIUM/LOW? Reject outputs that don't conform.

3. **Sanity checks.** An email with subject "URGENT: Production down" classified as PERSONAL/LOW is probably wrong. Simple rule-based checks can catch the most obvious SLM errors:
   - Subject contains "urgent", "critical", "production", "outage" -> importance must be HIGH
   - Sender is in the VIP list -> importance cannot be LOW
   - Content contains known project keywords -> category cannot be PERSONAL

4. **Confidence heuristic.** If the SLM's JSON reason is very short or generic ("general email"), treat the classification as low-confidence and route to Stage 2 regardless.

### Tracking Quality Over Time

Log every triage and extraction result to Langfuse (already integrated). Periodically review a sample:

- What percentage of emails did the SLM triage as PERSONAL that were actually project-related?
- What percentage of extracted entities were successfully resolved in Stage 3? (Low resolution rate might mean extraction quality is poor.)
- What percentage of Stage 4 analyses contradicted Stage 1 triage? (If the LLM frequently upgrades importance, the SLM threshold is too conservative.)

### When to Distrust the SLM

Be especially cautious with:

- **Forwarded emails.** The outer email might be "FYI" from a colleague, but the forwarded content is a critical customer escalation. The subject line and first paragraph may not reflect the actual content.
- **Newsletter-style emails.** Long emails with multiple topics. Triage based on the first 500 chars might miss important content further down.
- **Emails with minimal text.** "See attached" or "Thoughts?" with an attachment. The real content is in the attachment, not the email body. Flag these for attachment processing.

For these cases, add heuristic rules that override or supplement the SLM triage:
- If email is a forward (detected in Stage 0 from headers), always run Stage 2 on the forwarded content separately.
- If email body is under 100 characters and has attachments, classify as needs-attachment-review.
- If email is from a known distribution list for a tracked project, always route to Stage 2 minimum.
