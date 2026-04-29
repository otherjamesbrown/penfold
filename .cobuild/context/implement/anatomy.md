# Project Anatomy

Auto-generated file index. Use this to understand the codebase without reading every file.
Token estimates help you decide what's worth reading vs what you can skip.

## Root (~64.4K tokens)

17 files, ~64.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## .cobuild/ (~1.5K tokens)

- **AGENTS.md** (105 lines, ~1.1K tok) — CoBuild Pipeline Instructions
- **pipeline.yaml** (51 lines, ~399 tok) — penfold — repo-specific pipeline config
- **scan.yaml** (3 lines, ~10 tok) — scan.yaml in .cobuild/

## .cxp/context/ (~1.7K tokens)

- **agent-identity.md** (20 lines, ~113 tok) — Mycroft — Penfold Backend Developer
- **architecture.md** (38 lines, ~627 tok) — Architectural Principles — Hard Constraints
- **completion-protocol.md** (30 lines, ~283 tok) — Completion Protocol
- **deploy.md** (26 lines, ~255 tok) — Deploying
- **dispatch-completion.md** (27 lines, ~244 tok) — Dispatched Task — Completion Instructions
- **interactive-menu.md** (23 lines, ~156 tok) — Startup Instructions

## .github/workflows/ (~8.6K tokens)

- **auto-release.yml** (61 lines, ~648 tok) — Triggers when VERSION file changes on main branch
- **ci.yml** (265 lines, ~2.5K tok) — ci.yml in .github/workflows/
- **deploy-verify.yml** (249 lines, ~3.0K tok) — deploy-verify.yml in .github/workflows/
- **proto.yml** (61 lines, ~646 tok) — proto.yml in .github/workflows/
- **release.yml** (174 lines, ~1.8K tok) — release.yml in .github/workflows/

## api/proto/ai/v1/ (~14.5K tokens)

4 files, ~14.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## api/proto/alert/v1/ (~255 tokens)

- **alert.proto** (46 lines, ~255 tok) — alert.proto in api/proto/alert/v1/

## api/proto/assertions/v1/ (~1.3K tokens)

- **assertions.proto** (130 lines, ~1.3K tok) — assertions.proto in api/proto/assertions/v1/

## api/proto/audit/v1/ (~3.1K tokens)

- **audit.proto** (476 lines, ~3.1K tok) — audit.proto in api/proto/audit/v1/

## api/proto/bridge/v1/ (~278 tokens)

- **bridge.proto** (45 lines, ~278 tok) — bridge.proto in api/proto/bridge/v1/

## api/proto/ (~770 tokens)

- **buf.gen.yaml** (14 lines, ~107 tok) — buf.gen.yaml in api/proto/
- **buf.yaml** (16 lines, ~94 tok) — buf.yaml in api/proto/
- **go.mod** (18 lines, ~124 tok) — go.mod in api/proto/
- **go.sum** (21 lines, ~445 tok) — go.sum in api/proto/

## api/proto/classify/v1/ (~613 tokens)

- **classify.proto** (71 lines, ~613 tok) — classify.proto in api/proto/classify/v1/

## api/proto/cli/v1/ (~4.9K tokens)

- **cli.proto** (623 lines, ~4.4K tok) — cli.proto in api/proto/cli/v1/
- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/cli/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/cli/v1/

## api/proto/common/v1/ (~1.2K tokens)

- **common.proto** (126 lines, ~1.2K tok) — common.proto in api/proto/common/v1/
- **go.mod** (6 lines, ~31 tok) — go.mod in api/proto/common/v1/
- **go.sum** (3 lines, ~43 tok) — go.sum in api/proto/common/v1/

## api/proto/connectors/v1/ (~858 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/connectors/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/connectors/v1/
- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/connectors/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/connectors/v1/

## api/proto/connectors/v1/entitypb/ (~2.3K tokens)

- **entity.proto** (339 lines, ~2.3K tok) — entity.proto in api/proto/connectors/v1/entitypb/

## api/proto/connectors/v1/gmailpb/ (~3.1K tokens)

- **gmail.proto** (414 lines, ~3.1K tok) — gmail.proto in api/proto/connectors/v1/gmailpb/

## api/proto/connectors/v1/graphpb/ (~1.4K tokens)

- **graph.proto** (161 lines, ~1.4K tok) — graph.proto in api/proto/connectors/v1/graphpb/

## api/proto/content/v1/ (~11.0K tokens)

4 files, ~11.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## api/proto/conversation/v1/ (~2.4K tokens)

- **conversation.proto** (252 lines, ~2.4K tok) — conversation.proto in api/proto/conversation/v1/

## api/proto/core/v1/ (~856 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/core/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/core/v1/
- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/core/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/core/v1/

## api/proto/core/v1/clipb/ (~4.5K tokens)

- **cli.proto** (644 lines, ~4.5K tok) — cli.proto in api/proto/core/v1/clipb/

## api/proto/core/v1/commonpb/ (~1.2K tokens)

- **common.proto** (126 lines, ~1.2K tok) — common.proto in api/proto/core/v1/commonpb/

## api/proto/core/v1/gatewaypb/ (~3.1K tokens)

- **gateway.proto** (426 lines, ~3.1K tok) — gateway.proto in api/proto/core/v1/gatewaypb/

## api/proto/digest/v1/ (~528 tokens)

- **digest.proto** (87 lines, ~528 tok) — digest.proto in api/proto/digest/v1/

## api/proto/entity/v1/ (~5.5K tokens)

- **entity.proto** (802 lines, ~5.5K tok) — entity.proto in api/proto/entity/v1/

## api/proto/gateway/v1/ (~3.7K tokens)

- **gateway.proto** (426 lines, ~3.1K tok) — gateway.proto in api/proto/gateway/v1/
- **go.mod** (18 lines, ~127 tok) — go.mod in api/proto/gateway/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/gateway/v1/

## api/proto/glossary/v1/ (~2.4K tokens)

- **glossary.proto** (360 lines, ~2.4K tok) — glossary.proto in api/proto/glossary/v1/

## api/proto/gmail/v1/ (~3.6K tokens)

- **gmail.proto** (414 lines, ~3.1K tok) — gmail.proto in api/proto/gmail/v1/
- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/gmail/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/gmail/v1/

## api/proto/ingest/v1/ (~7.0K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/ingest/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/ingest/v1/
- **ingest.proto** (957 lines, ~6.3K tok) — ingest.proto in api/proto/ingest/v1/

## api/proto/instruction/v1/ (~868 tokens)

- **instruction.proto** (103 lines, ~868 tok) — instruction.proto in api/proto/instruction/v1/

## api/proto/intelligence/v1/aipb/ (~3.3K tokens)

- **ai.proto** (381 lines, ~3.3K tok) — ai.proto in api/proto/intelligence/v1/aipb/

## api/proto/intelligence/v1/ (~858 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/intelligence/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/intelligence/v1/
- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/intelligence/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/intelligence/v1/

## api/proto/intelligence/v1/glossarypb/ (~2.4K tokens)

- **glossary.proto** (349 lines, ~2.4K tok) — glossary.proto in api/proto/intelligence/v1/glossarypb/

## api/proto/intelligence/v1/mentionspb/ (~2.5K tokens)

- **mentions.proto** (351 lines, ~2.5K tok) — mentions.proto in api/proto/intelligence/v1/mentionspb/

## api/proto/intelligence/v1/questionspb/ (~2.5K tokens)

- **questions.proto** (346 lines, ~2.5K tok) — questions.proto in api/proto/intelligence/v1/questionspb/

## api/proto/intelligence/v1/relationshippb/ (~3.7K tokens)

- **relationship.proto** (486 lines, ~3.7K tok) — relationship.proto in api/proto/intelligence/v1/relationshippb/

## api/proto/intelligence/v1/searchpb/ (~3.2K tokens)

- **search.proto** (378 lines, ~3.2K tok) — search.proto in api/proto/intelligence/v1/searchpb/

## api/proto/ledger/v1/ (~2.0K tokens)

- **ledger.proto** (220 lines, ~2.0K tok) — ledger.proto in api/proto/ledger/v1/

## api/proto/logs/v1/ (~2.1K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/logs/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/logs/v1/
- **logs.proto** (219 lines, ~1.5K tok) — logs.proto in api/proto/logs/v1/

## api/proto/mentions/v1/ (~2.7K tokens)

- **mentions.proto** (371 lines, ~2.7K tok) — mentions.proto in api/proto/mentions/v1/

## api/proto/orchestrator/v1/ (~3.5K tokens)

- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/orchestrator/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/orchestrator/v1/
- **orchestrator.proto** (362 lines, ~3.0K tok) — orchestrator.proto in api/proto/orchestrator/v1/

## api/proto/pipeline/v1/ (~17.3K tokens)

1 files, ~17.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## api/proto/processing/v1/ (~858 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/processing/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/processing/v1/
- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/processing/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/processing/v1/

## api/proto/processing/v1/contentpb/ (~3.2K tokens)

