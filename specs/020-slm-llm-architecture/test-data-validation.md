# Validation Against Real Test Data

> Reference document. For the core pipeline design, see `design.md`.

The test data in `/Users/dev/penf-cli/TestData/` contains 269 real emails and 18 meeting transcripts. Before trusting any architectural design, we need to check it against what the pipeline will actually process.

## Email Data

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

## The File Size Is Misleading

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

## What This Means for the Pipeline Design

**The current 8,000 character truncation is a problem, but a smaller one than expected.** After proper text extraction:

- **73% of emails (under 5K chars)** fit entirely in a single SLM call. No chunking needed. The 7B model handles these without any issue.
- **10% of emails (5-10K chars)** are borderline. A 7B model can process them but quality may degrade. A single SLM call is still reasonable.
- **12% of emails (10-20K chars)** need chunking for the SLM. These are the long thread emails where quoted replies haven't been stripped yet. After removing quoted replies, most of these would drop to 2-5K of new content.
- **5% of emails (20-50K chars)** definitely need chunking or the map-reduce approach.
- **1 email (the CTG report at 70K chars)** is the real outlier. This is a structured status report with 22 programme outcomes. It needs special handling.

**The thread-stripping in Stage 0 is more important than chunking.** The TikTok FY26 thread has 22 quoted replies in 18K characters of plain text. If we strip quoted replies and extract only the newest message, each individual email in that thread probably contains 500-2,000 characters of new content. That's trivial for any model.

## The CTG Status Report: The Hardest Case

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

**The key insight:** This report has internal structure. Instead of blindly chunking at character boundaries, Stage 0 should detect the structure (numbered sections, repeating header patterns) and chunk semantically.

## Transcript Data

| Content type | File count | Size range | Character count |
|-------------|-----------|------------|-----------------|
| VTT transcripts | 4 | 51-64 KB | 51,265 - 64,144 chars |
| Text transcripts | 8 | 44-58 KB | 43,931 - 58,179 chars |
| Chat messages | 3 | 1.3-2.7 KB | 1,319 - 2,732 chars |

**Transcripts are consistent and manageable.** Every meeting transcript falls in the 44-64K character range (roughly 6,000-10,000 words). This aligns with the design's estimate of 30,000-60,000 characters for a 1-hour meeting.

At ~50-60K characters (~12,000-15,000 tokens), a full transcript **does not fit** in the 7B model's effective working range (quality degrades well before the 32K token limit). The topic-segmentation approach is validated: split into 8-15 segments of 3,000-5,000 characters each, extract per segment, synthesise with the remote LLM.

**Chat messages are trivial.** At 1-3KB, these are well within a single SLM call.

## Specific Concerns

**1. Quoted reply accumulation in email threads.**
The TikTok FY26 discount thread has 10+ versions of the same email at different stages of the conversation. Each version includes all previous replies. If we ingest all versions, we're re-processing the same quoted text multiple times. Stage 0 needs to deduplicate content across versions using Message-ID and References headers.

**2. The CTG report needs structure-aware chunking.**
Blind character-boundary chunking will split a programme outcome in the middle. The report has a clear repeating pattern (numbered sections with headers). Stage 0 should detect this and chunk by section.

**3. VTT format overhead.**
A 64KB VTT file contains significant non-content overhead: entry numbers, timestamp ranges. After stripping VTT formatting, the actual spoken content is probably 60-70% of the file size (~38-45K chars).

**4. Speaker name variations in transcripts.**
The transcript files show names like `Sara Weisman (she/her)` and `Mark Holland (he,him)`. Stage 0 needs to normalise these to canonical names before entity resolution.

## Does the Pipeline Design Hold Up?

**Yes, with minor adjustments:**

| Design assumption | Reality from test data | Adjustment needed? |
|-------------------|----------------------|-------------------|
| Most emails are short | 73% under 5K chars | No - design is validated |
| Long threads need chunking | Threads are large files but small text after reply stripping | **Prioritise reply stripping over chunking** |
| Transcripts are 30-60K chars | Real data: 44-64K chars | No - estimate was accurate |
| Need to handle huge emails | Largest actual text: 70K chars | Add structure-aware chunking for reports |
| SLM handles most content in one call | 83% of emails under 10K chars | No - confirmed |
| Attachments are a concern | Base64 attachments dominate file size, not text | **Stage 0 must strip MIME properly; attachment text extraction is a separate concern** |

The main additions needed:
1. **Quoted reply stripping** is more important than general chunking (handles most "large" emails)
2. **Structure-aware chunking** for formatted reports like the CTG status
3. **Thread deduplication** when multiple versions of the same thread are ingested
4. **MIME parsing in Stage 0** is the single most impactful step - it turns a 5.7MB file into 18K of processable text
