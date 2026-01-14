# Meeting Pipeline User Guide

**Version**: 1.0.0
**Target Audience**: Business users, knowledge workers
**Estimated Reading Time**: 10 minutes

## Overview

This guide walks you through uploading meetings, monitoring processing, reviewing results, and searching your meeting content.

## Getting Started

### Uploading Your First Meeting

1. **Access the Upload Interface**
   - Navigate to `/meetings/upload` in your browser
   - You'll see the meeting upload dashboard

2. **Choose Your Files**
   - Click "Select Files" or drag and drop
   - Supported formats: MP4, MP3, WAV, MOV, PDF, DOCX
   - Maximum size: 2GB per file
   - Multiple files can be uploaded at once

3. **Provide Meeting Context** (Optional but Recommended)
   - **Meeting Title**: Descriptive name for easy identification
   - **Date & Time**: When the meeting occurred (if different from upload time)
   - **Participants**: Known attendees (helps with speaker identification)
   - **Project Context**: Related project or initiative
   - **Privacy Level**: Public, Internal, Confidential, or Restricted

4. **Start Upload**
   - Click "Upload and Process"
   - You'll receive a tracking ID for monitoring progress
   - Large files show progress bar and estimated time remaining

### Monitoring Processing Progress

**Processing Phases Overview**:
1. **File Validation** (30 seconds) - Format and quality checks
2. **Transcription** (15-25 minutes) - Speech-to-text conversion
3. **Speaker Identification** (2-5 minutes) - Who said what
4. **Content Analysis** (3-8 minutes) - Summaries and insights
5. **Search Indexing** (1-2 minutes) - Making content searchable
6. **Quality Review** (Manual) - Optional human verification

**Monitoring Your Upload**:
- **Dashboard View**: See all your meetings and their processing status
- **Real-time Updates**: Status updates every 30 seconds
- **Notifications**: Email alerts when processing completes or requires attention
- **Detailed Progress**: Click any meeting to see detailed phase progress

**Status Indicators**:
- 🔄 **Processing**: Currently being worked on
- ✅ **Complete**: Ready for use
- ⚠️ **Needs Review**: AI confidence low, manual review recommended
- ❌ **Failed**: Processing error, support needed
- 📊 **Analyzing**: Content analysis in progress

## Working with Processed Meetings

### Viewing Meeting Results

1. **Access Your Meetings**
   - Go to `/meetings` to see all processed meetings
   - Filter by date, project, participants, or status

2. **Meeting Overview Page**
   - **Transcript**: Full text with timestamps and speaker labels
   - **Summary**: AI-generated overview with key points
   - **Action Items**: Extracted tasks and commitments
   - **Decisions**: Key decisions and their context
   - **Participants**: Identified speakers with contribution stats
   - **Topics**: Main themes and discussion areas

3. **Quality Indicators**
   - **Transcription Confidence**: 0-100% accuracy estimate
   - **Speaker ID Confidence**: How certain speaker identification is
   - **Content Quality**: Overall AI confidence in analysis

### Understanding AI Confidence Scores

**High Confidence (85-100%)**:
- ✅ Green indicators throughout
- Results likely very accurate
- Can be used immediately

**Medium Confidence (70-84%)**:
- 🟡 Yellow indicators in some areas
- Generally good quality
- Review recommended for important meetings

**Low Confidence (Below 70%)**:
- 🔴 Red indicators present
- Manual review strongly recommended
- Results may need significant correction

## Manual Review and Corrections

### When to Review

**Always Review For**:
- Important business decisions
- Legal or compliance matters
- Low AI confidence scores
- Meetings with poor audio quality

**Consider Reviewing For**:
- Meetings with new or unfamiliar speakers
- Technical discussions with specialized terminology
- Meetings with background noise or interruptions

### Review Workflows

#### 1. Transcript Review

**Access**: Click "Review Transcript" on meeting page

**Common Corrections**:
- Fix misheard words or phrases
- Correct technical terms or names
- Adjust punctuation for clarity
- Split or merge speaker segments

**How to Edit**:
- Click any text segment to edit inline
- Use speaker dropdown to reassign segments
- Add timestamps for better accuracy
- Save changes incrementally

#### 2. Speaker Identification Review

**Access**: Click "Review Speakers" on meeting page

**Verification Process**:
- Confirm or correct speaker names
- Merge duplicate speaker identities
- Link speakers to known contacts
- Add pronunciation guides for future meetings