- **content.proto** (354 lines, ~3.2K tok) — content.proto in api/proto/processing/v1/contentpb/

## api/proto/processing/v1/orchestratorpb/ (~3.0K tokens)

- **orchestrator.proto** (362 lines, ~3.0K tok) — orchestrator.proto in api/proto/processing/v1/orchestratorpb/

## api/proto/processing/v1/reviewpb/ (~3.2K tokens)

- **review.proto** (416 lines, ~3.2K tok) — review.proto in api/proto/processing/v1/reviewpb/

## api/proto/processing/v1/workflowpb/ (~1.8K tokens)

- **workflow.proto** (247 lines, ~1.8K tok) — workflow.proto in api/proto/processing/v1/workflowpb/

## api/proto/product/v1/ (~7.4K tokens)

- **product.proto** (1083 lines, ~7.4K tok) — product.proto in api/proto/product/v1/

## api/proto/project/v1/ (~2.4K tokens)

- **project.proto** (329 lines, ~2.4K tok) — project.proto in api/proto/project/v1/

## api/proto/quality/v1/ (~1.2K tokens)

- **quality.proto** (141 lines, ~1.2K tok) — quality.proto in api/proto/quality/v1/

## api/proto/questions/v1/ (~2.5K tokens)

- **questions.proto** (353 lines, ~2.5K tok) — questions.proto in api/proto/questions/v1/

## api/proto/relationship/v1/ (~8.2K tokens)

- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/relationship/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/relationship/v1/
- **relationship.proto** (1011 lines, ~7.6K tok) — relationship.proto in api/proto/relationship/v1/

## api/proto/review/v1/ (~5.2K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/review/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/review/v1/
- **review.proto** (595 lines, ~4.7K tok) — review.proto in api/proto/review/v1/

## api/proto/schedule/v1/ (~782 tokens)

- **schedule.proto** (122 lines, ~782 tok) — schedule.proto in api/proto/schedule/v1/

## api/proto/search/v1/ (~4.0K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/search/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/search/v1/
- **search.proto** (406 lines, ~3.5K tok) — search.proto in api/proto/search/v1/

## api/proto/source_mappings/v1/ (~827 tokens)

- **source_mappings.proto** (102 lines, ~827 tok) — source_mappings.proto in api/proto/source_mappings/v1/

## api/proto/teams/v1/ (~1.5K tokens)

- **teams.proto** (229 lines, ~1.5K tok) — teams.proto in api/proto/teams/v1/

## api/proto/tenant/v1/ (~1.9K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/tenant/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/tenant/v1/
- **tenant.proto** (195 lines, ~1.4K tok) — tenant.proto in api/proto/tenant/v1/

## api/proto/threads/v1/ (~744 tokens)

- **threads.proto** (89 lines, ~744 tok) — threads.proto in api/proto/threads/v1/

## api/proto/topic/v1/ (~1.3K tokens)

- **topic.proto** (192 lines, ~1.3K tok) — topic.proto in api/proto/topic/v1/

## api/proto/watchlist/v1/ (~1.7K tokens)

- **watchlist.proto** (200 lines, ~1.7K tok) — watchlist.proto in api/proto/watchlist/v1/

## api/proto/workflow/v1/ (~3.7K tokens)

- **go.mod** (18 lines, ~127 tok) — go.mod in api/proto/workflow/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/workflow/v1/
- **workflow.proto** (419 lines, ~3.2K tok) — workflow.proto in api/proto/workflow/v1/

## configs/ (~812 tokens)

- **enrichment_processors.yaml** (91 lines, ~812 tok) — Enrichment Pipeline Processor Configuration

## context-archive/ (~17.8K tokens)

4 files, ~17.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## context-archive/agents/ (~15.2K tokens)

9 files, ~15.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## context-archive/architecture/ (~10.4K tokens)

6 files, ~10.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## context-archive/beads/ (~1.9K tokens)

- **pe-egic-ssl-client-certs.md** (69 lines, ~544 tok) — PE-EGIC: PostgreSQL SSL Client Certificate Authentication
- **pe-gwcf-gateway-config.md** (61 lines, ~569 tok) — PE-GWCF: Gateway Configuration Fix
- **pe-tmsl-temporal-ssl.md** (80 lines, ~779 tok) — PE-TMSL: Temporal PostgreSQL SSL Certificate Authentication

## context-archive/development/ (~2.0K tokens)

- **index.md** (238 lines, ~2.0K tok) — Development Context (Sub-Agents)

## context-archive/development/standards/ (~5.5K tokens)

- **architecture.md** (60 lines, ~476 tok) — Architecture Standards
- **autonomy.md** (117 lines, ~801 tok) — Autonomy Guidelines
- **go-patterns.md** (77 lines, ~544 tok) — Go Patterns
- **logging.md** (222 lines, ~1.5K tok) — Logging Standards
- **testing.md** (290 lines, ~2.2K tok) — Testing Standards

## context-archive/development/workflows/ (~5.8K tokens)

- **deployment-checklist.md** (377 lines, ~2.5K tok) — Deployment Checklist
- **priorities.md** (64 lines, ~399 tok) — Finding Current Priorities
- **releases.md** (167 lines, ~1.1K tok) — Release Workflows
- **session.md** (47 lines, ~339 tok) — Session Protocol
- **shards.md** (190 lines, ~1.5K tok) — Shards Workflow

## context-archive/plans/ (~2.7K tokens)

- **agent-mail-integration.md** (340 lines, ~2.7K tok) — Agent Mail Integration Plan

## context/client/ (~6.2K tokens)

- **assistant-rules.md** (352 lines, ~3.2K tok) — Penfold Assistant Rules
- **index.md** (269 lines, ~2.2K tok) — Penfold System Documentation
- **preferences.md** (86 lines, ~523 tok) — Penfold User Preferences
- **processes.md** (34 lines, ~254 tok) — Penfold Processes

## context/client/concepts/ (~6.1K tokens)

- **entities.md** (138 lines, ~959 tok) — Entities
- **glossary.md** (183 lines, ~1.2K tok) — Glossary Terms
- **mentions.md** (191 lines, ~1.9K tok) — Mentions
- **people.md** (169 lines, ~1.0K tok) — People
- **products.md** (211 lines, ~1.1K tok) — Products

## context/client/workflows/ (~6.1K tokens)

- **acronym-review.md** (179 lines, ~1.5K tok) — Workflow: Acronym Review
- **init-entities.md** (236 lines, ~1.4K tok) — Workflow: Init Entities
- **mention-review.md** (259 lines, ~1.6K tok) — Workflow: Mention Review
- **onboarding.md** (235 lines, ~1.6K tok) — Workflow: Post-Import Onboarding

## context/shared/ (~15.0K tokens)

5 files, ~15.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## deploy/ (~1.1K tokens)

- **README.md** (107 lines, ~777 tok) — Penfold Deployment Configuration
- **mcp.env** (21 lines, ~168 tok) — mcp.env in deploy/
- **penfold-mcp.service** (35 lines, ~190 tok) — penfold-mcp.service in deploy/

## deploy/certs/ (~547 tokens)

- **ca.crt** (34 lines, ~547 tok) — ca.crt in deploy/certs/

## deploy/env/ (~909 tokens)

- **README.md** (152 lines, ~909 tok) — Penfold Environment Configuration

## deploy/langfuse/ (~2.7K tokens)

- **README.md** (89 lines, ~453 tok) — Langfuse Deployment for Penfold AI Provenance
- **docker-compose.yml** (187 lines, ~2.3K tok) — Langfuse deployment for Penfold AI Provenance

## deploy/launchd/ (~2.0K tokens)

- **README.md** (127 lines, ~780 tok) — Penfold launchd Services (macOS)
- **com.penfold.worker.plist** (53 lines, ~306 tok) — com.penfold.worker.plist in deploy/launchd/
- **install.sh** (112 lines, ~837 tok) — install.sh in deploy/launchd/
- **penfold-worker-start.sh** (22 lines, ~121 tok) — penfold-worker-start.sh in deploy/launchd/

## deploy/launchd/RETIRED/ (~660 tokens)

- **com.penfold.worker.plist** (85 lines, ~660 tok) — com.penfold.worker.plist in deploy/launchd/RETIRED/

## deploy/nomad-archived/ (~1.7K tokens)

- **README.md** (11 lines, ~113 tok) — Nomad Deployment (ARCHIVED)
- **ai-coordinator.nomad.hcl** (92 lines, ~481 tok) — ai-coordinator.nomad.hcl in deploy/nomad-archived/
- **gateway.nomad.hcl** (92 lines, ~468 tok) — gateway.nomad.hcl in deploy/nomad-archived/
- **mlx-services.nomad.hcl** (8 lines, ~104 tok) — mlx-services.nomad.hcl in deploy/nomad-archived/
- **worker.nomad.hcl** (97 lines, ~556 tok) — worker.nomad.hcl in deploy/nomad-archived/

## deploy/observability/ (~2.6K tokens)

