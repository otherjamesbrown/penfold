# Asset Versioning and Re-Analysis Architecture

## Design Philosophy
**Raw assets as long-term investment**: Store all original content permanently and maintain complete analysis history to enable re-processing as AI capabilities evolve.

## Core Concept: Analysis Versioning

### Raw Asset vs Analysis Separation
```
Raw Asset (Immutable):
├── meeting-uuid-123/
│   ├── original-recording.mp4     # Source truth - never changes
│   ├── metadata.json             # Basic facts (date, attendees, title)
│   └── user-notes.md             # Your original notes
│
└── Analysis Versions (Evolving):
    ├── v1-2025-01-basic/
    │   ├── transcript-whisper-v1.txt
    │   ├── summary-llama.md
    │   ├── entities-basic.json
    │   └── analysis-metadata.json
    ├── v2-2025-03-improved/
    │   ├── transcript-whisper-v2.txt
    │   ├── summary-qwen-finetuned.md
    │   ├── entities-enhanced.json
    │   ├── speaker-identification.json  # NEW: Voice analysis added
    │   └── analysis-metadata.json
    └── v3-2025-06-multimodal/
        ├── transcript-whisper-v3.txt
        ├── visual-analysis.json          # NEW: Slides/screen content
        ├── sentiment-analysis.json       # NEW: Emotional context
        └── analysis-metadata.json
```

## Re-Analysis Pipeline Architecture

### Analysis Metadata Tracking
```json
{
  "analysis_id": "uuid-456",
  "raw_asset_id": "meeting-uuid-123",
  "version": "v2-2025-03-improved",
  "created_at": "2025-03-15T10:00:00Z",
  "pipeline_config": {
    "transcription": {
      "model": "whisper-large-v3",
      "language": "en",
      "speaker_diarization": true
    },
    "summarization": {
      "model": "qwen2.5-7b-finetuned",
      "prompt_version": "v2.1",
      "temperature": 0.3
    },
    "entity_extraction": {
      "models": ["llama-3.1-8b", "phi-3-mini"],
      "ensemble_method": "weighted_voting",
      "confidence_threshold": 0.8
    },
    "categorization": {
      "model": "custom-meeting-classifier-v2",
      "project_definitions_version": "2025-03"
    }
  },
  "quality_metrics": {
    "transcription_accuracy": 0.94,
    "entity_extraction_recall": 0.87,
    "user_satisfaction_rating": 4.5
  },
  "supersedes": "v1-2025-01-basic",
  "improvements": [
    "Added speaker identification",
    "Improved entity extraction with fine-tuned model",
    "Better project categorization accuracy"
  ]
}
```

### Re-Analysis Triggers
```bash
# Bulk re-analysis when new capabilities are available
penfold reanalyze bulk --since "2025-01-01" --with-capability "speaker-identification"
> Found 47 meetings recorded since Jan 1
> New capability: speaker-identification via voice-print analysis
> Estimated processing time: 23 hours
> Process in background? [y/n]

# Selective re-analysis for improved models
penfold reanalyze --filter "project:Atlas" --model-upgrade "transcription"
> Found 12 Atlas meetings for re-transcription
> New model: Whisper-large-v4 (15% accuracy improvement)
> Preserve existing analysis or full reprocess? [preserve/full]

# Individual asset re-analysis
penfold meeting reanalyze --meeting-id uuid-123 --add-capabilities "visual,sentiment"
> Original: basic transcript + summary
> Adding: slide content analysis + emotional context
> Estimated time: 2 hours
> Queue for processing? [y/n]
```

## Analysis Pipeline Evolution Examples

### Phase 1: Basic Processing (MVP)
**Capabilities**: Transcription + basic summarization
```
Raw Asset → Whisper → Basic Summary → Simple Entity Extraction
```

### Phase 2: Enhanced Local Models (3 months)
**New Capabilities**: Speaker identification + fine-tuned categorization
```bash
# Re-analyze all meetings with improved pipeline
penfold reanalyze all --upgrade-to "enhanced-local"
> Processing 89 historical meetings...
> New features: speaker diarization, improved entity extraction
> Previous analysis preserved as v1, new analysis as v2
```

