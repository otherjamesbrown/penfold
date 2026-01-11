# Infrastructure Sizing and Resource Planning

## Data Volume Estimates

### Weekly Input Volume
- **Meetings**: 10-15 per week
- **Emails**: 200 per week
- **Annual projection**:
  - ~650 meetings/year
  - ~10,400 emails/year

### Storage Requirements Analysis

#### Email Storage (200/week)
- Average email: ~5KB text + metadata
- With embeddings (multiple models): ~2KB per email
- Annual emails: ~75MB text + 20MB embeddings = **~100MB/year**

#### Meeting Storage (15/week)
- Meeting metadata: ~1KB
- Personal notes: ~2KB average
- AI summaries: ~3KB average
- Transcripts: ~15KB average (assuming 1 hour meetings, 150 words/minute)
- Audio files: ~50MB average (1 hour, compressed)
- Video files: ~200MB average (1 hour, compressed)
- Documents: ~5MB average per meeting

**Conservative estimate per meeting**: 250MB
**Annual meetings**: 650 × 250MB = **~163GB/year**

#### Total Annual Storage Need
- **Raw content**: ~165GB/year
- **Processed data** (embeddings, indexes): ~50GB/year
- **Model storage** (local models): ~50GB
- **Working space** (temp processing): ~50GB

**Total: ~315GB/year** - well within available storage capacity

## Hardware Architecture

### Primary Development Machine: Mac Mini M4
**Specifications**:
- CPU: M4 chip with Neural Engine
- RAM: 32GB unified memory
- Storage: 2TB SSD
- Role: Primary development, real-time processing, model serving

**Optimal Use**:
- Real-time email/meeting ingestion
- Local model inference (Llama 3.1 8B, Phi-3, Qwen2.5 7B)
- Vector database (Qdrant)
- Development environment
- Daily user interactions

### Storage Server: Network Storage
**Specifications**:
- 2TB NVMe SSD (high-performance working storage)
- 6TB HDD (archival and backup)
- GbE connectivity

**Optimal Use**:
- Raw content storage (emails, meeting recordings, documents)
- Backup and archival
- Large model storage when needed
- Historical data warehouse

### Database Server: Intel NUC
**Specifications**:
- CPU: Intel i7-7567U @ 3.5GHz (4 cores, 8 threads, 2017)
- RAM: 32GB DDR4 (2x16GB)
- Storage: WD Black NVMe SSD
- Network: Likely GbE (Intel NUCs typically have Intel I219-V)

**Optimal Use**:
- PostgreSQL + pgvector database hosting
- Qdrant vector database
- Prometheus monitoring and metrics
- Nightly batch analysis jobs
- Backup and maintenance tasks
- Long-running data processing

**Performance Assessment**:
- **Database workload**: Excellent - i7-7567U + 32GB RAM + NVMe SSD ideal for PostgreSQL
- **Vector operations**: Good - sufficient for pgvector and Qdrant workloads
- **Batch processing**: Adequate - older CPU but sufficient for overnight analysis
- **Network throughput**: Should handle database queries + monitoring traffic easily

## Performance Projections

### Real-time Processing Capacity
**Mac Mini M4 with 32GB RAM can handle**:
- Multiple local models simultaneously
- Real-time email processing: 200 emails << daily capacity
- Meeting upload processing: 15 meetings/week = 2-3 per day
- Vector search across year of data: sub-second response

### Model Serving Strategy
**Simultaneous model serving on M4**:
- Llama 3.1 8B: ~8GB VRAM
- Phi-3 Mini 3.8B: ~4GB VRAM
- Qwen2.5 7B: ~7GB VRAM
- System overhead: ~5GB
- **Total: ~24GB** - fits comfortably in 32GB

### Future Scaling Options
**If value proven and needs expand**:
- Mac Studio (M4 Ultra, 128GB RAM): Large model hosting (70B+ models)
- Additional storage expansion: Scale to 10TB+ if needed
- Intel server upgrade: Modern processors for heavy batch work

## Database Sizing

### PostgreSQL (Structured Data)
**Tables and estimated sizes**:
- Information entities: ~50MB/year (emails + meetings)
- People and projects: ~10MB/year
- Relationships: ~25MB/year
- User feedback: ~5MB/year
- **Total**: ~90MB/year structured data

### Qdrant Vector Database
**Vector storage requirements**:
- Embeddings per document: 4 models × 1536 dimensions × 4 bytes = ~25KB
- Annual documents: ~11,000 (emails + meetings)
- Vector storage: 11K × 25KB = **~275MB/year**
- Plus metadata and indexes: **~400MB/year total**

## Network and Bandwidth

### Local Network Traffic
- Meeting uploads: 250MB × 3 per day = 750MB daily upload
- GbE easily handles: theoretical 125MB/s, practical 50-100MB/s
- Upload time: ~10-15 seconds per meeting
- **Network capacity**: Not a constraint

### Internet Bandwidth (Cloud API Calls)
- Cloud model calls: Text-only processing
- Estimated: 10-50KB per complex query
- Daily usage: <1MB typical, <10MB heavy usage
- **Internet bandwidth**: Minimal impact

## Development Environment Setup

### Local Development Stack
**On Mac Mini M4**:
- Python 3.12 development environment
- PostgreSQL + pgvector extension
- Qdrant vector database
- Ollama for model serving
- Docker for service management

### Storage Mount Strategy
```bash
# Network storage mounts
/mnt/penfold-nvme/     # High-performance working storage
/mnt/penfold-archive/  # Long-term archival storage

# Local storage allocation
~/penfold-dev/         # Development environment
/opt/penfold/          # Production installation
~/.penfold/            # User configuration and caches
```

## Resource Utilization Monitoring

### Key Metrics to Track
- **CPU utilization**: Model inference load
- **Memory usage**: Model serving + vector database
- **Storage growth**: Content accumulation rate
- **Network I/O**: Meeting upload frequency
- **Model performance**: Inference speed and accuracy

### Scaling Triggers
- **CPU >80% sustained**: Consider model optimization or hardware upgrade
- **Memory >28GB used**: Evaluate model selection or RAM upgrade
- **Storage >80% full**: Expand storage or implement archival
- **Query response >5 seconds**: Optimize indexes or upgrade hardware

## Cost Analysis

### Current Infrastructure Cost
- **Hardware**: Already owned, no additional cost
- **Power consumption**: Mac Mini ~20W, minimal impact
- **Cloud API usage**: Estimated <$50/month for complex queries
- **Development time**: Primary cost factor

### Future Scaling Costs
- **Mac Studio upgrade**: ~$4K-8K if needed for larger models
- **Storage expansion**: ~$100-200 per TB as needed
- **Cloud usage scaling**: Proportional to query complexity and frequency

This infrastructure comfortably supports the projected workload with significant room for growth.