- **README.md** (289 lines, ~1.8K tok) — Penfold Observability Stack
- **docker-compose.yml** (99 lines, ~819 tok) — docker-compose.yml in deploy/observability/

## deploy/observability/alertmanager/ (~219 tokens)

- **alertmanager.yml** (33 lines, ~219 tok) — alertmanager.yml in deploy/observability/alertmanager/

## deploy/observability/grafana/provisioning/dashboards/ (~90 tokens)

- **dashboards.yml** (14 lines, ~90 tok) — dashboards.yml in deploy/observability/grafana/provisioning/dashboards/

## deploy/observability/grafana/provisioning/dashboards/json/ (~3.8K tokens)

- **penfold-overview.json** (229 lines, ~3.8K tok) — penfold-overview.json in deploy/observability/grafana/provisioning/dashboards/json/

## deploy/observability/grafana/provisioning/datasources/ (~108 tokens)

- **datasources.yml** (20 lines, ~108 tok) — datasources.yml in deploy/observability/grafana/provisioning/datasources/

## deploy/observability/loki/ (~374 tokens)

- **config.yml** (57 lines, ~374 tok) — config.yml in deploy/observability/loki/

## deploy/observability/prometheus/ (~610 tokens)

- **prometheus.yml** (81 lines, ~610 tok) — prometheus.yml in deploy/observability/prometheus/

## deploy/observability/prometheus/rules/ (~1.4K tokens)

- **penfold.yml** (130 lines, ~1.4K tok) — penfold.yml in deploy/observability/prometheus/rules/

## deploy/observability/promtail/ (~418 tokens)

- **config.yml** (53 lines, ~418 tok) — config.yml in deploy/observability/promtail/

## deploy/systemd/ (~2.8K tokens)

- **README.md** (98 lines, ~681 tok) — Penfold systemd Services (Linux)
- **install.sh** (157 lines, ~1.2K tok) — install.sh in deploy/systemd/
- **penfold-ai-coordinator.service** (36 lines, ~203 tok) — penfold-ai-coordinator.service in deploy/systemd/
- **penfold-alert-webhook.service** (27 lines, ~149 tok) — penfold-alert-webhook.service in deploy/systemd/
- **penfold-gateway.service** (41 lines, ~254 tok) — penfold-gateway.service in deploy/systemd/
- **penfold-gmail.service** (42 lines, ~248 tok) — penfold-gmail.service in deploy/systemd/

## docs/adr/ (~3.1K tokens)

- **tempts-evaluation.md** (281 lines, ~3.1K tok) — ADR: Evaluate tempts for Temporal Workflow Type Safety

## docs/ai-coordination/ (~3.2K tokens)

- **README.md** (456 lines, ~3.2K tok) — AI Coordination Framework User Guide

## docs/analysis/ (~1.5K tokens)

- **e2e-glossary-test-failures.md** (204 lines, ~1.5K tok) — E2E Glossary Test Failures Analysis

## docs/ (~14.0K tokens)

6 files, ~14.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## docs/benchmarks/ (~803 tokens)

- **llm-pipeline-benchmark-2026-01-23.md** (97 lines, ~803 tok) — LLM Model Benchmark Results

## docs/concepts/ (~6.1K tokens)

- **entities.md** (138 lines, ~959 tok) — Entities
- **glossary.md** (183 lines, ~1.2K tok) — Glossary Terms
- **mentions.md** (191 lines, ~1.9K tok) — Mentions
- **people.md** (169 lines, ~1.0K tok) — People
- **products.md** (211 lines, ~1.1K tok) — Products

## docs/database-schema/ (~5.2K tokens)

- **README.md** (600 lines, ~5.2K tok) — Penfold Database Schema Documentation

## docs/event-processing/ (~3.5K tokens)

- **README.md** (494 lines, ~3.5K tok) — Event Processing Framework - User Guide

## docs/gmail-integration/ (~25.8K tokens)

5 files, ~25.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## docs/infrastructure/ (~35.7K tokens)

7 files, ~35.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## docs/meeting-pipeline/ (~7.8K tokens)

- **README.md** (157 lines, ~1.3K tok) — Meeting Pipeline Documentation
- **api-reference.md** (646 lines, ~3.7K tok) — Meeting Pipeline API Reference
- **user-guide.md** (481 lines, ~2.9K tok) — Meeting Pipeline User Guide

## docs/observability-framework/ (~8.0K tokens)

- **README.md** (465 lines, ~3.2K tok) — Penfold Observability Framework
- **quickstart.md** (760 lines, ~4.8K tok) — Observability Framework Quickstart Guide

## docs/relationship-discovery/ (~2.5K tokens)

- **README.md** (133 lines, ~975 tok) — Relationship Discovery and Management
- **api-reference.md** (257 lines, ~1.5K tok) — Relationship Discovery API Reference

## docs/shared/ (~12.4K tokens)

4 files, ~12.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## docs/testing-framework/ (~9.2K tokens)

- **BENCHMARKING.md** (59 lines, ~337 tok) — Benchmark Tests
- **FIXTURES-GUIDE.md** (138 lines, ~960 tok) — Test Fixtures
- **LOCAL-SETUP.md** (59 lines, ~423 tok) — Local Test Setup
- **MIGRATIONS.md** (58 lines, ~590 tok) — Migration Workflow
- **TROUBLESHOOTING.md** (79 lines, ~718 tok) — Test Troubleshooting
- **ai-mocking.md** (859 lines, ~6.2K tok) — AI Model Mocking Strategies

## docs/workflows/ (~6.1K tokens)

- **acronym-review.md** (179 lines, ~1.5K tok) — Workflow: Acronym Review
- **init-entities.md** (236 lines, ~1.4K tok) — Workflow: Init Entities
- **mention-review.md** (259 lines, ~1.6K tok) — Workflow: Mention Review
- **onboarding.md** (235 lines, ~1.6K tok) — Workflow: Post-Import Onboarding

## migrations/ (~141.9K tokens)

173 files, ~141.9K tokens — summarized. Use `cobuild scan --verbose` to expand.

## ops/grafana/dashboards/ (~18.4K tokens)

3 files, ~18.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## penfold-go-pipeline/ (~7.2K tokens)

- **Makefile** (138 lines, ~963 tok) — Makefile in penfold-go-pipeline/
- **README.md** (194 lines, ~1.6K tok) — Penfold Go AI Processing Pipeline
- **docker-compose.temporal.yml** (58 lines, ~516 tok) — Temporal Server for Penfold AI Processing Pipeline
- **go.mod** (58 lines, ~669 tok) — go.mod in penfold-go-pipeline/
- **go.sum** (143 lines, ~3.5K tok) — go.sum in penfold-go-pipeline/

## penfold-go-pipeline/cmd/pipeline/ (~1.9K tokens)

- **main.go** (222 lines, ~1.9K tok) — Package main provides the entry point for the penfold-go-pipeline service.

## penfold-go-pipeline/cmd/worker/ (~1.7K tokens)

- **main.go** (193 lines, ~1.7K tok) — Package main provides the entry point for the Penfold Temporal worker.

## penfold-go-pipeline/docs/ (~6.1K tokens)

- **temporal-integration-plan.md** (714 lines, ~6.1K tok) — Temporal Workflow Orchestration - Integration Plan

## penfold-go-pipeline/internal/activities/ (~3.2K tokens)

- **activities.go** (75 lines, ~754 tok) — Package activities provides Temporal activity implementations for the Penfold pipeline.
- **embedding.go** (66 lines, ~590 tok) — Go package: activities
- **llm.go** (128 lines, ~1.2K tok) — Go package: activities
- **source.go** (50 lines, ~424 tok) — Go package: activities
- **storage.go** (38 lines, ~312 tok) — Go package: activities

## penfold-go-pipeline/internal/clients/ (~2.0K tokens)

- **ai_adapter.go** (230 lines, ~2.0K tok) — Package clients provides AI client adapters for the pipeline.

## penfold-go-pipeline/internal/config/ (~1.4K tokens)

- **config.go** (131 lines, ~1.4K tok) — Package config provides environment-based configuration for the penfold-go-pipeline.

## penfold-go-pipeline/internal/events/ (~6.8K tokens)

- **router.go** (278 lines, ~2.4K tok) — Go package: events
- **schemas.go** (272 lines, ~2.6K tok) — Package events provides event type definitions and infrastructure for the Penfold pipeline.
- **subscriber.go** (286 lines, ~1.8K tok) — Go package: events

## penfold-go-pipeline/internal/health/ (~2.5K tokens)

- **health.go** (349 lines, ~2.5K tok) — Package health provides HTTP health check endpoints for the pipeline service.

## penfold-go-pipeline/internal/storage/ (~10.6K tokens)

4 files, ~10.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## penfold-go-pipeline/internal/temporal/ (~4.0K tokens)

- **activities.go** (91 lines, ~807 tok) — Package temporal provides Temporal client factory and utilities for Penfold.
- **activities_test.go** (111 lines, ~911 tok) — Go package: temporal
- **client.go** (82 lines, ~630 tok) — Package temporal provides Temporal client factory and configuration for Penfold.
- **heartbeat.go** (101 lines, ~944 tok) — Package temporal provides Temporal client factory and utilities for Penfold.
- **heartbeat_test.go** (86 lines, ~678 tok) — Go package: temporal

