# Appendix: Test Email Analysis

Part of [Content Enrichment Pipeline](spec.md)

---

## Test Data Overview

Complete analysis of `/penfold_test_data/email-small/` (14 files).

---

## Classification Distribution

| Classification | Count | Percentage |
|----------------|-------|------------|
| `email/thread` | 10 | 71% |
| `email/forward` | 2 | 14% |
| `calendar/cancellation` | 1 | 7% |
| `notification/jira` | 1 | 7% |

---

## Detailed Analysis

### Meeting Cancellation

```
File: Canceled- MTC Leader Standup[4].eml
From: Weisman, Sara <sweisman@akamai.com>
To: 10 recipients (all @akamai.com)
Classification: calendar/cancellation
Exchange Header: X-MS-Exchange-Organization-AuthAs: Internal
Extracted URLs: WebEx meeting links
Key extraction: attendee list, WebEx meeting details, recurring series
```

### Jira Notification

```
File: [TRACK-JIRA] Updates for OUT-697- Launch new products...eml
From: TRACK JIRA <gsd-jira@akamai.com>
To: 1 recipient
Classification: notification/jira
Headers: Auto-Submitted: auto-generated, Precedence: bulk
Exchange Header: X-MS-Exchange-Organization-AuthAs: Anonymous
Attachments: 2
Key extraction: OUT-697 ticket ID
```

### Email Forwards

```
File: FW- BLT MTC discussion .eml
From: Weisman, Sara <sweisman@akamai.com>
Classification: email/forward
Has Reply-To: true
Attachments: 1

File: FW- PACE Technical Readiness Review Jan 6.eml
From: Prb-Facilitator <Prb-Facilitator@akamai.com>
Classification: email/forward
False positive entities: PM-2, UTC-05, PACE-2026 (not Jira tickets)
```

### Email Threads (TikTok Discussion)

```
Files: RE- Tiktok FY26 discounts.eml (5 messages in thread)
Participants: Rick Eskelsen, Sabina Sawyer, Hrishikesh Varma
Classification: email/thread
All internal (@akamai.com)
Has Reply-To: true (all)
Attachments: 0-13 per message
```

### External Notification

```
File: TT MTC MVP - LC T... - @jabrown@akamai.com...eml
From: Shobana Shankar (Google Slides) <comments-noreply@docs.google.com>
Classification: email/thread (should be notification/google)
Exchange Header: X-MS-Exchange-Organization-AuthAs: Anonymous
Cross-domain: docs.google.com → akamai.com
Note: @mention notification from Google Slides
```

---

## Entity Extraction Results

### False Positive Jira Patterns Observed

- `PM-2` - Appears to be time reference
- `UTC-05` - Timezone reference
- `PACE-2026` - Project year reference
- `Y-8` - Unknown abbreviation

### Valid Jira Pattern

- `OUT-697` - Actual Jira ticket (from Jira notification sender)

### URL Domains Found

- `akamai.webex.com` - Meeting links
- `clpe90m2ll.feishu.cn` - External document links
- Internal URLs in body text

---

## Unique People Observed

| Name | Email | Role in Test Data |
|------|-------|-------------------|
| Sara Weisman | sweisman@akamai.com | Meeting organizer, forwarder |
| Rick Eskelsen | reskelse@akamai.com | Thread participant |
| Sabina Sawyer | ssawyer@akamai.com | Thread participant |
| Hrishikesh Varma | hvarma@akamai.com | Thread participant |
| Aditya Netalkar | adrajend@akamai.com | Thread participant |
| Nate Ye | nye@akamai.com | Thread participant |
| Prasanna Laghate | plaghate@akamai.com | Thread participant |
| TRACK JIRA | gsd-jira@akamai.com | System account |
| Prb-Facilitator | Prb-Facilitator@akamai.com | System/role account |
| Shobana Shankar | comments-noreply@docs.google.com | External notification |

---

## Observations for Implementation

1. **Thread identification is reliable** - In-Reply-To/References headers work well
2. **Exchange headers valuable** - `X-MS-Exchange-Organization-AuthAs` reliably identifies internal vs external
3. **Jira detection needs sender check** - Subject pattern alone has too many false positives
4. **Google notifications need category** - External sender but references internal users
5. **System accounts need flagging** - `Prb-Facilitator`, `gsd-jira` are not people
6. **Attachment counts vary widely** - 0-13 per message, useful for search filtering

---

## User Scenarios Testing

### User Story 1 - Entity Resolution

**Test**: Ingest emails where same person appears with different identifiers.

```
Input:
  - Email 1: From "Rick Eskelsen <reskelse@akamai.com>"
  - Email 2: From "Eskelsen, Rick <reskelse@akamai.com>"
  - Email 3: To "reskelse@akamai.com" (no display name)

Expected:
  - All resolve to same person_id
  - Display name variations stored as aliases
  - Canonical name: "Rick Eskelsen"
```

### User Story 2 - Smart Classification

**Test**: Verify classification for each email type.

```
Input: Mixed batch of emails
Expected:
  - Jira notification → notification/jira, metadata_only
  - Meeting cancellation → calendar/cancellation, state_tracking
  - Thread reply → email/thread, full_ai
  - Forward → email/forward, full_ai
  - Google comment → notification/google, metadata_only
```

### User Story 6 - Jira Ticket Tracking

**Test**: Extract Jira state changes.

```
Input: TRACK-JIRA notification for OUT-697
Expected:
  - jira_tickets record created/updated
  - jira_ticket_changes shows: Open → In Progress
  - Source email linked to ticket
  - No embedding generated
```

---

## Edge Cases from Test Data

| Edge Case | File | Handling |
|-----------|------|----------|
| 10 recipients on cancellation | Canceled- MTC Leader Standup | Resolve all 10 to people |
| Forward with role sender | FW- PACE Technical Readiness | Flag Prb-Facilitator as role_account |
| Google notification external | TT MTC MVP | Classify as notification/google |
| False Jira pattern | FW- PACE Technical | Don't extract PM-2, UTC-05, PACE-2026 |
| Thread with 13 attachments | RE- Tiktok FY26 discounts | Process attachments, link to source |
| Bot sender with bulk header | TRACK-JIRA | Auto-Submitted + Precedence: bulk → bot |