**Speaker Confidence Indicators**:
- **Green**: High confidence, likely correct
- **Yellow**: Medium confidence, worth checking
- **Red**: Low confidence, needs attention

#### 3. Entity Resolution Review

**Access**: Navigate to "Entity Resolution" queue

**Review Tasks**:
- Confirm participant identities
- Link mentions to known projects
- Resolve topic categories
- Validate action item assignments

### Version Control

**Every Edit Creates a Version**:
- Original transcript preserved
- All changes tracked with timestamps
- Full edit history available
- Rollback to any previous version

**Version History Features**:
- See what changed and when
- Compare any two versions
- View edit attribution
- Restore previous versions

## Searching and Discovery

### Basic Search

**Quick Search**:
- Use the search bar at the top of any page
- Searches across transcripts, summaries, and participants
- Results ranked by relevance and confidence

**Search Tips**:
- Use quotes for exact phrases: `"action items"`
- Include participant names: `John discussed budget`
- Try topic keywords: `marketing campaign Q4`

### Advanced Search

**Filters Available**:
- **Date Range**: Specific time periods
- **Participants**: Meetings with specific people
- **Projects**: Related to particular initiatives
- **Confidence Level**: Only high-quality results
- **Content Type**: Decisions, action items, discussions

**Semantic Search Features**:
- Find concepts, not just keywords
- Search example: "budget concerns" finds discussions about costs, expenses, financial worries
- Context-aware results show related meetings

### Search Result Types

**Text Matches**:
- Exact transcript segments with timestamps
- Click to jump directly to that moment
- Speaker attribution included

**Summary Matches**:
- AI-generated insight matches
- Key points and conclusions
- Action items and decisions

**Contextual Matches**:
- Related meetings and topics
- Participant connection networks
- Project timeline correlations

## Best Practices

### For Better Transcription Quality

**Before Upload**:
- Use good quality recording equipment
- Minimize background noise
- Encourage clear speech
- Record in quiet environments

**Meeting Context**:
- Provide participant names in advance
- Include project context
- Set appropriate privacy levels
- Use descriptive meeting titles

### For Effective Search

**Organize Meetings**:
- Use consistent naming conventions
- Tag meetings with project names
- Include all relevant participants
- Set privacy levels appropriately

**Search Strategies**:
- Start broad, then narrow with filters
- Use participant names for focused searches
- Combine keywords with date ranges
- Review confidence scores before using results

### Privacy and Security

**Privacy Levels**:
- **Public**: Accessible to all users
- **Internal**: Organization members only
- **Confidential**: Specific team/project access
- **Restricted**: Named individuals only

**Data Protection**:
- All files encrypted at rest
- Access logged for audit
- Retention policies enforced
- Deletion capabilities available

**Best Practices**:
- Set appropriate privacy levels
- Review access permissions regularly
- Don't upload personal conversations
- Follow organizational data policies

## Troubleshooting Common Issues

### Upload Problems

**Large File Upload Fails**:
- Check internet connection stability
- Try uploading during off-peak hours
- Split large meetings into segments
- Ensure file format is supported

**Processing Takes Too Long**:
- 2-hour meeting = ~30 minutes processing time
- Poor audio quality increases processing time
- Multiple concurrent uploads may slow processing
- Check system status dashboard

### Quality Issues

**Poor Transcription Quality**:
- Review audio quality of original recording
- Check for background noise or multiple speakers
- Consider manual review and correction
- Provide feedback to improve AI accuracy

**Speaker Identification Problems**:
- Add known participant names before upload
- Use speaker review interface to make corrections
- Train system by confirming correct identifications
- Consider voice signature enrollment for frequent speakers

### Search Problems

**Can't Find Expected Content**:
- Try broader search terms
- Check date range filters
- Verify privacy level access
- Use semantic search for concepts

**Low Confidence Results**:
- Filter by confidence level
- Review original transcript
- Try alternative search terms
- Consider manual transcript review

## Getting Help

### Self-Service Resources

- **Dashboard Help**: Tooltips and guided tours available
- **Video Tutorials**: Step-by-step demonstrations
- **FAQ**: Common questions and answers
- **Community Forum**: User discussions and tips

### Support Channels

**In-App Support**:
- Feedback button on every page
- Report quality issues directly
- Request feature improvements
- Submit bug reports

**Contact Support**:
- Technical issues: IT help desk
- Processing problems: Submit support ticket
- Feature requests: Product feedback form
- Training questions: User training resources

---

*This user guide provides comprehensive coverage of the Meeting Pipeline system for business users. For technical documentation, see the Administrator Guide and API Reference.*