## penfold-go-pipeline/internal/workflows/ (~2.0K tokens)

- **email.go** (216 lines, ~2.0K tok) — Package workflows provides Temporal workflow definitions for the Penfold pipeline.

## penfold-go-pipeline/sidecar/ (~1.4K tokens)

- **app.py** (166 lines, ~1.3K tok) — MLX Embeddings Sidecar Service.
- **mlx-sidecar.service** (23 lines, ~130 tok) — mlx-sidecar.service in penfold-go-pipeline/sidecar/
- **requirements.txt** (6 lines, ~30 tok) — requirements.txt in penfold-go-pipeline/sidecar/

## penfold-go-pipeline/temporal-config/ (~254 tokens)

- **dynamic_config.yaml** (32 lines, ~254 tok) — Temporal Dynamic Configuration for Penfold AI Processing Pipeline

## pkg/ai/ (~12.4K tokens)

4 files, ~12.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/alert/ (~1.0K tokens)

- **repository.go** (138 lines, ~1.0K tok) — Package alert provides the repository layer for alert management.

## pkg/assertions/ (~5.9K tokens)

- **repository.go** (545 lines, ~4.1K tok) — Package assertions provides database operations for querying assertions across all content.
- **repository_test.go** (232 lines, ~1.8K tok) — Go package: assertions

## pkg/auth/ (~10.3K tokens)

4 files, ~10.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/automation/ (~6.9K tokens)

- **models.go** (97 lines, ~1.0K tok) — Package automation provides repository and types for automation rules.
- **repository.go** (448 lines, ~3.9K tok) — Go package: automation
- **repository_test.go** (271 lines, ~2.0K tok) — Go package: automation

## pkg/buildinfo/ (~2.9K tokens)

- **buildinfo.go** (52 lines, ~399 tok) — Go package: buildinfo
- **buildinfo_integration_test.go** (122 lines, ~995 tok) — Go package: buildinfo_test
- **buildinfo_test.go** (125 lines, ~811 tok) — Go package: buildinfo
- **handler_test.go** (89 lines, ~648 tok) — Go package: buildinfo_test

## pkg/chunking/ (~7.1K tokens)

- **chunker.go** (448 lines, ~3.2K tok) — Package chunking provides utilities for splitting text into embedding-safe chunks.
- **chunker_test.go** (486 lines, ~3.9K tok) — Go package: chunking

## pkg/classify/ (~4.3K tokens)

- **suggestion_repository.go** (171 lines, ~1.2K tok) — Package classify repository
- **suggestion_repository_test.go** (372 lines, ~3.0K tok) — Go package: classify
- **types.go** (16 lines, ~113 tok) — Package classify provides classification suggestion types and repository.

## pkg/config/ (~8.7K tokens)

- **config.go** (359 lines, ~2.6K tok) — Package config provides shared configuration loading for Penfold Go microservices.
- **config_test.go** (755 lines, ~5.8K tok) — Go package: config
- **tenant.go** (31 lines, ~244 tok) — Go package: config

## pkg/contentid/ (~3.7K tokens)

- **contentid.go** (192 lines, ~1.5K tok) — Package contentid provides unique content identifier generation and validation.
- **contentid_test.go** (332 lines, ~2.3K tok) — Go package: contentid

## pkg/db/ (~14.6K tokens)

8 files, ~14.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/digest/ (~7.7K tokens)

- **gather.go** (363 lines, ~3.5K tok) — Package digest provides data gathering functions for digest generation.
- **repository.go** (249 lines, ~2.0K tok) — Package digest provides the repository layer for digest management.
- **window.go** (78 lines, ~654 tok) — Go package: digest
- **window_test.go** (211 lines, ~1.5K tok) — Go package: digest

## pkg/embeddings/ (~34.8K tokens)

11 files, ~34.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/classification/ (~21.3K tokens)

5 files, ~21.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/ (~18.7K tokens)

6 files, ~18.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/config/ (~13.5K tokens)

5 files, ~13.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/entities/ (~58.3K tokens)

12 files, ~58.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/extraction/ (~15.8K tokens)

5 files, ~15.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/handlers/ (~18.6K tokens)

5 files, ~18.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/observability/ (~11.2K tokens)

4 files, ~11.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/enrichment/pipeline/ (~3.9K tokens)

- **pipeline.go** (472 lines, ~3.9K tok) — Package pipeline provides the enrichment pipeline orchestrator.

## pkg/enrichment/processors/ (~6.4K tokens)

- **config.go** (203 lines, ~1.7K tok) — Go package: processors
- **config_test.go** (160 lines, ~1.3K tok) — Go package: processors
- **processor.go** (132 lines, ~1.2K tok) — Package processors defines the interfaces and common types for enrichment processors.
- **registry.go** (115 lines, ~804 tok) — Go package: processors
- **registry_test.go** (173 lines, ~1.4K tok) — Go package: processors

## pkg/enrichment/query/ (~8.1K tokens)

- **assertions.go** (81 lines, ~868 tok) — Go package: query
- **jira.go** (41 lines, ~392 tok) — Go package: query
- **people.go** (41 lines, ~424 tok) — Go package: query
- **projects.go** (33 lines, ~308 tok) — Go package: query
- **query.go** (106 lines, ~990 tok) — Package query provides a shared query library for accessing enrichment data.
- **query_test.go** (239 lines, ~1.6K tok) — Go package: query
- **status.go** (33 lines, ~347 tok) — Go package: query
- **threads.go** (49 lines, ~544 tok) — Go package: query
- **types.go** (209 lines, ~2.6K tok) — Go package: query

## pkg/enrichment/queues/ (~7.7K tokens)

- **errors.go** (104 lines, ~847 tok) — Go package: queues
- **messages.go** (204 lines, ~2.0K tok) — Package queues provides queue infrastructure for the enrichment pipeline.
- **messages_test.go** (139 lines, ~1.0K tok) — Go package: queues
- **redis.go** (399 lines, ~3.0K tok) — Go package: queues
- **retry.go** (121 lines, ~813 tok) — Go package: queues

## pkg/enrichment/routing/ (~4.7K tokens)

- **repository.go** (53 lines, ~338 tok) — Go package: routing
- **router.go** (62 lines, ~475 tok) — Go package: routing
- **router_test.go** (347 lines, ~3.6K tok) — Go package: routing
- **types.go** (33 lines, ~291 tok) — Package routing provides data-driven pipeline routing for the Penfold pipeline.

## pkg/enrichment/workers/ (~2.3K tokens)

- **pool.go** (370 lines, ~2.3K tok) — Package workers provides worker pool management for the enrichment pipeline.

## pkg/errors/ (~8.7K tokens)

- **codes.go** (122 lines, ~1.2K tok) — Go package: errors
- **codes_test.go** (129 lines, ~1.1K tok) — Go package: errors
- **errors.go** (80 lines, ~722 tok) — Package errors provides common domain error types for the penfold application.
- **errors_test.go** (178 lines, ~1.1K tok) — Go package: errors
- **pipeline.go** (184 lines, ~1.6K tok) — Go package: errors
- **pipeline_test.go** (393 lines, ~2.9K tok) — Go package: errors

## pkg/glossary/ (~9.6K tokens)

- **matcher.go** (272 lines, ~2.0K tok) — Go package: glossary
- **repository.go** (685 lines, ~5.5K tok) — Go package: glossary
- **repository_test.go** (123 lines, ~985 tok) — Go package: glossary
- **types.go** (81 lines, ~1.1K tok) — Package glossary provides domain terminology and acronym management for query expansion.

## pkg/ (~6.6K tokens)

- **go.mod** (87 lines, ~1.1K tok) — go.mod in pkg/
- **go.sum** (219 lines, ~5.5K tok) — go.sum in pkg/

## pkg/graph/ (~15.0K tokens)

11 files, ~15.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/health/ (~6.0K tokens)

- **checks.go** (106 lines, ~948 tok) — Go package: health
- **health.go** (228 lines, ~1.6K tok) — Package health provides shared health check functionality for Go microservices.
- **health_test.go** (513 lines, ~3.4K tok) — Go package: health

## pkg/ingest/attachments/ (~10.6K tokens)

5 files, ~10.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/ingest/batch/ (~14.5K tokens)

4 files, ~14.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/ingest/eml/ (~11.8K tokens)

4 files, ~11.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/ingest/eml/testdata/ (~591 tokens)

- **multipart.eml** (32 lines, ~221 tok) — multipart.eml in pkg/ingest/eml/testdata/
- **no_message_id.eml** (9 lines, ~66 tok) — no_message_id.eml in pkg/ingest/eml/testdata/
- **simple.eml** (14 lines, ~76 tok) — simple.eml in pkg/ingest/eml/testdata/
- **with_attachment.eml** (32 lines, ~228 tok) — with_attachment.eml in pkg/ingest/eml/testdata/