### Phase 3: Multimodal Analysis (6 months)
**New Capabilities**: Visual content + sentiment analysis
```bash
# Add visual analysis to video meetings
penfold reanalyze --has-video --add-capability "visual-content"
> Found 34 video meetings with screen sharing
> Will analyze: slides, diagrams, screen content
> Estimated improvement in context understanding: +40%
```

### Phase 4: Advanced AI (12 months)
**New Capabilities**: Advanced reasoning + cross-meeting synthesis
```bash
# Re-analyze with advanced reasoning models
penfold reanalyze --with-advanced-reasoning
> Will apply: GPT-5 level reasoning, cross-meeting connection detection
> Focus on: decision timelines, dependency mapping, risk prediction
```

## Storage and Performance Strategy

### Hot vs Cold Storage
```
Hot Storage (NVMe SSD):
├── Current analysis versions (last 6 months)
├── Frequently queried assets
└── Active processing queue

Cold Storage (HDD):
├── Historical analysis versions
├── Raw assets (always preserved)
└── Archived processing results
```

### Incremental Processing
```python
class ReAnalysisEngine:
    def incremental_reprocess(self, asset_id, new_capabilities):
        """Add new analysis without redoing existing work"""
        existing_analysis = self.get_latest_analysis(asset_id)

        # Only process new capabilities
        new_results = {}
        if 'speaker-id' in new_capabilities:
            new_results['speakers'] = self.analyze_speakers(asset_id)
        if 'visual' in new_capabilities:
            new_results['visual'] = self.analyze_visual_content(asset_id)

        # Merge with existing analysis
        updated_analysis = self.merge_analysis(existing_analysis, new_results)

        return self.save_analysis_version(asset_id, updated_analysis)
```

## Query and Search Evolution

### Version-Aware Searching
```bash
# Search latest analysis only
penfold search "deployment concerns" --analysis-version latest

# Search specific analysis version
penfold search "deployment concerns" --analysis-version v2

# Compare search results across versions
penfold search "deployment concerns" --compare-versions
> v1 results: 12 matches (basic entity extraction)
> v2 results: 18 matches (improved entity extraction + speaker context)
> v3 results: 24 matches (+ visual content from slides)
```

### Historical Analysis Comparison
```bash
# See analysis improvement over time
penfold meeting show uuid-123 --analysis-evolution
> v1 (Jan 2025): Basic transcript, 3 entities, 1 project
> v2 (Mar 2025): Speaker ID, 8 entities, 2 projects, action items
> v3 (Jun 2025): + Visual analysis, sentiment, 12 entities, cross-meeting links
>
> Quality progression: 3.2 → 4.1 → 4.7 (user ratings)
```

## Implementation Benefits

### Future-Proof Investment
- **Raw assets preserved**: Never lose source material
- **Analysis evolution**: Continuous improvement without data loss
- **Technology adoption**: Easy to integrate new AI capabilities
- **Quality metrics**: Track improvement over time

### Learning Laboratory
- **Model comparison**: A/B test new models on historical data
- **Pipeline optimization**: Measure impact of processing changes
- **Capability validation**: Prove value of new features before deployment
- **Cost analysis**: ROI measurement for processing investments

### Business Intelligence Evolution
```bash
# Example: 2 years of analysis evolution enables advanced queries
penfold analyze trends --analysis-evolution
> Analysis: How meeting quality insights improved over 24 months
>
> Jan 2025: Basic keyword search, 65% query satisfaction
> Jan 2026: Multi-modal search, 87% query satisfaction
> Jan 2027: Predictive insights, cross-meeting synthesis, 94% satisfaction
>
> Business impact:
> - Decision timeline reconstruction: 3 hours → 15 minutes
> - Risk identification: Reactive → 2-week early warning
> - Project health monitoring: Manual → Automated insights
```

This architecture treats your content as a **long-term data asset** that becomes more valuable as AI capabilities advance, rather than a one-time processing job.