## pkg/ingest/events/ (~4.6K tokens)

- **publisher.go** (344 lines, ~3.1K tok) — Package events provides event publishing for the email ingest pipeline.
- **publisher_test.go** (195 lines, ~1.5K tok) — Go package: events

## pkg/ingest/meeting/ (~21.0K tokens)

15 files, ~21.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/ingest/storage/ (~11.1K tokens)

3 files, ~11.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/ingest/teams/ (~3.0K tokens)

- **parser.go** (143 lines, ~1.1K tok) — Package teams provides parsing for Microsoft Teams channel messages.
- **parser_test.go** (263 lines, ~1.9K tok) — Go package: teams

## pkg/ingest/types/ (~568 tokens)

- **attachment.go** (62 lines, ~568 tok) — Package types provides shared types for the ingest pipeline.

## pkg/instructions/ (~3.8K tokens)

- **repository.go** (447 lines, ~3.8K tok) — Package instructions provides the repository layer for watch instructions management.

## pkg/langfuse/ (~15.7K tokens)

6 files, ~15.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/ledger/ (~6.5K tokens)

- **repository.go** (764 lines, ~5.6K tok) — Go package: ledger
- **types.go** (92 lines, ~879 tok) — Package ledger provides types and repository for session ledger management.

## pkg/logging/ (~9.8K tokens)

- **logger.go** (402 lines, ~2.8K tok) — Package logging provides structured logging for Penfold Go microservices.
- **logger_test.go** (403 lines, ~2.9K tok) — Go package: logging
- **sink.go** (279 lines, ~1.7K tok) — Go package: logging
- **sink_test.go** (351 lines, ~2.4K tok) — Go package: logging

## pkg/logs/ (~3.9K tokens)

- **repository.go** (415 lines, ~2.9K tok) — Go package: logs
- **types.go** (96 lines, ~953 tok) — Package logs provides types and operations for service log management.

## pkg/mentions/audit/ (~21.3K tokens)

6 files, ~21.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/mentions/learning/ (~3.9K tokens)

- **clusters.go** (278 lines, ~1.9K tok) — Package learning provides correction tracking and learning capabilities.
- **corrections.go** (239 lines, ~2.0K tok) — Package learning provides correction tracking and learning capabilities for mention resolution.

## pkg/mentions/ (~12.0K tokens)

3 files, ~12.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/mentions/resolver/ (~36.1K tokens)

13 files, ~36.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/metrics/ (~8.4K tokens)

- **metrics.go** (218 lines, ~1.8K tok) — Package metrics provides shared Prometheus metrics functionality for Go microservices.
- **metrics_test.go** (261 lines, ~1.8K tok) — Go package: metrics
- **middleware.go** (203 lines, ~1.5K tok) — Go package: metrics
- **middleware_test.go** (441 lines, ~3.3K tok) — Go package: metrics

## pkg/migration/validation/ (~28.8K tokens)

6 files, ~28.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/parse/ (~22.6K tokens)

5 files, ~22.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/pipeline/ (~31.2K tokens)

11 files, ~31.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/products/ (~36.7K tokens)

11 files, ~36.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/projects/ (~5.7K tokens)

- **repository.go** (562 lines, ~4.1K tok) — Go package: projects
- **repository_test.go** (134 lines, ~986 tok) — Go package: projects
- **types.go** (60 lines, ~637 tok) — Package projects provides types and repository for project management.

## pkg/relationships/ (~24.2K tokens)

4 files, ~24.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/repository/ (~11.7K tokens)

2 files, ~11.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/reviewqueue/ (~11.8K tokens)

4 files, ~11.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/schedule/ (~6.8K tokens)

- **repository.go** (290 lines, ~2.7K tok) — Package schedule provides the repository and Temporal wrapper for DB-driven schedule management.
- **repository_execution_test.go** (81 lines, ~594 tok) — Go package: schedule
- **temporal.go** (214 lines, ~2.0K tok) — Go package: schedule
- **temporal_trigger_test.go** (180 lines, ~1.6K tok) — Go package: schedule

## pkg/source_mappings/ (~2.0K tokens)

- **repository.go** (247 lines, ~2.0K tok) — Package source_mappings provides database operations for project source mappings.

## pkg/sources/ (~1.1K tokens)

- **repository.go** (138 lines, ~1.1K tok) — Package sources provides database access for content sources.

## pkg/temporal/ (~75.4K tokens)

33 files, ~75.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/temporal/observability/ (~34.1K tokens)

10 files, ~34.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/tenant/ (~2.8K tokens)

- **repository.go** (351 lines, ~2.4K tok) — Go package: tenant
- **types.go** (36 lines, ~386 tok) — Package tenant provides multi-tenant management for Penfold.

## pkg/testfixtures/ (~4.7K tokens)

- **loader.go** (327 lines, ~3.0K tok) — Package testfixtures provides data types and loaders for test fixtures.
- **types.go** (82 lines, ~723 tok) — Package testfixtures provides data types and loaders for test fixtures.
- **validate_test.go** (130 lines, ~988 tok) — Go package: testfixtures

## pkg/timeout/ (~5.5K tokens)

- **config.go** (277 lines, ~2.1K tok) — Package timeout provides runtime configuration management for pipeline timeouts.
- **config_test.go** (515 lines, ~3.4K tok) — Go package: timeout

## pkg/timeouts/ (~1.3K tokens)

- **timeouts.go** (56 lines, ~523 tok) — Package timeouts provides centralized timeout constants for the AI/LLM request path.
- **timeouts_test.go** (96 lines, ~763 tok) — Go package: timeouts

## pkg/topics/ (~3.2K tokens)

- **repository.go** (396 lines, ~2.8K tok) — Go package: topics
- **types.go** (32 lines, ~323 tok) — Package topics provides types and repository for topic management.

## pkg/tracing/ (~20.6K tokens)

6 files, ~20.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## pkg/watchlist/ (~5.9K tokens)

- **repository.go** (396 lines, ~3.0K tok) — Go package: watchlist
- **repository_test.go** (261 lines, ~2.3K tok) — Go package: watchlist
- **types.go** (56 lines, ~626 tok) — Package watchlist provides types and repository for watch list and priority management.

## processes/ (~1.5K tokens)

- **acronym-review.md** (179 lines, ~1.5K tok) — Workflow: Acronym Review

## project-lifecycle/ (~2.5K tokens)

- **agent-management.md** (281 lines, ~2.5K tok) — Agent Lifecycle Management

## review/arch/2026-01-16T13-32-45Z/ (~30.1K tokens)

8 files, ~30.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## review/arch/2026-01-23T08-47-58Z/ (~29.3K tokens)

8 files, ~29.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## scripts/ (~28.1K tokens)

16 files, ~28.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## scripts/certs/ (~3.7K tokens)

- **create-ca.sh** (204 lines, ~1.5K tok) — create-ca.sh in scripts/certs/
- **create-client-cert.sh** (286 lines, ~2.2K tok) — create-client-cert.sh in scripts/certs/

## scripts/lib/ (~2.3K tokens)

- **deploy-common.sh** (238 lines, ~2.3K tok) — deploy-common.sh in scripts/lib/

## services/ai/ (~9.8K tokens)

- **Makefile** (84 lines, ~432 tok) — Makefile in services/ai/
- **go.mod** (58 lines, ~662 tok) — go.mod in services/ai/
- **go.sum** (111 lines, ~2.7K tok) — go.sum in services/ai/
- **main.go** (438 lines, ~4.1K tok) — Package main provides the entry point for the AI Coordinator service.
- **main_test.go** (219 lines, ~1.9K tok) — Go package: main

## services/ai/backend/ (~46.8K tokens)

13 files, ~46.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/config/ (~12.9K tokens)

4 files, ~12.9K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/cost/ (~28.2K tokens)

9 files, ~28.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/ensemble/ (~28.1K tokens)

6 files, ~28.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/escalation/ (~30.0K tokens)

6 files, ~30.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/integration/ (~7.2K tokens)

- **integration_test.go** (863 lines, ~7.2K tok) — Package integration provides integration tests for the AI Coordinator service.

## services/ai/registry/ (~31.4K tokens)

10 files, ~31.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/router/ (~18.0K tokens)

3 files, ~18.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/selector/ (~15.3K tokens)

3 files, ~15.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/server/ (~108.3K tokens)

22 files, ~108.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/ai/testutil/ (~737 tokens)

- **mlx.go** (110 lines, ~737 tok) — Package testutil provides test helpers for the AI service.

## services/gateway/alertservice/ (~746 tokens)

- **service.go** (81 lines, ~746 tok) — Package alertservice implements the AlertService gRPC server.

## services/gateway/assertionsservice/ (~1.9K tokens)

- **service.go** (246 lines, ~1.9K tok) — Package assertionsservice implements the AssertionsService gRPC server.

## services/gateway/auditservice/ (~3.7K tokens)

- **service.go** (444 lines, ~3.7K tok) — Package auditservice implements the AuditService gRPC server.

## services/gateway/bridgeservice/ (~4.6K tokens)

- **repository.go** (187 lines, ~1.5K tok) — Package bridgeservice implements the BridgeService gRPC server.
- **service.go** (351 lines, ~3.1K tok) — Package bridgeservice implements the BridgeService gRPC server.

## services/gateway/classifyservice/ (~3.6K tokens)

- **service.go** (117 lines, ~1.1K tok) — Package classifyservice implements the ClassificationSuggestionService gRPC server.
- **service_test.go** (312 lines, ~2.4K tok) — Go package: classifyservice

## services/gateway/config/ (~4.3K tokens)

- **config.go** (541 lines, ~4.3K tok) — Package config provides gateway service-specific configuration.

## services/gateway/contentservice/ (~45.0K tokens)

7 files, ~45.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/conversationservice/ (~57.0K tokens)

9 files, ~57.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/digestservice/ (~2.9K tokens)

- **service.go** (328 lines, ~2.9K tok) — Package digestservice implements the DigestService gRPC server.

## services/gateway/entitymanagementservice/ (~23.1K tokens)

2 files, ~23.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/entityservice/ (~15.9K tokens)

2 files, ~15.9K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/glossaryservice/ (~5.9K tokens)

- **service.go** (503 lines, ~4.4K tok) — Package glossaryservice implements the GlossaryService gRPC server.
- **service_test.go** (121 lines, ~1.5K tok) — Go package: glossaryservice

## services/gateway/gmailproxyservice/ (~430 tokens)

- **service.go** (47 lines, ~430 tok) — Package gmailproxyservice implements a thin proxy for GmailConnectorService.

## services/gateway/ (~16.5K tokens)

4 files, ~16.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/graphservice/ (~7.2K tokens)

- **service.go** (492 lines, ~4.2K tok) — Package graphservice implements the GraphConnectorService gRPC server.
- **service_test.go** (379 lines, ~3.0K tok) — Go package: graphservice

## services/gateway/health/ (~10.1K tokens)

3 files, ~10.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/ingestservice/ (~31.3K tokens)

3 files, ~31.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/instructionservice/ (~3.2K tokens)

- **service.go** (361 lines, ~3.2K tok) — Package instructionservice implements the InstructionService gRPC server.

## services/gateway/internal/langfuse/ (~3.2K tokens)

- **client.go** (177 lines, ~1.7K tok) — Package langfuse provides a client for interacting with the Langfuse API.
- **client_test.go** (177 lines, ~1.5K tok) — Go package: langfuse

## services/gateway/ledgerservice/ (~3.7K tokens)

- **service.go** (436 lines, ~3.7K tok) — Package ledgerservice implements the LedgerService gRPC server.

## services/gateway/logsservice/ (~2.2K tokens)

- **service.go** (309 lines, ~2.2K tok) — Package logsservice implements the LogsService gRPC server.

## services/gateway/mentionsservice/ (~6.0K tokens)

- **service.go** (698 lines, ~6.0K tok) — Package mentionsservice implements the MentionsService gRPC server.

## services/gateway/middleware/ (~18.4K tokens)

4 files, ~18.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/modelservice/ (~16.6K tokens)

2 files, ~16.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/orchestrator/ (~10.5K tokens)

2 files, ~10.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/pipelineservice/ (~72.2K tokens)

21 files, ~72.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/productservice/ (~14.6K tokens)

1 files, ~14.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/projectservice/ (~4.3K tokens)

- **service.go** (477 lines, ~4.3K tok) — Package projectservice implements the ProjectService gRPC server.

## services/gateway/proxy/ (~13.2K tokens)

2 files, ~13.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/qualityservice/ (~7.6K tokens)

- **service.go** (277 lines, ~2.5K tok) — Package qualityservice implements the QualityService gRPC server.
- **service_test.go** (605 lines, ~5.1K tok) — Go package: qualityservice

## services/gateway/questionsservice/ (~5.4K tokens)

- **service.go** (603 lines, ~5.4K tok) — Package questionsservice implements the QuestionsService gRPC server.

## services/gateway/ratelimit/ (~13.2K tokens)

4 files, ~13.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/relationshipservice/ (~21.1K tokens)

3 files, ~21.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/reviewservice/ (~8.0K tokens)

- **service.go** (732 lines, ~6.6K tok) — Package reviewservice implements the ReviewService gRPC server.
- **service_test.go** (154 lines, ~1.4K tok) — Go package: reviewservice

## services/gateway/router/ (~13.6K tokens)

2 files, ~13.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/scheduleservice/ (~6.5K tokens)

- **service.go** (439 lines, ~4.2K tok) — Package scheduleservice implements the ScheduleService gRPC server.
- **service_test.go** (243 lines, ~2.3K tok) — Go package: scheduleservice

## services/gateway/searchservice/ (~17.8K tokens)

2 files, ~17.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/server/ (~15.3K tokens)

2 files, ~15.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/sourcemappingsservice/ (~1.8K tokens)

- **service.go** (194 lines, ~1.8K tok) — Package sourcemappingsservice implements the SourceMappingService gRPC server.

## services/gateway/teamsservice/ (~3.3K tokens)

- **service.go** (393 lines, ~3.3K tok) — Package teamsservice implements the TeamsService gRPC server.

## services/gateway/tenantservice/ (~2.0K tokens)

- **service.go** (265 lines, ~2.0K tok) — Package tenantservice implements the TenantService gRPC server.

## services/gateway/threadsservice/ (~6.1K tokens)

- **repository.go** (175 lines, ~1.2K tok) — Package threadsservice repository
- **service.go** (143 lines, ~1.3K tok) — Package threadsservice implements the ThreadsService gRPC server.
- **service_test.go** (410 lines, ~3.3K tok) — Go package: threadsservice
- **types.go** (44 lines, ~285 tok) — Package threadsservice types

## services/gateway/topicservice/ (~1.9K tokens)

- **service.go** (234 lines, ~1.9K tok) — Package topicservice implements the TopicService gRPC server.

## services/gateway/watchlistservice/ (~5.9K tokens)

- **service.go** (349 lines, ~3.2K tok) — Package watchlistservice implements the WatchListService gRPC server.
- **service_test.go** (360 lines, ~2.7K tok) — Go package: watchlistservice

## services/gateway/workflows/ (~28.9K tokens)

10 files, ~28.9K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gateway/workflowservice/ (~3.6K tokens)

- **service.go** (380 lines, ~3.6K tok) — Package workflowservice implements the WorkflowService gRPC server.

## services/gmail/attachment/ (~16.6K tokens)

3 files, ~16.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gmail/config/ (~5.1K tokens)

- **config.go** (164 lines, ~1.2K tok) — Package config provides Gmail Connector service-specific configuration.
- **config_test.go** (515 lines, ~3.9K tok) — Go package: config

## services/gmail/ (~5.8K tokens)

- **go.mod** (44 lines, ~426 tok) — go.mod in services/gmail/
- **go.sum** (93 lines, ~2.2K tok) — go.sum in services/gmail/
- **main.go** (383 lines, ~3.2K tok) — Package main is the entry point for the Gmail Connector gRPC service.

## services/gmail/oauth/ (~24.2K tokens)

6 files, ~24.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gmail/privacy/ (~19.2K tokens)

3 files, ~19.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gmail/push/ (~31.0K tokens)

7 files, ~31.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gmail/scheduler/ (~27.7K tokens)

6 files, ~27.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gmail/server/ (~7.0K tokens)

- **server.go** (529 lines, ~5.0K tok) — Package server provides the gRPC server implementation for the Gmail Connector service.
- **server_test.go** (216 lines, ~2.0K tok) — Go package: server

## services/gmail/sync/ (~27.5K tokens)

3 files, ~27.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/gmail/tests/ (~7.7K tokens)

- **integration_test.go** (954 lines, ~7.7K tok) — Package tests provides integration tests for the Gmail connector components.

## services/mcp/ (~31.3K tokens)

29 files, ~31.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/worker/activities/ (~539.3K tokens)

142 files, ~539.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/worker/config/ (~2.3K tokens)

- **config.go** (294 lines, ~2.3K tok) — Package config provides configuration for the Temporal worker service.

## services/worker/ (~22.5K tokens)

5 files, ~22.5K tokens — summarized. Use `cobuild scan --verbose` to expand.

## services/worker/integration/ (~4.8K tokens)

- **workflow_integration_test.go** (460 lines, ~4.8K tok) — Package integration provides integration tests for Temporal workflows.

## services/worker/workflows/ (~237.3K tokens)

57 files, ~237.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## skills/ (~733 tokens)

- **appointment-alert.md** (23 lines, ~165 tok) — Appointment Alert
- **healthcare-daily-summary.md** (38 lines, ~397 tok) — Healthcare Daily Summary
- **newsletter-rollup.md** (21 lines, ~171 tok) — Newsletter Weekly Rollup

## skills/decompose/ (~1.9K tokens)

- **decompose-design.md** (197 lines, ~1.9K tok) — Break a design into implementable tasks with dependency ordering and wave assignment. Trigger after the readiness gate passes and the pipeline advances to the decompose phase.

## skills/design/ (~1.7K tokens)

- **gate-readiness-review.md** (104 lines, ~1.1K tok) — Evaluate whether a design is ready for decomposition. Trigger on design review, readiness gate, or when a design reaches the design phase.
- **implementability.md** (47 lines, ~520 tok) — Check whether a design is specific enough for an agent to implement without asking questions. Called as part of the readiness review gate.

## skills/done/ (~653 tokens)

- **gate-retrospective.md** (106 lines, ~653 tok) — Review a completed pipeline to extract lessons learned and suggest improvements. Trigger when a design reaches the done phase.

## skills/implement/ (~1.7K tokens)

- **dispatch-task.md** (83 lines, ~732 tok) — Dispatch tasks to implementing agents and monitor until complete. Trigger when tasks are ready for implementation.
- **stall-check.md** (137 lines, ~985 tok) — Diagnose a task that may be stalled, crashed, or rate-limited. Trigger on health check, stall detection, or agent crash.

## skills/investigate/ (~1.3K tokens)

- **bug-investigation.md** (151 lines, ~1.3K tok) — Investigate a bug to identify root cause, affected areas, and produce a fix spec. Trigger when a bug enters the pipeline or when investigation is needed before implementation.

## skills/review/ (~2.2K tokens)

- **gate-process-review.md** (146 lines, ~1.2K tok) — Process external review feedback (Gemini, CI) on a task PR and decide approve, request-changes, or escalate. Trigger when a PR has external review results.
- **gate-review-pr.md** (73 lines, ~551 tok) — Review a pull request against its task spec and parent design. Trigger when an agent-based review is needed for a task PR.
- **merge-and-verify.md** (78 lines, ~484 tok) — Merge an approved PR, run post-merge tests, and auto-revert on failure. Trigger after a task PR is approved.

## skills/shared/ (~4.9K tokens)

- **create-design.md** (151 lines, ~1.3K tok) — Create a well-formed design work item that will pass the readiness review gate. Trigger on "create design", "write a design", "new design".
- **design-review.md** (127 lines, ~1.3K tok) — Review a design for pipeline readiness. Pre-flight check before submitting to CoBuild. Trigger on "review design", "design review", "is this ready".
- **playbook.md** (279 lines, ~2.3K tok) — Pipeline orchestration decision tree. Trigger when a pipeline event occurs — phase transition, gate result, task completion, or health check.

## specs/003-ai-coordination/ (~13.4K tokens)

4 files, ~13.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/003-ai-coordination/checklists/ (~293 tokens)

- **requirements.md** (34 lines, ~293 tok) — Specification Quality Checklist: Multi-Model AI Coordination

## specs/005-meeting-pipeline/ (~17.7K tokens)

7 files, ~17.7K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/005-meeting-pipeline/checklists/ (~552 tokens)

- **requirements.md** (48 lines, ~552 tok) — Specification Quality Checklist: Meeting Upload and Processing Pipeline

## specs/005-meeting-pipeline/contracts/ (~7.9K tokens)

- **api-spec.yaml** (797 lines, ~7.9K tok) — api-spec.yaml in specs/005-meeting-pipeline/contracts/

## specs/006-daily-review/checklists/ (~294 tokens)

- **requirements.md** (34 lines, ~294 tok) — Specification Quality Checklist: Daily Review Workflow Interface

## specs/006-daily-review/contracts/ (~3.2K tokens)

- **cli-api.md** (579 lines, ~3.2K tok) — CLI API Contracts: Daily Review Workflow

## specs/006-daily-review/ (~11.4K tokens)

5 files, ~11.4K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/007-search-interface/ (~17.6K tokens)

6 files, ~17.6K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/007-search-interface/checklists/ (~496 tokens)

- **requirements.md** (56 lines, ~496 tok) — Specification Quality Checklist: Search and Query Interface

## specs/007-search-interface/contracts/ (~7.1K tokens)

- **query-schema.json** (153 lines, ~1.3K tok) — query-schema.json in specs/007-search-interface/contracts/
- **search-api.yaml** (688 lines, ~5.7K tok) — search-api.yaml in specs/007-search-interface/contracts/

## specs/008-automation-engine/checklists/ (~292 tokens)

- **requirements.md** (34 lines, ~292 tok) — Specification Quality Checklist: Automation Rules Engine

## specs/008-automation-engine/contracts/ (~8.1K tokens)

- **automation-api.yaml** (665 lines, ~5.8K tok) — Automation Rules Engine - CLI API Contract
- **events.md** (342 lines, ~2.4K tok) — Automation Engine Events Contract

## specs/008-automation-engine/ (~14.0K tokens)

5 files, ~14.0K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/009-relationship-discovery-and-management/archive/ (~1.2K tokens)

- **lessons-learned.md** (112 lines, ~1.2K tok) — Lessons Learned: 009-Relationship-Discovery-and-Management

## specs/009-relationship-discovery-and-management/checklists/ (~295 tokens)

- **requirements.md** (34 lines, ~295 tok) — Specification Quality Checklist: Relationship Discovery and Management

## specs/009-relationship-discovery-and-management/contracts/ (~6.7K tokens)

- **relationship-api.yaml** (482 lines, ~4.0K tok) — relationship-api.yaml in specs/009-relationship-discovery-and-management/contracts/
- **relationship-events.yaml** (301 lines, ~2.7K tok) — Event Contracts for Relationship Discovery and Management

## specs/009-relationship-discovery-and-management/ (~13.3K tokens)

5 files, ~13.3K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/010-testing-framework/ (~17.1K tokens)

5 files, ~17.1K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/011-observability-framework/ (~19.8K tokens)

7 files, ~19.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/011-observability-framework/checklists/ (~458 tokens)

- **requirements.md** (45 lines, ~458 tok) — Specification Quality Checklist: Penfold Production Agent Observability

## specs/011-observability-framework/contracts/ (~13.8K tokens)

2 files, ~13.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/020-slm-llm-architecture/ (~76.9K tokens)

16 files, ~76.9K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/020-slm-llm-architecture/feedback/ (~15.2K tokens)

4 files, ~15.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## specs/020-slm-llm-architecture/feedback2/ (~3.1K tokens)

- **architecture-review-2026-02-04.md** (152 lines, ~3.1K tok) — SLM/LLM Architecture Review

## tests/benchmark/ (~4.5K tokens)

- **helpers.go** (254 lines, ~2.0K tok) — Package benchmark provides benchmarking tests for Penfold components.
- **llm_test.go** (326 lines, ~2.5K tok) — Go package: benchmark

## tests/e2e/ (~205.2K tokens)

64 files, ~205.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## tests/ (~4.4K tokens)

- **go.mod** (90 lines, ~1.2K tok) — go.mod in tests/
- **go.sum** (134 lines, ~3.3K tok) — go.sum in tests/

## tests/integration/ (~122.2K tokens)

38 files, ~122.2K tokens — summarized. Use `cobuild scan --verbose` to expand.

## tests/integration/testdata/certs/ (~6.4K tokens)

- **expired-client.crt** (22 lines, ~338 tok) — expired-client.crt in tests/integration/testdata/certs/
- **expired-client.key** (29 lines, ~454 tok) — expired-client.key in tests/integration/testdata/certs/
- **generate.sh** (178 lines, ~1.6K tok) — generate.sh in tests/integration/testdata/certs/
- **server.crt** (24 lines, ~365 tok) — server.crt in tests/integration/testdata/certs/
- **server.key** (29 lines, ~454 tok) — server.key in tests/integration/testdata/certs/
- **test-ca.crt** (22 lines, ~332 tok) — test-ca.crt in tests/integration/testdata/certs/
- **test-ca.key** (29 lines, ~454 tok) — test-ca.key in tests/integration/testdata/certs/
- **valid-client.crt** (22 lines, ~343 tok) — valid-client.crt in tests/integration/testdata/certs/
- **valid-client.key** (29 lines, ~455 tok) — valid-client.key in tests/integration/testdata/certs/
- **wrong-ca-client.crt** (22 lines, ~342 tok) — wrong-ca-client.crt in tests/integration/testdata/certs/
- **wrong-ca-client.key** (29 lines, ~454 tok) — wrong-ca-client.key in tests/integration/testdata/certs/
- **wrong-ca.crt** (22 lines, ~330 tok) — wrong-ca.crt in tests/integration/testdata/certs/
- **wrong-ca.key** (29 lines, ~455 tok) — wrong-ca.key in tests/integration/testdata/certs/

## tests/live/ (~3.1K tokens)

- **gemini_test.go** (158 lines, ~1.2K tok) — Go package: live
- **gmail_test.go** (189 lines, ~1.5K tok) — Go package: live
- **helpers.go** (60 lines, ~392 tok) — Package live provides helpers for live API tests.

## tests/quality/ (~37.8K tokens)

16 files, ~37.8K tokens — summarized. Use `cobuild scan --verbose` to expand.

## tests/quality/fixtures/project_classification/ (~1.7K tokens)

- **pc-001-alpha-status.yaml** (7 lines, ~79 tok) — pc-001-alpha-status.yaml in tests/quality/fixtures/project_classification/
- **pc-002-mobile-design.yaml** (7 lines, ~81 tok) — pc-002-mobile-design.yaml in tests/quality/fixtures/project_classification/
- **pc-003-security-findings.yaml** (7 lines, ~87 tok) — pc-003-security-findings.yaml in tests/quality/fixtures/project_classification/
- **pc-004-docs-request.yaml** (7 lines, ~83 tok) — pc-004-docs-request.yaml in tests/quality/fixtures/project_classification/
- **pc-005-crm-update.yaml** (7 lines, ~85 tok) — pc-005-crm-update.yaml in tests/quality/fixtures/project_classification/
- **pc-006-alpha-api-dependency.yaml** (8 lines, ~88 tok) — pc-006-alpha-api-dependency.yaml in tests/quality/fixtures/project_classification/
- **pc-007-ml-data-dependency.yaml** (8 lines, ~104 tok) — pc-007-ml-data-dependency.yaml in tests/quality/fixtures/project_classification/
- **pc-008-portal-enterprise.yaml** (8 lines, ~98 tok) — pc-008-portal-enterprise.yaml in tests/quality/fixtures/project_classification/
- **pc-009-crm-enterprise-sync.yaml** (8 lines, ~101 tok) — pc-009-crm-enterprise-sync.yaml in tests/quality/fixtures/project_classification/
- **pc-010-alpha-mobile-scope.yaml** (8 lines, ~99 tok) — pc-010-alpha-mobile-scope.yaml in tests/quality/fixtures/project_classification/
- **pc-011-lunch-invite.yaml** (6 lines, ~66 tok) — pc-011-lunch-invite.yaml in tests/quality/fixtures/project_classification/
- **pc-012-all-hands.yaml** (6 lines, ~68 tok) — pc-012-all-hands.yaml in tests/quality/fixtures/project_classification/
- **pc-013-ooo-reply.yaml** (6 lines, ~68 tok) — pc-013-ooo-reply.yaml in tests/quality/fixtures/project_classification/
- **pc-014-generic-update.yaml** (6 lines, ~72 tok) — pc-014-generic-update.yaml in tests/quality/fixtures/project_classification/
- **pc-015-hr-benefits.yaml** (6 lines, ~72 tok) — pc-015-hr-benefits.yaml in tests/quality/fixtures/project_classification/
- **pc-016-short-subject-only.yaml** (7 lines, ~77 tok) — pc-016-short-subject-only.yaml in tests/quality/fixtures/project_classification/
- **pc-017-reply-thread.yaml** (7 lines, ~76 tok) — pc-017-reply-thread.yaml in tests/quality/fixtures/project_classification/
- **pc-018-ambiguous-generic.yaml** (6 lines, ~87 tok) — pc-018-ambiguous-generic.yaml in tests/quality/fixtures/project_classification/
- **pc-019-channel-mapped-alpha.yaml** (9 lines, ~121 tok) — pc-019-channel-mapped-alpha.yaml in tests/quality/fixtures/project_classification/
- **pc-020-channel-mapped-crm.yaml** (9 lines, ~124 tok) — pc-020-channel-mapped-crm.yaml in tests/quality/fixtures/project_classification/

## tests/quality/golden/ (~1.5K tokens)

- **002-incident-response.yaml** (36 lines, ~301 tok) — 002-incident-response.yaml in tests/quality/golden/
- **011-risk-escalation.yaml** (47 lines, ~487 tok) — 011-risk-escalation.yaml in tests/quality/golden/
- **012-low-priority-fyi.yaml** (28 lines, ~239 tok) — 012-low-priority-fyi.yaml in tests/quality/golden/
- **013-thread-with-decisions.yaml** (44 lines, ~433 tok) — 013-thread-with-decisions.yaml in tests/quality/golden/

## tests/quality/golden/newsletter/ (~1.8K tokens)

- **001-ctg-post-its.yaml** (51 lines, ~360 tok) — 001-ctg-post-its.yaml in tests/quality/golden/newsletter/
- **002-akamai-wave.yaml** (51 lines, ~348 tok) — 002-akamai-wave.yaml in tests/quality/golden/newsletter/
- **003-emea-newsletter.yaml** (51 lines, ~350 tok) — 003-emea-newsletter.yaml in tests/quality/golden/newsletter/
- **004-dynamic-signal.yaml** (34 lines, ~246 tok) — 004-dynamic-signal.yaml in tests/quality/golden/newsletter/
- **005-eng-learning.yaml** (19 lines, ~146 tok) — 005-eng-learning.yaml in tests/quality/golden/newsletter/
- **006-spark-wellness.yaml** (51 lines, ~361 tok) — 006-spark-wellness.yaml in tests/quality/golden/newsletter/

## tests/quality/golden/notification/ (~2.1K tokens)

- **001-aha-daily-todos.yaml** (27 lines, ~238 tok) — 001-aha-daily-todos.yaml in tests/quality/golden/notification/
- **002-aha-digest-compute.yaml** (27 lines, ~226 tok) — 002-aha-digest-compute.yaml in tests/quality/golden/notification/
- **003-aha-digest-compute-2.yaml** (27 lines, ~217 tok) — 003-aha-digest-compute-2.yaml in tests/quality/golden/notification/
- **004-jira-track-updates.yaml** (27 lines, ~230 tok) — 004-jira-track-updates.yaml in tests/quality/golden/notification/
- **005-oracle-antibribery.yaml** (26 lines, ~249 tok) — 005-oracle-antibribery.yaml in tests/quality/golden/notification/
- **006-google-signin-alert.yaml** (27 lines, ~248 tok) — 006-google-signin-alert.yaml in tests/quality/golden/notification/
- **007-globalsecops-malicious-dns.yaml** (26 lines, ~233 tok) — 007-globalsecops-malicious-dns.yaml in tests/quality/golden/notification/
- **008-bitmovin-action-required.yaml** (27 lines, ~240 tok) — 008-bitmovin-action-required.yaml in tests/quality/golden/notification/
- **009-internal-a360-cleanup.yaml** (27 lines, ~230 tok) — 009-internal-a360-cleanup.yaml in tests/quality/golden/notification/

## tests/quality/golden/standard/ (~4.4K tokens)

- **001-project-update.yaml** (35 lines, ~243 tok) — 001-project-update.yaml in tests/quality/golden/standard/
- **002-incident-response.yaml** (39 lines, ~283 tok) — 002-incident-response.yaml in tests/quality/golden/standard/
- **005-project-kickoff.yaml** (38 lines, ~286 tok) — 005-project-kickoff.yaml in tests/quality/golden/standard/
- **008-security-review.yaml** (36 lines, ~257 tok) — 008-security-review.yaml in tests/quality/golden/standard/
- **010-postmortem.yaml** (37 lines, ~268 tok) — 010-postmortem.yaml in tests/quality/golden/standard/
- **011-risk-escalation.yaml** (42 lines, ~334 tok) — 011-risk-escalation.yaml in tests/quality/golden/standard/
- **012-low-priority-fyi.yaml** (33 lines, ~245 tok) — 012-low-priority-fyi.yaml in tests/quality/golden/standard/
- **013-thread-with-decisions.yaml** (47 lines, ~374 tok) — 013-thread-with-decisions.yaml in tests/quality/golden/standard/
- **personal_amazon_delivery_notification.yaml** (25 lines, ~195 tok) — personal_amazon_delivery_notification.yaml in tests/quality/golden/standard/
- **personal_anthropic_api_update.yaml** (25 lines, ~187 tok) — personal_anthropic_api_update.yaml in tests/quality/golden/standard/
- **personal_ayla_school_update.yaml** (25 lines, ~206 tok) — personal_ayla_school_update.yaml in tests/quality/golden/standard/
- **personal_dental_appointment.yaml** (27 lines, ~218 tok) — personal_dental_appointment.yaml in tests/quality/golden/standard/
- **personal_linkedin_job_alert.yaml** (25 lines, ~198 tok) — personal_linkedin_job_alert.yaml in tests/quality/golden/standard/
- **personal_nhs_prescription_renewal.yaml** (25 lines, ~210 tok) — personal_nhs_prescription_renewal.yaml in tests/quality/golden/standard/
- **personal_porsche_service_booking.yaml** (27 lines, ~232 tok) — personal_porsche_service_booking.yaml in tests/quality/golden/standard/
- **personal_spring_jackets_promo.yaml** (23 lines, ~181 tok) — personal_spring_jackets_promo.yaml in tests/quality/golden/standard/
- **personal_substack_political_article.yaml** (26 lines, ~224 tok) — personal_substack_political_article.yaml in tests/quality/golden/standard/
- **personal_webuyanycar_appointment.yaml** (27 lines, ~225 tok) — personal_webuyanycar_appointment.yaml in tests/quality/golden/standard/

---

1630 files, ~4313.4K tokens total
