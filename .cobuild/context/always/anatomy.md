# Project Anatomy

Auto-generated file index. Use this to understand the codebase without reading every file.
Token estimates help you decide what's worth reading vs what you can skip.

## Root (~60.6K tokens)

- **.cobuild.yaml** (5 lines, ~46 tok) — Multi-repo: designs may span penf-cli and penfold
- **AGENTS.md** (169 lines, ~1.7K tok) — Agent Instructions
- **ARCHITECTURE.md** (69 lines, ~517 tok) — Penfold Architecture
- **CLAUDE.md** (39 lines, ~290 tok) — Penfold Backend
- **Makefile** (104 lines, ~735 tok) — Makefile
- **README.md** (78 lines, ~527 tok) — Penfold
- **context-palace.md** (624 lines, ~4.2K tok) — Context-Palace
- **go.work** (43 lines, ~196 tok) — go.work
- **go.work.sum** (1494 lines, ~38.8K tok) — go.work.sum
- **observability_schema.sql** (608 lines, ~6.8K tok) — =====================================================
- **palace-sync-docs** (116 lines, ~874 tok) — palace-sync-docs
- **pf-rules.md** (113 lines, ~668 tok) — pf-rules.md - Context-Palace Rules for penfold
- **preferences.md** (86 lines, ~523 tok) — Penfold User Preferences
- **processes.md** (53 lines, ~340 tok) — Penfold Processes
- **project-constitution.md** (346 lines, ~3.3K tok) — Penfold Project Constitution
- **project_review.md** (40 lines, ~1.1K tok) — Project Review: Penfold

## .cobuild/ (~3.5K tokens)

- **AGENTS.md** (105 lines, ~1.1K tok) — CoBuild Pipeline Instructions
- **last-prompt.md** (156 lines, ~2.1K tok) — Task: Fix: extend PreClassifyContent to detect newsletter senders for early pipeline resolution
- **pipeline.yaml** (40 lines, ~326 tok) — penfold — repo-specific pipeline config
- **session_id** (2 lines, ~0 tok) — session_id in .cobuild/

## .cxp/context/ (~1.7K tokens)

- **agent-identity.md** (20 lines, ~113 tok) — Mycroft — Penfold Backend Developer
- **architecture.md** (38 lines, ~627 tok) — Architectural Principles — Hard Constraints
- **completion-protocol.md** (30 lines, ~283 tok) — Completion Protocol
- **deploy.md** (26 lines, ~255 tok) — Deploying
- **dispatch-completion.md** (27 lines, ~244 tok) — Dispatched Task — Completion Instructions
- **interactive-menu.md** (23 lines, ~156 tok) — Startup Instructions

## .github/workflows/ (~8.9K tokens)

- **auto-release.yml** (61 lines, ~648 tok) — Triggers when VERSION file changes on main branch
- **ci.yml** (306 lines, ~2.8K tok) — ci.yml in .github/workflows/
- **deploy-verify.yml** (249 lines, ~3.0K tok) — deploy-verify.yml in .github/workflows/
- **proto.yml** (61 lines, ~646 tok) — proto.yml in .github/workflows/
- **release.yml** (174 lines, ~1.8K tok) — release.yml in .github/workflows/

## .specify/memory/ (~1.8K tokens)

- **constitution.md** (114 lines, ~1.8K tok) — constitution.md in .specify/memory/

## .specify/scripts/bash/ (~13.5K tokens)

- **check-prerequisites.sh** (167 lines, ~1.4K tok) — check-prerequisites.sh in .specify/scripts/bash/
- **common.sh** (157 lines, ~1.4K tok) — common.sh in .specify/scripts/bash/
- **create-new-feature.sh** (298 lines, ~2.9K tok) — create-new-feature.sh in .specify/scripts/bash/
- **setup-plan.sh** (62 lines, ~464 tok) — setup-plan.sh in .specify/scripts/bash/
- **update-agent-context.sh** (800 lines, ~7.3K tok) — update-agent-context.sh in .specify/scripts/bash/

## .specify/templates/ (~4.6K tokens)

- **agent-file-template.md** (29 lines, ~116 tok) — [PROJECT NAME] Development Guidelines
- **checklist-template.md** (41 lines, ~328 tok) — [CHECKLIST TYPE] Checklist: [FEATURE NAME]
- **plan-template.md** (105 lines, ~917 tok) — Implementation Plan: [FEATURE]
- **spec-template.md** (116 lines, ~990 tok) — Feature Specification: [FEATURE NAME]
- **tasks-template.md** (252 lines, ~2.3K tok) — "Task list template for feature implementation"

## api/proto/ai/v1/ (~117.6K tokens)

- **ai.pb.go** (7382 lines, ~89.8K tok) — ai.pb.go in api/proto/ai/v1/
- **ai.proto** (1522 lines, ~13.4K tok) — ai.proto in api/proto/ai/v1/
- **ai_grpc.pb.go** (1024 lines, ~13.8K tok) — ai_grpc.pb.go in api/proto/ai/v1/
- **go.mod** (18 lines, ~125 tok) — go.mod in api/proto/ai/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/ai/v1/

## api/proto/alert/v1/ (~7.7K tokens)

- **alert.pb.go** (500 lines, ~5.7K tok) — Go package: alertv1
- **alert.proto** (46 lines, ~255 tok) — alert.proto in api/proto/alert/v1/
- **alert_grpc.pb.go** (160 lines, ~1.8K tok) — Go package: alertv1

## api/proto/assertions/v1/ (~17.4K tokens)

- **assertions.pb.go** (1044 lines, ~13.4K tok) — Go package: assertionsv1
- **assertions.proto** (130 lines, ~1.3K tok) — assertions.proto in api/proto/assertions/v1/
- **assertions_grpc.pb.go** (211 lines, ~2.6K tok) — Go package: assertionsv1

## api/proto/audit/v1/ (~34.6K tokens)

- **audit.pb.go** (2418 lines, ~27.5K tok) — Go package: auditv1
- **audit.proto** (476 lines, ~3.1K tok) — audit.proto in api/proto/audit/v1/
- **audit_grpc.pb.go** (331 lines, ~4.0K tok) — Go package: auditv1

## api/proto/bridge/v1/ (~8.8K tokens)

- **bridge.pb.go** (517 lines, ~6.2K tok) — Go package: bridgev1
- **bridge.proto** (45 lines, ~278 tok) — bridge.proto in api/proto/bridge/v1/
- **bridge_grpc.pb.go** (198 lines, ~2.3K tok) — Go package: bridgev1

## api/proto/ (~770 tokens)

- **buf.gen.yaml** (14 lines, ~107 tok) — buf.gen.yaml in api/proto/
- **buf.yaml** (16 lines, ~94 tok) — buf.yaml in api/proto/
- **go.mod** (18 lines, ~124 tok) — go.mod in api/proto/
- **go.sum** (21 lines, ~445 tok) — go.sum in api/proto/

## api/proto/classify/v1/ (~9.9K tokens)

- **classify.pb.go** (530 lines, ~6.5K tok) — Go package: classifyv1
- **classify.proto** (71 lines, ~613 tok) — classify.proto in api/proto/classify/v1/
- **classify_grpc.pb.go** (214 lines, ~2.8K tok) — Go package: classifyv1

## api/proto/cli/v1/ (~44.0K tokens)

- **cli.pb.go** (3032 lines, ~34.5K tok) — cli.pb.go in api/proto/cli/v1/
- **cli.proto** (623 lines, ~4.4K tok) — cli.proto in api/proto/cli/v1/
- **cli_grpc.pb.go** (391 lines, ~4.6K tok) — cli_grpc.pb.go in api/proto/cli/v1/
- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/cli/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/cli/v1/

## api/proto/common/v1/ (~7.8K tokens)

- **common.pb.go** (613 lines, ~6.6K tok) — Go package: commonv1
- **common.proto** (126 lines, ~1.2K tok) — common.proto in api/proto/common/v1/
- **go.mod** (6 lines, ~31 tok) — go.mod in api/proto/common/v1/
- **go.sum** (3 lines, ~43 tok) — go.sum in api/proto/common/v1/

## api/proto/connectors/v1/ (~858 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/connectors/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/connectors/v1/
- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/connectors/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/connectors/v1/

## api/proto/connectors/v1/entitypb/ (~26.3K tokens)

- **entity.pb.go** (1832 lines, ~21.4K tok) — Go package: entityv1
- **entity.proto** (339 lines, ~2.3K tok) — entity.proto in api/proto/connectors/v1/entitypb/
- **entity_grpc.pb.go** (213 lines, ~2.6K tok) — Go package: entityv1

## api/proto/connectors/v1/gmailpb/ (~30.2K tokens)

- **gmail.pb.go** (1930 lines, ~23.4K tok) — Go package: gmailv1
- **gmail.proto** (414 lines, ~3.1K tok) — gmail.proto in api/proto/connectors/v1/gmailpb/
- **gmail_grpc.pb.go** (299 lines, ~3.8K tok) — Go package: gmailv1

## api/proto/connectors/v1/graphpb/ (~15.6K tokens)

- **graph.pb.go** (867 lines, ~10.8K tok) — Go package: graphv1
- **graph.proto** (161 lines, ~1.4K tok) — graph.proto in api/proto/connectors/v1/graphpb/
- **graph_grpc.pb.go** (253 lines, ~3.4K tok) — Go package: graphv1

## api/proto/content/v1/ (~93.6K tokens)

- **content.pb.go** (5491 lines, ~70.5K tok) — Go package: contentv1
- **content.proto** (1024 lines, ~8.7K tok) — content.proto in api/proto/content/v1/
- **content_enum_test.go** (162 lines, ~1.7K tok) — Go package: contentv1_test
- **content_grpc.pb.go** (870 lines, ~12.2K tok) — Go package: contentv1
- **go.mod** (18 lines, ~127 tok) — go.mod in api/proto/content/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/content/v1/

## api/proto/conversation/v1/ (~35.9K tokens)

- **conversation.pb.go** (2142 lines, ~28.5K tok) — Go package: conversationv1
- **conversation.proto** (252 lines, ~2.4K tok) — conversation.proto in api/proto/conversation/v1/
- **conversation_grpc.pb.go** (371 lines, ~5.0K tok) — Go package: conversationv1

## api/proto/core/v1/ (~856 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/core/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/core/v1/
- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/core/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/core/v1/

## api/proto/core/v1/clipb/ (~45.4K tokens)

- **cli.pb.go** (3129 lines, ~36.3K tok) — cli.pb.go in api/proto/core/v1/clipb/
- **cli.proto** (644 lines, ~4.5K tok) — cli.proto in api/proto/core/v1/clipb/
- **cli_grpc.pb.go** (391 lines, ~4.6K tok) — cli_grpc.pb.go in api/proto/core/v1/clipb/

## api/proto/core/v1/commonpb/ (~7.9K tokens)

- **common.pb.go** (616 lines, ~6.8K tok) — Go package: commonv1
- **common.proto** (126 lines, ~1.2K tok) — common.proto in api/proto/core/v1/commonpb/

## api/proto/core/v1/gatewaypb/ (~32.2K tokens)

- **gateway.pb.go** (2109 lines, ~25.9K tok) — gateway.pb.go in api/proto/core/v1/gatewaypb/
- **gateway.proto** (426 lines, ~3.1K tok) — gateway.proto in api/proto/core/v1/gatewaypb/
- **gateway_grpc.pb.go** (262 lines, ~3.2K tok) — gateway_grpc.pb.go in api/proto/core/v1/gatewaypb/

## api/proto/digest/v1/ (~14.3K tokens)

- **digest.pb.go** (953 lines, ~11.0K tok) — Go package: digestv1
- **digest.proto** (87 lines, ~528 tok) — digest.proto in api/proto/digest/v1/
- **digest_grpc.pb.go** (236 lines, ~2.7K tok) — Go package: digestv1

## api/proto/entity/v1/ (~74.5K tokens)

- **entity.pb.go** (4759 lines, ~54.9K tok) — Go package: entityv1
- **entity.proto** (802 lines, ~5.5K tok) — entity.proto in api/proto/entity/v1/
- **entity_grpc.pb.go** (1044 lines, ~14.1K tok) — Go package: entityv1

## api/proto/gateway/v1/ (~32.0K tokens)

- **gateway.pb.go** (2097 lines, ~25.1K tok) — gateway.pb.go in api/proto/gateway/v1/
- **gateway.proto** (426 lines, ~3.1K tok) — gateway.proto in api/proto/gateway/v1/
- **gateway_grpc.pb.go** (262 lines, ~3.2K tok) — gateway_grpc.pb.go in api/proto/gateway/v1/
- **go.mod** (18 lines, ~127 tok) — go.mod in api/proto/gateway/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/gateway/v1/

## api/proto/glossary/v1/ (~30.1K tokens)

- **glossary.pb.go** (1959 lines, ~21.8K tok) — Go package: glossaryv1
- **glossary.proto** (360 lines, ~2.4K tok) — glossary.proto in api/proto/glossary/v1/
- **glossary_grpc.pb.go** (493 lines, ~6.0K tok) — Go package: glossaryv1

## api/proto/gmail/v1/ (~29.6K tokens)

- **gmail.pb.go** (1909 lines, ~22.2K tok) — Go package: gmailv1
- **gmail.proto** (414 lines, ~3.1K tok) — gmail.proto in api/proto/gmail/v1/
- **gmail_grpc.pb.go** (299 lines, ~3.7K tok) — Go package: gmailv1
- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/gmail/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/gmail/v1/

## api/proto/ingest/v1/ (~77.9K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/ingest/v1/
- **go.sum** (29 lines, ~626 tok) — go.sum in api/proto/ingest/v1/
- **ingest.pb.go** (5117 lines, ~60.0K tok) — Go package: ingestv1
- **ingest.proto** (957 lines, ~6.3K tok) — ingest.proto in api/proto/ingest/v1/
- **ingest_grpc.pb.go** (851 lines, ~10.8K tok) — Go package: ingestv1

## api/proto/instruction/v1/ (~25.3K tokens)

- **instruction.pb.go** (1566 lines, ~19.3K tok) — Go package: instructionv1
- **instruction.proto** (103 lines, ~868 tok) — instruction.proto in api/proto/instruction/v1/
- **instruction_grpc.pb.go** (388 lines, ~5.1K tok) — Go package: instructionv1

## api/proto/intelligence/v1/aipb/ (~28.5K tokens)

- **ai.pb.go** (1700 lines, ~21.2K tok) — ai.pb.go in api/proto/intelligence/v1/aipb/
- **ai.proto** (381 lines, ~3.3K tok) — ai.proto in api/proto/intelligence/v1/aipb/
- **ai_grpc.pb.go** (306 lines, ~4.0K tok) — ai_grpc.pb.go in api/proto/intelligence/v1/aipb/

## api/proto/intelligence/v1/ (~858 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/intelligence/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/intelligence/v1/
- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/intelligence/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/intelligence/v1/

## api/proto/intelligence/v1/glossarypb/ (~31.5K tokens)

- **glossary.pb.go** (1957 lines, ~23.1K tok) — Go package: glossaryv1
- **glossary.proto** (349 lines, ~2.4K tok) — glossary.proto in api/proto/intelligence/v1/glossarypb/
- **glossary_grpc.pb.go** (493 lines, ~6.0K tok) — Go package: glossaryv1

## api/proto/intelligence/v1/mentionspb/ (~47.7K tokens)

- **mentions.pb.go** (3001 lines, ~39.3K tok) — Go package: mentionsv1
- **mentions.proto** (351 lines, ~2.5K tok) — mentions.proto in api/proto/intelligence/v1/mentionspb/
- **mentions_grpc.pb.go** (457 lines, ~5.9K tok) — Go package: mentionsv1

## api/proto/intelligence/v1/questionspb/ (~32.5K tokens)

- **questions.pb.go** (1899 lines, ~24.6K tok) — Go package: questionsv1
- **questions.proto** (346 lines, ~2.5K tok) — questions.proto in api/proto/intelligence/v1/questionspb/
- **questions_grpc.pb.go** (419 lines, ~5.4K tok) — Go package: questionsv1

## api/proto/intelligence/v1/relationshippb/ (~40.6K tokens)

- **relationship.pb.go** (2374 lines, ~32.9K tok) — Go package: relationshipv1
- **relationship.proto** (486 lines, ~3.7K tok) — relationship.proto in api/proto/intelligence/v1/relationshippb/
- **relationship_grpc.pb.go** (299 lines, ~4.0K tok) — Go package: relationshipv1

## api/proto/intelligence/v1/searchpb/ (~31.0K tokens)

- **search.pb.go** (1765 lines, ~23.5K tok) — Go package: searchv1
- **search.proto** (378 lines, ~3.2K tok) — search.proto in api/proto/intelligence/v1/searchpb/
- **search_grpc.pb.go** (345 lines, ~4.3K tok) — Go package: searchv1

## api/proto/ledger/v1/ (~31.3K tokens)

- **ledger.pb.go** (1995 lines, ~23.8K tok) — Go package: ledgerv1
- **ledger.proto** (220 lines, ~2.0K tok) — ledger.proto in api/proto/ledger/v1/
- **ledger_grpc.pb.go** (451 lines, ~5.5K tok) — Go package: ledgerv1

## api/proto/logs/v1/ (~19.8K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/logs/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/logs/v1/
- **logs.pb.go** (1245 lines, ~14.3K tok) — logs.pb.go in api/proto/logs/v1/
- **logs.proto** (219 lines, ~1.5K tok) — logs.proto in api/proto/logs/v1/
- **logs_grpc.pb.go** (296 lines, ~3.5K tok) — logs_grpc.pb.go in api/proto/logs/v1/

## api/proto/mentions/v1/ (~47.2K tokens)

- **mentions.pb.go** (3075 lines, ~38.7K tok) — Go package: mentionsv1
- **mentions.proto** (371 lines, ~2.7K tok) — mentions.proto in api/proto/mentions/v1/
- **mentions_grpc.pb.go** (457 lines, ~5.8K tok) — Go package: mentionsv1

## api/proto/orchestrator/v1/ (~32.7K tokens)

- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/orchestrator/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/orchestrator/v1/
- **orchestrator.pb.go** (1898 lines, ~23.9K tok) — Go package: orchestratorv1
- **orchestrator.proto** (362 lines, ~3.0K tok) — orchestrator.proto in api/proto/orchestrator/v1/
- **orchestrator_grpc.pb.go** (417 lines, ~5.3K tok) — Go package: orchestratorv1

## api/proto/pipeline/v1/ (~157.4K tokens)

- **pipeline.pb.go** (9938 lines, ~117.6K tok) — Go package: pipelinev1
- **pipeline.proto** (1778 lines, ~13.5K tok) — pipeline.proto in api/proto/pipeline/v1/
- **pipeline_grpc.pb.go** (1933 lines, ~26.3K tok) — Go package: pipelinev1

## api/proto/processing/v1/ (~858 tokens)

- **buf.gen.yaml** (11 lines, ~69 tok) — buf.gen.yaml in api/proto/processing/v1/
- **buf.yaml** (14 lines, ~78 tok) — buf.yaml in api/proto/processing/v1/
- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/processing/v1/
- **go.sum** (27 lines, ~583 tok) — go.sum in api/proto/processing/v1/

## api/proto/processing/v1/contentpb/ (~30.2K tokens)

- **content.pb.go** (1659 lines, ~22.8K tok) — Go package: contentv1
- **content.proto** (354 lines, ~3.2K tok) — content.proto in api/proto/processing/v1/contentpb/
- **content_grpc.pb.go** (306 lines, ~4.2K tok) — Go package: contentv1

## api/proto/processing/v1/orchestratorpb/ (~33.9K tokens)

- **orchestrator.pb.go** (1932 lines, ~25.6K tok) — Go package: orchestratorv1
- **orchestrator.proto** (362 lines, ~3.0K tok) — orchestrator.proto in api/proto/processing/v1/orchestratorpb/
- **orchestrator_grpc.pb.go** (417 lines, ~5.4K tok) — Go package: orchestratorv1

## api/proto/processing/v1/reviewpb/ (~36.6K tokens)

- **review.pb.go** (2202 lines, ~28.5K tok) — Go package: reviewv1
- **review.proto** (416 lines, ~3.2K tok) — review.proto in api/proto/processing/v1/reviewpb/
- **review_grpc.pb.go** (387 lines, ~4.9K tok) — Go package: reviewv1

## api/proto/processing/v1/workflowpb/ (~17.9K tokens)

- **workflow.pb.go** (1389 lines, ~16.1K tok) — Go package: workflowv1
- **workflow.proto** (247 lines, ~1.8K tok) — workflow.proto in api/proto/processing/v1/workflowpb/

## api/proto/product/v1/ (~93.9K tokens)

- **product.pb.go** (6135 lines, ~72.3K tok) — Go package: productv1
- **product.proto** (1083 lines, ~7.4K tok) — product.proto in api/proto/product/v1/
- **product_grpc.pb.go** (1097 lines, ~14.1K tok) — Go package: productv1

## api/proto/project/v1/ (~29.0K tokens)

- **project.pb.go** (1815 lines, ~21.0K tok) — Go package: projectv1
- **project.proto** (329 lines, ~2.4K tok) — project.proto in api/proto/project/v1/
- **project_grpc.pb.go** (453 lines, ~5.7K tok) — Go package: projectv1

## api/proto/quality/v1/ (~12.7K tokens)

- **quality.pb.go** (756 lines, ~8.8K tok) — Go package: qualityv1
- **quality.proto** (141 lines, ~1.2K tok) — quality.proto in api/proto/quality/v1/
- **quality_grpc.pb.go** (213 lines, ~2.7K tok) — Go package: qualityv1

## api/proto/questions/v1/ (~31.3K tokens)

- **questions.pb.go** (1904 lines, ~23.4K tok) — Go package: questionsv1
- **questions.proto** (353 lines, ~2.5K tok) — questions.proto in api/proto/questions/v1/
- **questions_grpc.pb.go** (419 lines, ~5.4K tok) — Go package: questionsv1

## api/proto/relationship/v1/ (~86.3K tokens)

- **go.mod** (18 lines, ~128 tok) — go.mod in api/proto/relationship/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/relationship/v1/
- **relationship.pb.go** (5158 lines, ~66.5K tok) — Go package: relationshipv1
- **relationship.proto** (1011 lines, ~7.6K tok) — relationship.proto in api/proto/relationship/v1/
- **relationship_grpc.pb.go** (859 lines, ~11.6K tok) — Go package: relationshipv1

## api/proto/review/v1/ (~52.2K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/review/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/review/v1/
- **review.pb.go** (3182 lines, ~39.0K tok) — Go package: reviewv1
- **review.proto** (595 lines, ~4.7K tok) — review.proto in api/proto/review/v1/
- **review_grpc.pb.go** (631 lines, ~8.0K tok) — Go package: reviewv1

## api/proto/schedule/v1/ (~22.6K tokens)

- **schedule.pb.go** (1418 lines, ~16.9K tok) — Go package: schedulev1
- **schedule.proto** (122 lines, ~782 tok) — schedule.proto in api/proto/schedule/v1/
- **schedule_grpc.pb.go** (388 lines, ~4.8K tok) — Go package: schedulev1

## api/proto/search/v1/ (~32.4K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/search/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/search/v1/
- **search.pb.go** (1907 lines, ~24.1K tok) — Go package: searchv1
- **search.proto** (401 lines, ~3.4K tok) — search.proto in api/proto/search/v1/
- **search_grpc.pb.go** (345 lines, ~4.3K tok) — Go package: searchv1

## api/proto/source_mappings/v1/ (~15.2K tokens)

- **source_mappings.pb.go** (848 lines, ~11.1K tok) — Go package: source_mappingsv1
- **source_mappings.proto** (102 lines, ~827 tok) — source_mappings.proto in api/proto/source_mappings/v1/
- **source_mappings_grpc.pb.go** (251 lines, ~3.3K tok) — Go package: source_mappingsv1

## api/proto/teams/v1/ (~20.2K tokens)

- **teams.pb.go** (1327 lines, ~14.4K tok) — Go package: teamsv1
- **teams.proto** (229 lines, ~1.5K tok) — teams.proto in api/proto/teams/v1/
- **teams_grpc.pb.go** (373 lines, ~4.4K tok) — Go package: teamsv1

## api/proto/tenant/v1/ (~18.0K tokens)

- **go.mod** (18 lines, ~126 tok) — go.mod in api/proto/tenant/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/tenant/v1/
- **tenant.pb.go** (1079 lines, ~12.1K tok) — Go package: tenantv1
- **tenant.proto** (195 lines, ~1.4K tok) — tenant.proto in api/proto/tenant/v1/
- **tenant_grpc.pb.go** (333 lines, ~4.0K tok) — Go package: tenantv1

## api/proto/threads/v1/ (~11.2K tokens)

- **threads.pb.go** (705 lines, ~8.6K tok) — Go package: threadsv1
- **threads.proto** (89 lines, ~744 tok) — threads.proto in api/proto/threads/v1/
- **threads_grpc.pb.go** (171 lines, ~1.9K tok) — Go package: threadsv1

## api/proto/topic/v1/ (~16.5K tokens)

- **topic.pb.go** (1078 lines, ~11.9K tok) — topic.pb.go in api/proto/topic/v1/
- **topic.proto** (192 lines, ~1.3K tok) — topic.proto in api/proto/topic/v1/
- **topic_grpc.pb.go** (292 lines, ~3.3K tok) — topic_grpc.pb.go in api/proto/topic/v1/

## api/proto/watchlist/v1/ (~27.0K tokens)

- **watchlist.pb.go** (1676 lines, ~20.2K tok) — Go package: watchlistv1
- **watchlist.proto** (200 lines, ~1.7K tok) — watchlist.proto in api/proto/watchlist/v1/
- **watchlist_grpc.pb.go** (401 lines, ~5.1K tok) — Go package: watchlistv1

## api/proto/workflow/v1/ (~34.4K tokens)

- **go.mod** (18 lines, ~127 tok) — go.mod in api/proto/workflow/v1/
- **go.sum** (20 lines, ~421 tok) — go.sum in api/proto/workflow/v1/
- **workflow.pb.go** (2336 lines, ~27.6K tok) — workflow.pb.go in api/proto/workflow/v1/
- **workflow.proto** (419 lines, ~3.2K tok) — workflow.proto in api/proto/workflow/v1/
- **workflow_grpc.pb.go** (252 lines, ~3.1K tok) — workflow_grpc.pb.go in api/proto/workflow/v1/

## configs/ (~812 tokens)

- **enrichment_processors.yaml** (91 lines, ~812 tok) — Enrichment Pipeline Processor Configuration

## context-archive/ (~17.8K tokens)

- **ARCHITECTURE.md** (196 lines, ~2.5K tok) — Penfold Architecture
- **cli-audit-report.md** (164 lines, ~1.5K tok) — penf CLI Audit Report: AI-Native Design Gap Analysis
- **infrastructure.md** (853 lines, ~10.2K tok) — Penfold Infrastructure
- **root-agent.md** (393 lines, ~3.6K tok) — Penfold Development Context (Root Agent)

## context-archive/agents/ (~15.2K tokens)

- **ai-dev.md** (138 lines, ~1.1K tok) — Intelligence layer - search, LLM, embeddings, correlations, context assembly
- **cli-dev.md** (296 lines, ~2.5K tok) — Command-line interface - Cobra commands, user interaction, output formatting
- **data-dev.md** (212 lines, ~1.4K tok) — PostgreSQL, pgvector, repositories, migrations, multi-tenant patterns
- **debugger.md** (242 lines, ~1.8K tok) — Investigate bugs without fixing them. Produces root cause analysis and creates fix shards. Use for complex bugs (>30 min), recurring issues, or "why did this happen?" questions. NOT for simple typos or "just fix it" requests.
- **gmail-dev.md** (238 lines, ~1.6K tok) — Gmail connector - OAuth2 PKCE, sync, push notifications, attachments
- **service-dev.md** (330 lines, ~2.5K tok) — Go services, gRPC servers, protobuf definitions, service wiring, middleware, and cross-service communication
- **speckit-dev.md** (211 lines, ~1.2K tok) — Reference documentation for speckit skills (not an agent)
- **testing-dev.md** (259 lines, ~1.5K tok) — Test framework, fixtures, all test tiers (unit, integration, e2e, live, benchmark)
- **worker-dev.md** (194 lines, ~1.5K tok) — Temporal workflows and activities - durable execution, orchestration, retry handling

## context-archive/architecture/ (~9.8K tokens)

- **core-patterns.md** (151 lines, ~1.3K tok) — Core Architecture Patterns
- **email-patterns.md** (81 lines, ~683 tok) — Email Integration Patterns
- **observability-patterns.md** (94 lines, ~838 tok) — Observability Patterns
- **relationship-patterns.md** (114 lines, ~882 tok) — Relationship Discovery Patterns
- **slm-llm-pipeline.md** (286 lines, ~3.9K tok) — SLM/LLM Content Processing Pipeline
- **testing-patterns.md** (308 lines, ~2.2K tok) — Testing Patterns

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

- **agent-mail.md** (373 lines, ~2.6K tok) — Agent Mail
- **entities.md** (620 lines, ~6.0K tok) — Penfold Entity Model
- **interaction-model.md** (179 lines, ~2.2K tok) — Penfold Interaction Model
- **use-cases.md** (383 lines, ~3.5K tok) — Penfold Use Cases
- **vision.md** (75 lines, ~697 tok) — Penfold Vision

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

## deploy/systemd/ (~2.5K tokens)

- **README.md** (98 lines, ~681 tok) — Penfold systemd Services (Linux)
- **install.sh** (157 lines, ~1.2K tok) — install.sh in deploy/systemd/
- **penfold-ai-coordinator.service** (36 lines, ~203 tok) — penfold-ai-coordinator.service in deploy/systemd/
- **penfold-alert-webhook.service** (27 lines, ~149 tok) — penfold-alert-webhook.service in deploy/systemd/
- **penfold-gateway.service** (40 lines, ~240 tok) — penfold-gateway.service in deploy/systemd/

## docs/adr/ (~3.1K tokens)

- **tempts-evaluation.md** (281 lines, ~3.1K tok) — ADR: Evaluate tempts for Temporal Workflow Type Safety

## docs/ai-coordination/ (~3.2K tokens)

- **README.md** (456 lines, ~3.2K tok) — AI Coordination Framework User Guide

## docs/analysis/ (~1.5K tokens)

- **e2e-glossary-test-failures.md** (204 lines, ~1.5K tok) — E2E Glossary Test Failures Analysis

## docs/ (~14.0K tokens)

- **assistant-rules.md** (352 lines, ~3.2K tok) — Penfold Assistant Rules
- **index.md** (269 lines, ~2.2K tok) — Penfold System Documentation
- **ingest-pipeline.md** (288 lines, ~2.4K tok) — Penfold Ingest Pipeline
- **preferences.md** (86 lines, ~523 tok) — Penfold User Preferences
- **processes.md** (34 lines, ~254 tok) — Penfold Processes
- **ways-of-working.md** (509 lines, ~5.4K tok) — Ways of Working

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

- **README.md** (236 lines, ~2.3K tok) — Gmail Integration Documentation
- **api-reference.md** (1216 lines, ~8.4K tok) — Gmail Integration API Reference
- **architecture.md** (780 lines, ~6.0K tok) — Gmail Integration Architecture
- **setup-guide.md** (512 lines, ~3.1K tok) — Gmail Integration Setup Guide
- **troubleshooting.md** (1009 lines, ~6.0K tok) — Gmail Integration Troubleshooting Guide

## docs/infrastructure/ (~35.9K tokens)

- **ai-model-server-setup.md** (701 lines, ~4.6K tok) — AI Model Server Setup
- **backup-recovery.md** (668 lines, ~4.5K tok) — Backup and Recovery Procedures
- **ci-cd-pipeline.md** (821 lines, ~5.7K tok) — CI/CD Pipeline Definition
- **monitoring-observability.md** (970 lines, ~7.5K tok) — Monitoring & Observability Infrastructure Guide
- **mtls-setup.md** (441 lines, ~2.7K tok) — mTLS Authentication Setup
- **production-deployment.md** (933 lines, ~6.4K tok) — Penfold Production Deployment Guide
- **secrets-management.md** (670 lines, ~4.5K tok) — Secrets Management

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

- **entities.md** (620 lines, ~6.0K tok) — Penfold Entity Model
- **interaction-model.md** (179 lines, ~2.2K tok) — Penfold Interaction Model
- **use-cases.md** (383 lines, ~3.5K tok) — Penfold Use Cases
- **vision.md** (75 lines, ~697 tok) — Penfold Vision

## docs/testing-framework/ (~8.5K tokens)

- **BENCHMARKING.md** (59 lines, ~337 tok) — Benchmark Tests
- **FIXTURES-GUIDE.md** (138 lines, ~960 tok) — Test Fixtures
- **LOCAL-SETUP.md** (52 lines, ~345 tok) — Local Test Setup
- **TROUBLESHOOTING.md** (79 lines, ~718 tok) — Test Troubleshooting
- **ai-mocking.md** (859 lines, ~6.2K tok) — AI Model Mocking Strategies

## docs/workflows/ (~6.1K tokens)

- **acronym-review.md** (179 lines, ~1.5K tok) — Workflow: Acronym Review
- **init-entities.md** (236 lines, ~1.4K tok) — Workflow: Init Entities
- **mention-review.md** (259 lines, ~1.6K tok) — Workflow: Mention Review
- **onboarding.md** (235 lines, ~1.6K tok) — Workflow: Post-Import Onboarding

## migrations/ (~134.7K tokens)

- **001_ingest_tables.sql** (59 lines, ~876 tok) — =====================================================
- **002_content_enrichment.sql** (136 lines, ~1.7K tok) — =====================================================
- **003_entity_resolution.sql** (219 lines, ~2.4K tok) — =====================================================
- **004_type_handlers.sql** (499 lines, ~5.7K tok) — =====================================================
- **005_ai_extraction.sql** (384 lines, ~5.1K tok) — =====================================================
- **006_queue_infrastructure.sql** (237 lines, ~2.6K tok) — =====================================================
- **007_tenant_configuration.sql** (202 lines, ~2.8K tok) — Migration 007: Tenant Configuration Tables
- **008_source_attachments.sql** (86 lines, ~1.3K tok) — =====================================================
- **009_drop_sources_fulltext_index.sql** (10 lines, ~134 tok) — Migration: 009_drop_sources_fulltext_index
- **010_meetings.sql** (79 lines, ~829 tok) — Migration: 010_meetings
- **011_meeting_participants.sql** (45 lines, ~644 tok) — Migration: meeting_participants junction table
- **012_meeting_mentions.sql** (44 lines, ~645 tok) — Migration: meeting_mentions table
- **013_glossary.sql** (84 lines, ~856 tok) — Glossary table for acronyms and domain terminology
- **014_review_queue.sql** (100 lines, ~1.1K tok) — Review queue for AI questions requiring human input
- **015_products.sql** (350 lines, ~4.2K tok) — =====================================================
- **016_glossary_linked_entity.sql** (41 lines, ~456 tok) — =====================================================
- **017_mention_resolution.sql** (435 lines, ~5.0K tok) — =====================================================
- **018_resolution_comparisons.sql** (123 lines, ~1.6K tok) — =====================================================
- **019_people_company.sql** (10 lines, ~125 tok) — Migration: Add company field to people table
- **020_meeting_series.sql** (61 lines, ~625 tok) — Migration: 020_meeting_series
- **020_sources_participant_emails.sql** (21 lines, ~289 tok) — =====================================================
- **021_content_id.sql** (33 lines, ~364 tok) — =====================================================
- **022_ai_model_registry.sql** (204 lines, ~2.9K tok) — =====================================================
- **023_service_logs.sql** (90 lines, ~1.2K tok) — =====================================================
- **024_fix_tenant_schema.sql** (54 lines, ~611 tok) — Migration 024: Fix Tenant Schema
- **025_glossary_embeddings.sql** (19 lines, ~234 tok) — Add embedding column to glossary for semantic vector search
- **026_add_content_validation.sql** (49 lines, ~583 tok) — Add content validation tracking columns to sources table
- **027_content_insights.sql** (89 lines, ~1.2K tok) — Migration: 027_content_insights
- **028_assertion_lifecycle.sql** (78 lines, ~1.2K tok) — Migration: 028_assertion_lifecycle
- **029_pipeline_registry.sql** (129 lines, ~2.4K tok) — Migration: 029_pipeline_registry
- **030_trust_seniority.sql** (66 lines, ~774 tok) — Migration: 030_trust_seniority
- **031_watch_list.sql** (45 lines, ~538 tok) — Migration: 031_watch_list
- **032_seed_triage_prompt.sql** (17 lines, ~339 tok) — Seed the triage prompt template into prompt_templates table
- **033_multi_level_embeddings.sql** (61 lines, ~769 tok) — Migration: 033_multi_level_embeddings
- **034_assertions_schema_align.sql** (55 lines, ~935 tok) — 034_assertions_schema_align.sql
- **035_pipeline_config.sql** (80 lines, ~1.3K tok) — 035_pipeline_config.sql
- **036_deploy_history.sql** (38 lines, ~487 tok) — 036_deploy_history.sql
- **037_entity_lifecycle.sql** (44 lines, ~675 tok) — 037_entity_lifecycle.sql
- **038_assertions_attribution.sql** (26 lines, ~345 tok) — 038_assertions_attribution.sql
- **039_pipeline_stage_io.sql** (26 lines, ~309 tok) — Migration: 039_pipeline_stage_io
- **040_entity_account_types.sql** (10 lines, ~93 tok) — Migration: Add 'team' and 'service' account types
- **041_assertions_dedup.sql** (46 lines, ~534 tok) — 041_assertions_dedup.sql
- **042_fix_tenant_config_tables.sql** (180 lines, ~2.6K tok) — Migration 042: Fix Tenant Configuration Tables
- **043_person_message_counts.sql** (21 lines, ~290 tok) — Migration: Add message count tracking to persons table
- **044_add_parsed_extracted_status.sql** (28 lines, ~370 tok) — Migration 044: Add 'parsed' and 'extracted' to processing_status constraint
- **045_rename_people_title.sql** (26 lines, ~265 tok) — Migration: Rename people.title to people.job_title
- **046_cleanup_job_title_garbage.sql** (20 lines, ~233 tok) — Migration: Clean up garbage data in people.job_title column
- **047_source_system.sql** (22 lines, ~293 tok) — Migration: Add source_system classification to content_enrichment
- **048_fix_assertion_dedup_key.sql** (69 lines, ~1.0K tok) — 048_fix_assertion_dedup_key.sql
- **049_conversations.sql** (222 lines, ~2.4K tok) — 049_conversations.sql
- **050_conversation_summaries.sql** (67 lines, ~897 tok) — 050_conversation_summaries.sql
- **051_classification_suggestions.sql** (39 lines, ~514 tok) — Migration: Create classification_suggestions table for LLM classification fallback
- **052_model_config.sql** (27 lines, ~299 tok) — 052_model_config.sql
- **053_concurrency_config_seed.sql** (7 lines, ~92 tok) — Migration 053: Seed concurrency configuration
- **054_per_stage_timeouts.sql** (22 lines, ~506 tok) — Per-stage timeout configuration keys.
- **055_model_stage_config.sql** (24 lines, ~383 tok) — 055_model_stage_config.sql
- **056_embedding_chunks.sql** (11 lines, ~114 tok) — 056_embedding_chunks.sql
- **057_langfuse_trace_id.sql** (10 lines, ~91 tok) — 057_langfuse_trace_id.sql
- **058_pipeline_batches.sql** (18 lines, ~214 tok) — 058_pipeline_batches.sql
- **059_pipeline_runs_observability.sql** (12 lines, ~125 tok) — 059_pipeline_runs_observability.sql
- **060_expand_failure_category.sql** (49 lines, ~578 tok) — 060_expand_failure_category.sql
- **061_backfill_embedding_vec.sql** (14 lines, ~118 tok) — 061_backfill_embedding_vec.sql
- **062_triage_llm_timeouts.sql** (18 lines, ~225 tok) — Fix triage stage timeouts: use LLM-appropriate values instead of embedding-level.
- **063_glossary_lowercase_aliases.sql** (8 lines, ~138 tok) — Normalize existing glossary aliases to lowercase for case-insensitive JSONB containment queries.
- **064_add_graph_source_systems.sql** (19 lines, ~254 tok) — Migration: Add Graph connector source systems to CHECK constraint
- **064_classification_rules.sql** (31 lines, ~332 tok) — 064_classification_rules.sql in migrations/
- **064_people_email_case_insensitive.sql** (133 lines, ~1.2K tok) — =====================================================
- **065_seed_classification_rules.sql** (91 lines, ~1.3K tok) — Seed classification rules from hardcoded source_system.go
- **066_content_classification_wave2.sql** (11 lines, ~152 tok) — Migration 066: Content classification Wave 2
- **067_newsletter_seed_rules.sql** (38 lines, ~470 tok) — Seed newsletter detection rule for the classification rule engine.
- **068_pipeline_routing.sql** (54 lines, ~746 tok) — Pipeline routing table: data-driven routing from classification to pipeline stages.
- **069_seed_all_prompts.sql** (211 lines, ~2.9K tok) — Migration: 067_seed_all_prompts
- **070_backfill_classification_fields.sql** (90 lines, ~991 tok) — Migration 070: Backfill existing items with classification fields
- **071_seed_model_config.sql** (20 lines, ~253 tok) — Migration: 071_seed_model_config
- **072_restore_confidence_default.sql** (17 lines, ~226 tok) — 072_restore_confidence_default.sql
- **073_seed_stage_model_config.sql** (25 lines, ~359 tok) — Migration: 073_seed_stage_model_config
- **074_pipeline_definitions.sql** (94 lines, ~1.4K tok) — Migration 074: Pipeline definitions table
- **075_fix_meeting_transcript_route.sql** (23 lines, ~202 tok) — Migration 075: Fix MEETING/TRANSCRIPT routing to use 'transcript' pipeline.
- **076_delete_dead_pipeline_config_model_rows.sql** (16 lines, ~158 tok) — Migration 076: Delete dead model.stage.* rows from pipeline_config.
- **077_delete_dead_pipeline_config_timeout_rows.sql** (15 lines, ~133 tok) — Migration 077: Delete dead timeout.stage.* rows from pipeline_config.
- **078_pipeline_operational_config.sql** (37 lines, ~441 tok) — Migration 078: Create pipeline_operational_config and drop pipeline_config.
- **079_tenant_primary_user_email.sql** (10 lines, ~104 tok) — Migration 079: Set primary_user_email in tenant settings.
- **080_fix_transcript_pipeline_definition.sql** (165 lines, ~1.8K tok) — Migration 080: Fix transcript pipeline definition
- **081_pipeline_definitions_content_type.sql** (14 lines, ~229 tok) — Migration 081: Add content_type to pipeline_definitions
- **082_topics.sql** (31 lines, ~394 tok) — Migration 082: Add topics entity type
- **083_akamai_tenant_patterns.sql** (24 lines, ~365 tok) — Migration 083: Seed Akamai-specific patterns into tenant config
- **084_seed_deep_analysis_prompt.sql** (120 lines, ~1.2K tok) — Migration 084: Seed deep_analysis prompt template
- **085_tenant_domain_companies.sql** (47 lines, ~475 tok) — Migration 084: Create tenant_domain_companies table
- **086_seed_mention_resolver_prompts.sql** (229 lines, ~2.9K tok) — Migration: 082_seed_mention_resolver_prompts
- **087_pipeline_definitions_llm_params.sql** (38 lines, ~510 tok) — 087_pipeline_definitions_llm_params.sql in migrations/
- **088_routing_rules_conditions.sql** (41 lines, ~716 tok) — Add conditions JSONB column to ai_routing_rules for conditional matching.
- **089_session_ledger_entries.sql** (47 lines, ~746 tok) — 089_session_ledger_entries.sql in migrations/
- **090_session_ledger_consolidations.sql** (29 lines, ~414 tok) — 090_session_ledger_consolidations.sql in migrations/
- **091_seed_consolidation_prompt.sql** (54 lines, ~741 tok) — Migration 091: Seed session_ledger_consolidation prompt template
- **092_seed_classification_rules_wave3.sql** (158 lines, ~2.4K tok) — Wave 3 classification rules: calendar responses, notifications, newsletters
- **093_lightweight_pipelines.sql** (128 lines, ~1.7K tok) — Migration 093: Lightweight pipeline definitions and routing rules
- **094_fix_pipeline_stage_definitions.sql** (68 lines, ~897 tok) — Migration 094: Fix pipeline stage definitions to match actual execution
- **095_update_ner_prompt_v2.sql** (36 lines, ~667 tok) — pf-303111: Update NER prompt to reduce entity type noise.
- **096_replace_deprecated_model.sql** (19 lines, ~267 tok) — pf-58cad6: Replace deprecated gemini-2.0-flash with gemini-2.5-flash in all routing rules.
- **097_add_summarize_to_standard_transcript.sql** (101 lines, ~1.2K tok) — Migration 097: Add summarize stage to standard and transcript pipelines
- **098_seed_prompt_variants.sql** (161 lines, ~2.8K tok) — pf-14f931: Seed pipeline-specific prompt variants for notification and newsletter pipelines.
- **099_set_pipeline_prompt_overrides.sql** (10 lines, ~145 tok) — pf-14f931: Set prompt_override on notification and newsletter pipeline definitions.
- **100_entity_content_roles.sql** (47 lines, ~620 tok) — Migration 100: Entity-Content Role Associations (Phase 1)
- **101_schedules.sql** (70 lines, ~789 tok) — Migration 101: Scheduling Infrastructure (Phase 1)
- **102_update_embedding_routing_rule.sql** (10 lines, ~108 tok) — Update the embeddings-local routing rule to use mxbai-embed-large (actual model in use)
- **103_entity_model_extensions.sql** (27 lines, ~390 tok) — Migration 103: Entity Model Extensions — Phase 1
- **104_entity_enrichment_stage.sql** (50 lines, ~542 tok) — Migration 104: Register enrich_entities pipeline stage for Phase 2 Entity Model Extensions
- **105_bridge_sessions.sql** (28 lines, ~338 tok) — Migration 105: Create bridge_sessions table for messaging bridge session history.
- **106_seed_bridge_config.sql** (33 lines, ~592 tok) — Migration 106: Seed bridge service configuration.
- **107_heartbeat_indexes.sql** (18 lines, ~276 tok) — Migration: heartbeat query indexes
- **108_microsoft_graph_integration.sql** (9 lines, ~111 tok) — Migration 108: Add microsoft_graph to tenant_integrations integration_type
- **109_project_source_mappings.sql** (21 lines, ~272 tok) — Migration 109: Project source mappings
- **110_attribution_columns.sql** (26 lines, ~377 tok) — Migration 110: Add attribution columns for project attribution pipeline stage
- **111_attribute_project_stage.sql** (44 lines, ~712 tok) — Migration 111: Register attribute_project pipeline stage and seed configuration
- **112_ad_sync_support.sql** (22 lines, ~299 tok) — Migration 112: AD Sync Support
- **113_watch_instructions.sql** (66 lines, ~855 tok) — Migration 113: Watch instructions tables, pipeline stage, and operational config
- **114_instruction_evaluate_stage.sql** (27 lines, ~662 tok) — Migration 114: Wire instruction_evaluate pipeline stage — prompt, routing, pipeline defs
- **115_digests.sql** (48 lines, ~864 tok) — Migration 115: Digests table and daily digest prompt template
- **116_weekly_digest.sql** (49 lines, ~944 tok) — Migration 116: Weekly digest pipeline stages and prompt templates
- **117_journal_digest.sql** (26 lines, ~560 tok) — Migration 117: Journal digest pipeline stage and prompt template
- **118_alerts.sql** (23 lines, ~285 tok) — Migration 118: Alert model for instruction-triggered notifications
- **119_seed_embedding_chunk_config.sql** (15 lines, ~194 tok) — Migration 119: Seed embedding chunk configuration defaults
- **120_update_ner_semantic_prompt_v3.sql** (71 lines, ~1.1K tok) — pf-9b64d2: Add disambiguation boundary instruction to NER and semantic prompt templates.
- **121_update_notification_prompt_v3.sql** (78 lines, ~1.4K tok) — pf-1c083d: Update triage prompt v2 (notification pipeline) to correctly handle hybrid notifications.
- **122_fix_pipeline_routing_dedup.sql** (50 lines, ~650 tok) — Migration 122: Fix pipeline_routing MEETING/TRANSCRIPT data and deduplicate rows.
- **123_add_missing_routing_rules.sql** (15 lines, ~259 tok) — Add missing deep_analysis routing rules for DECISION and ACTION_REQUEST/HIGH.
- **124_fix_summarize_stage_name.sql** (47 lines, ~554 tok) — Migration 123: Add 'summarize' to pipeline_stages registry
- **125_summarize_routing_rules.sql** (31 lines, ~350 tok) — Migration 125: Content-length-aware summarization routing
- **126_seed_ai_models.sql** (20 lines, ~678 tok) — Seed ai_models registry with all models currently in use plus Anthropic models
- **127_normalise_model_names.sql** (14 lines, ~258 tok) — Normalise model references to provider/model-name format
- **128_add_response_schema.sql** (118 lines, ~1.0K tok) — Add response_schema column for structured output enforcement
- **129_seed_triage_stage_params.sql** (11 lines, ~104 tok) — Seed triage stage params in pipeline_definitions
- **130_qwen3_json_schema.sql** (10 lines, ~83 tok) — Add json_schema capability to qwen3:8b
- **131_consolidate_model_config_routing_rules.sql** (32 lines, ~593 tok) — +goose Up
- **132_drop_model_config.sql** (14 lines, ~114 tok) — +goose Up
- **133_schedule_execution_history.sql** (20 lines, ~195 tok) — +goose Up
- **134_digest_routing_and_prompts.sql** (59 lines, ~1.3K tok) — +goose Up
- **135_newsletter_broad_patterns.sql** (45 lines, ~636 tok) — +goose Up
- **136_newsletter_enrichment.sql** (69 lines, ~1.2K tok) — +goose Up
- **137_newsletter_rollup_schedule.sql** (59 lines, ~1.0K tok) — +goose Up
- **138_digests_nullable_project_id.sql** (7 lines, ~56 tok) — +goose Up
- **139_email_config.sql** (15 lines, ~295 tok) — Migration 139: Email delivery configuration
- **140_notification_enrichment.sql** (76 lines, ~1.3K tok) — +goose Up
- **141_inbound_whitelist.sql** (6 lines, ~102 tok) — 141_inbound_whitelist.sql in migrations/
- **142_fix_notification_prompt_overrides.sql** (21 lines, ~223 tok) — pf-c4682e: Fix prompt_override for notification pipeline summarize and extract_semantic stages.
- **143_newsletter_extract_prompt_v2.sql** (23 lines, ~964 tok) — +goose Up
- **144_structured_extract_metadata.sql** (27 lines, ~369 tok) — +goose Up
- **145_seed_newsletter_user_context.sql** (9 lines, ~197 tok) — Migration 145: Seed newsletter user context into pipeline_operational_config.
- **146_newsletter_variant_routing.sql** (114 lines, ~2.2K tok) — +goose Up
- **147_newsletter_variant_pattern.sql** (175 lines, ~2.2K tok) — +goose Up
- **148_fix_notification_skip_when_low.sql** (21 lines, ~276 tok) — Migration 148: Fix notification pipeline skip_when_low values
- **148_newsletter_variant_dynamic_signal.sql** (35 lines, ~1.3K tok) — +goose Up
- **148_pipeline_definitions_depends_on.sql** (12 lines, ~195 tok) — Migration 148: Add depends_on column to pipeline_definitions
- **149_add_context_providers.sql** (36 lines, ~434 tok) — +goose Up
- **150_seed_concurrency_config.sql** (37 lines, ~381 tok) — +goose Up
- **151_set_stage_timeouts.sql** (21 lines, ~219 tok) — +goose Up
- **152_seed_backpressure_config.sql** (46 lines, ~515 tok) — +goose Up
- **153_pipeline_definitions_timeout_not_null.sql** (7 lines, ~76 tok) — +goose Up
- **154_fix_newsletter_ctg_subtype.sql** (7 lines, ~85 tok) — Fix newsletter_ctg classification rule: CTG Post-Its is an internal corporate
- **155_newsletter_digest_triage_calibration.sql** (59 lines, ~915 tok) — +goose Up

## ops/grafana/dashboards/ (~18.4K tokens)

- **penfold-overview.json** (316 lines, ~6.1K tok) — penfold-overview.json in ops/grafana/dashboards/
- **temporal-queues.json** (350 lines, ~6.6K tok) — temporal-queues.json in ops/grafana/dashboards/
- **vllm-mlx.json** (309 lines, ~5.7K tok) — vllm-mlx.json in ops/grafana/dashboards/

## penfold-go-pipeline/ (~7.3K tokens)

- **Makefile** (138 lines, ~963 tok) — Makefile in penfold-go-pipeline/
- **README.md** (194 lines, ~1.6K tok) — Penfold Go AI Processing Pipeline
- **docker-compose.temporal.yml** (58 lines, ~516 tok) — Temporal Server for Penfold AI Processing Pipeline
- **go.mod** (58 lines, ~669 tok) — go.mod in penfold-go-pipeline/
- **go.sum** (147 lines, ~3.6K tok) — go.sum in penfold-go-pipeline/

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

- **embeddings.go** (468 lines, ~3.5K tok) — Go package: storage
- **postgres.go** (209 lines, ~1.5K tok) — Package storage provides PostgreSQL database connectivity and repositories.
- **results.go** (404 lines, ~3.2K tok) — Go package: storage
- **sources.go** (345 lines, ~2.4K tok) — Go package: storage

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

## pkg/ai/ (~12.3K tokens)

- **client.go** (410 lines, ~3.4K tok) — Package ai provides a gRPC client for the AI Coordinator service.
- **client_options.go** (190 lines, ~1.3K tok) — Package ai provides a gRPC client for the AI Coordinator service.
- **client_test.go** (814 lines, ~7.2K tok) — Go package: ai
- **doc.go** (45 lines, ~378 tok) — Package ai provides a gRPC client for the AI Coordinator service.

## pkg/alert/ (~1.0K tokens)

- **repository.go** (138 lines, ~1.0K tok) — Package alert provides the repository layer for alert management.

## pkg/assertions/ (~5.8K tokens)

- **repository.go** (545 lines, ~4.1K tok) — Package assertions provides database operations for querying assertions across all content.
- **repository_test.go** (218 lines, ~1.7K tok) — Go package: assertions

## pkg/auth/ (~10.3K tokens)

- **apikey.go** (197 lines, ~1.6K tok) — Go package: auth
- **auth.go** (182 lines, ~1.5K tok) — Package auth provides shared authentication functionality for Go microservices.
- **auth_test.go** (724 lines, ~5.1K tok) — Go package: auth
- **middleware.go** (257 lines, ~2.1K tok) — Go package: auth

## pkg/buildinfo/ (~2.9K tokens)

- **buildinfo.go** (52 lines, ~398 tok) — Go package: buildinfo
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

## pkg/db/ (~14.5K tokens)

- **db.go** (192 lines, ~1.4K tok) — Package db provides shared PostgreSQL database utilities for Penfold microservices.
- **db_test.go** (241 lines, ~1.6K tok) — Go package: db
- **health.go** (81 lines, ~471 tok) — Go package: db
- **health_test.go** (35 lines, ~204 tok) — Go package: db
- **metrics.go** (118 lines, ~1.1K tok) — Package db provides shared PostgreSQL database utilities for Penfold microservices.
- **metrics_test.go** (160 lines, ~1.2K tok) — Go package: db
- **migrations.go** (403 lines, ~3.2K tok) — Go package: db
- **migrations_test.go** (573 lines, ~5.4K tok) — Go package: db

## pkg/digest/ (~7.7K tokens)

- **gather.go** (363 lines, ~3.5K tok) — Package digest provides data gathering functions for digest generation.
- **repository.go** (249 lines, ~2.0K tok) — Package digest provides the repository layer for digest management.
- **window.go** (78 lines, ~654 tok) — Go package: digest
- **window_test.go** (211 lines, ~1.5K tok) — Go package: digest

## pkg/embeddings/ (~34.5K tokens)

- **cache.go** (385 lines, ~2.7K tok) — Package embeddings provides MLX embedding generation for semantic search.
- **cache_test.go** (537 lines, ~3.5K tok) — Go package: embeddings
- **client.go** (523 lines, ~4.1K tok) — Package embeddings provides embedding generation for semantic search.
- **client_test.go** (698 lines, ~5.2K tok) — Go package: embeddings
- **config.go** (194 lines, ~1.7K tok) — Package embeddings provides embedding generation for semantic search.
- **config_test.go** (204 lines, ~1.4K tok) — Go package: embeddings
- **errors.go** (62 lines, ~693 tok) — Package embeddings provides MLX embedding generation for semantic search.
- **mlx_client.go** (611 lines, ~4.8K tok) — Package embeddings provides embedding generation for semantic search.
- **mlx_client_test.go** (558 lines, ~4.1K tok) — Go package: embeddings
- **normalize.go** (302 lines, ~2.0K tok) — Package embeddings provides MLX embedding generation for semantic search.
- **normalize_test.go** (569 lines, ~4.2K tok) — Go package: embeddings

## pkg/enrichment/classification/ (~21.3K tokens)

- **classifier.go** (383 lines, ~3.4K tok) — Package classification provides content classification for the enrichment pipeline.
- **classifier_test.go** (270 lines, ~2.3K tok) — Go package: classification
- **engine.go** (174 lines, ~1.4K tok) — Go package: classification
- **engine_test.go** (1315 lines, ~13.2K tok) — Go package: classification
- **repository.go** (138 lines, ~973 tok) — Go package: classification

## pkg/enrichment/ (~18.7K tokens)

- **classifier.go** (226 lines, ~2.0K tok) — Go package: enrichment
- **classifier_test.go** (609 lines, ~5.0K tok) — Go package: enrichment
- **repository.go** (743 lines, ~5.5K tok) — Go package: enrichment
- **repository_create_test.go** (131 lines, ~1.5K tok) — Go package: enrichment
- **types.go** (254 lines, ~2.8K tok) — Package enrichment provides the content enrichment pipeline for Penfold.
- **types_source_system_test.go** (243 lines, ~1.8K tok) — Go package: enrichment

## pkg/enrichment/config/ (~13.5K tokens)

- **config.go** (450 lines, ~3.6K tok) — Package config provides tenant configuration management for the enrichment pipeline.
- **config_test.go** (293 lines, ~2.1K tok) — Go package: config
- **repository.go** (351 lines, ~2.8K tok) — Go package: config
- **repository_integration_test.go** (175 lines, ~1.7K tok) — Go package: config
- **repository_test.go** (374 lines, ~3.3K tok) — Go package: config

## pkg/enrichment/entities/ (~56.7K tokens)

- **entity_similarity_test.go** (200 lines, ~1.7K tok) — Go package: entities
- **migration_consistency_test.go** (173 lines, ~1.9K tok) — Go package: entities
- **normalize.go** (580 lines, ~4.3K tok) — Go package: entities
- **normalize_test.go** (496 lines, ~4.5K tok) — Go package: entities
- **repository.go** (1972 lines, ~15.9K tok) — Go package: entities
- **repository_delete_integration_test.go** (231 lines, ~2.1K tok) — Go package: entities
- **repository_lifecycle_test.go** (1061 lines, ~8.2K tok) — Go package: entities
- **repository_message_counts_test.go** (310 lines, ~2.7K tok) — Go package: entities
- **resolver.go** (345 lines, ~2.9K tok) — Go package: entities
- **resolver_integration_test.go** (579 lines, ~6.0K tok) — Go package: entities
- **resolver_test.go** (376 lines, ~4.4K tok) — Go package: entities
- **types.go** (232 lines, ~2.3K tok) — Package entities provides entity resolution for people, teams, and projects.

## pkg/enrichment/extraction/ (~15.8K tokens)

- **context.go** (528 lines, ~4.6K tok) — Package extraction provides AI extraction capabilities for the enrichment pipeline.
- **extraction_test.go** (209 lines, ~1.6K tok) — Go package: extraction
- **extractor.go** (468 lines, ~4.2K tok) — Go package: extraction
- **mentions.go** (273 lines, ~2.3K tok) — Go package: extraction
- **templates.go** (309 lines, ~3.1K tok) — Go package: extraction

## pkg/enrichment/handlers/ (~18.6K tokens)

- **calendar.go** (456 lines, ~3.9K tok) — Go package: handlers
- **handlers_test.go** (746 lines, ~6.0K tok) — Go package: handlers
- **jira.go** (296 lines, ~2.9K tok) — Go package: handlers
- **links.go** (458 lines, ~3.4K tok) — Package handlers provides type-specific enrichment processors.
- **threads.go** (289 lines, ~2.4K tok) — Go package: handlers

## pkg/enrichment/observability/ (~11.2K tokens)

- **events.go** (322 lines, ~3.2K tok) — Package observability provides event schemas, metrics, and tracing for the enrichment pipeline.
- **metrics.go** (263 lines, ~2.8K tok) — Go package: observability
- **observability_test.go** (372 lines, ~3.3K tok) — Go package: observability
- **tracing.go** (232 lines, ~2.0K tok) — Go package: observability

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

## pkg/ (~6.8K tokens)

- **go.mod** (87 lines, ~1.1K tok) — go.mod in pkg/
- **go.sum** (224 lines, ~5.6K tok) — go.sum in pkg/

## pkg/graph/ (~15.0K tokens)

- **auth.go** (201 lines, ~1.8K tok) — Go package: graph
- **auth_test.go** (399 lines, ~3.5K tok) — Go package: graph
- **client.go** (139 lines, ~1.3K tok) — Go package: graph
- **directory.go** (205 lines, ~1.7K tok) — Go package: graph
- **directory_test.go** (55 lines, ~414 tok) — Go package: graph
- **mail.go** (219 lines, ~2.0K tok) — Go package: graph
- **models.go** (103 lines, ~948 tok) — Package graph provides a Microsoft Graph API client for the Penfold system.
- **teams.go** (153 lines, ~1.4K tok) — Go package: graph
- **token_store.go** (104 lines, ~882 tok) — Go package: graph
- **token_store_test.go** (39 lines, ~310 tok) — Go package: graph
- **transcripts.go** (61 lines, ~723 tok) — Go package: graph

## pkg/health/ (~6.0K tokens)

- **checks.go** (106 lines, ~948 tok) — Go package: health
- **health.go** (228 lines, ~1.6K tok) — Package health provides shared health check functionality for Go microservices.
- **health_test.go** (513 lines, ~3.4K tok) — Go package: health

## pkg/ingest/attachments/ (~10.6K tokens)

- **classifier.go** (160 lines, ~1.3K tok) — Go package: attachments
- **extractor.go** (413 lines, ~3.8K tok) — Go package: attachments
- **heuristic.go** (337 lines, ~2.7K tok) — Go package: attachments
- **heuristic_test.go** (252 lines, ~1.6K tok) — Go package: attachments
- **types.go** (109 lines, ~1.1K tok) — Package attachments provides attachment extraction and classification for email ingest.

## pkg/ingest/batch/ (~14.5K tokens)

- **processor.go** (775 lines, ~6.3K tok) — Go package: batch
- **processor_classification_test.go** (585 lines, ~5.0K tok) — Go package: batch
- **processor_test.go** (262 lines, ~1.7K tok) — Go package: batch
- **progress.go** (218 lines, ~1.5K tok) — Package batch provides batch processing for email ingest.

## pkg/ingest/eml/ (~11.5K tokens)

- **parser.go** (720 lines, ~5.4K tok) — Go package: eml
- **parser_test.go** (466 lines, ~3.7K tok) — Go package: eml
- **types.go** (210 lines, ~1.6K tok) — Package eml provides parsing for RFC 5322 email (.eml) files.
- **types_test.go** (115 lines, ~884 tok) — Go package: eml

## pkg/ingest/eml/testdata/ (~591 tokens)

- **multipart.eml** (32 lines, ~221 tok) — multipart.eml in pkg/ingest/eml/testdata/
- **no_message_id.eml** (9 lines, ~66 tok) — no_message_id.eml in pkg/ingest/eml/testdata/
- **simple.eml** (14 lines, ~76 tok) — simple.eml in pkg/ingest/eml/testdata/
- **with_attachment.eml** (32 lines, ~228 tok) — with_attachment.eml in pkg/ingest/eml/testdata/

## pkg/ingest/events/ (~4.6K tokens)

- **publisher.go** (344 lines, ~3.1K tok) — Package events provides event publishing for the email ingest pipeline.
- **publisher_test.go** (195 lines, ~1.5K tok) — Go package: events

## pkg/ingest/meeting/ (~20.9K tokens)

- **acronyms.go** (242 lines, ~1.8K tok) — Go package: meeting
- **acronyms_test.go** (198 lines, ~1.4K tok) — Go package: meeting
- **chat_parser.go** (141 lines, ~921 tok) — Go package: meeting
- **chat_parser_test.go** (149 lines, ~1.4K tok) — Go package: meeting
- **mentions.go** (163 lines, ~1.2K tok) — Go package: meeting
- **mentions_test.go** (169 lines, ~1.4K tok) — Go package: meeting
- **resolver.go** (154 lines, ~1.1K tok) — Go package: meeting
- **resolver_test.go** (195 lines, ~1.5K tok) — Go package: meeting
- **scanner.go** (385 lines, ~2.7K tok) — Go package: meeting
- **scanner_test.go** (226 lines, ~1.8K tok) — Go package: meeting
- **txt_parser.go** (91 lines, ~592 tok) — Go package: meeting
- **txt_parser_test.go** (155 lines, ~1.3K tok) — Go package: meeting
- **types.go** (61 lines, ~654 tok) — Package meeting provides parsing and processing for meeting transcripts and chat logs.
- **vtt_parser.go** (180 lines, ~1.4K tok) — Go package: meeting
- **vtt_parser_test.go** (243 lines, ~1.8K tok) — Go package: meeting

## pkg/ingest/storage/ (~11.1K tokens)

- **attachments.go** (467 lines, ~3.5K tok) — Go package: storage
- **repository.go** (676 lines, ~5.6K tok) — Package storage provides database operations for email ingest.
- **repository_test.go** (225 lines, ~1.9K tok) — Go package: storage

## pkg/ingest/teams/ (~3.0K tokens)

- **parser.go** (143 lines, ~1.1K tok) — Package teams provides parsing for Microsoft Teams channel messages.
- **parser_test.go** (263 lines, ~1.9K tok) — Go package: teams

## pkg/ingest/types/ (~568 tokens)

- **attachment.go** (62 lines, ~568 tok) — Package types provides shared types for the ingest pipeline.

## pkg/instructions/ (~3.8K tokens)

- **repository.go** (447 lines, ~3.8K tok) — Package instructions provides the repository layer for watch instructions management.

## pkg/langfuse/ (~15.3K tokens)

- **bug_pf_73ed30_test.go** (198 lines, ~2.1K tok) — Package langfuse_test contains reproduction tests for pf-73ed30:
- **client.go** (389 lines, ~3.6K tok) — Package langfuse provides a client for the Langfuse Datasets API.
- **client_test.go** (405 lines, ~3.2K tok) — Go package: langfuse_test
- **ingestion.go** (304 lines, ~2.5K tok) — Go package: langfuse
- **ingestion_test.go** (425 lines, ~3.5K tok) — Go package: langfuse_test
- **types.go** (51 lines, ~394 tok) — Go package: langfuse

## pkg/ledger/ (~6.5K tokens)

- **repository.go** (764 lines, ~5.6K tok) — Go package: ledger
- **types.go** (92 lines, ~879 tok) — Package ledger provides types and repository for session ledger management.

## pkg/logging/ (~9.8K tokens)

- **logger.go** (398 lines, ~2.8K tok) — Package logging provides structured logging for Penfold Go microservices.
- **logger_test.go** (403 lines, ~2.9K tok) — Go package: logging
- **sink.go** (279 lines, ~1.7K tok) — Go package: logging
- **sink_test.go** (351 lines, ~2.4K tok) — Go package: logging

## pkg/logs/ (~3.9K tokens)

- **repository.go** (415 lines, ~2.9K tok) — Go package: logs
- **types.go** (96 lines, ~953 tok) — Package logs provides types and operations for service log management.

## pkg/mentions/audit/ (~21.3K tokens)

- **comparison.go** (421 lines, ~3.7K tok) — Package audit provides resolution trace recording, auditing, and model comparison capabilities.
- **postgres_repository.go** (1178 lines, ~9.9K tok) — Go package: audit
- **repository.go** (59 lines, ~616 tok) — Go package: audit
- **trace.go** (275 lines, ~2.0K tok) — Go package: audit
- **tracing_tracer.go** (387 lines, ~3.2K tok) — Package audit provides resolution trace recording and auditing capabilities.
- **types.go** (134 lines, ~1.9K tok) — Package audit provides resolution trace recording and auditing capabilities.

## pkg/mentions/learning/ (~3.9K tokens)

- **clusters.go** (278 lines, ~1.9K tok) — Package learning provides correction tracking and learning capabilities.
- **corrections.go** (239 lines, ~2.0K tok) — Package learning provides correction tracking and learning capabilities for mention resolution.

## pkg/mentions/ (~12.0K tokens)

- **postgres_repository.go** (1036 lines, ~8.1K tok) — Package mentions provides unified mention resolution for all entity types.
- **repository.go** (72 lines, ~1.0K tok) — Package mentions provides unified mention resolution for all entity types.
- **types.go** (290 lines, ~2.8K tok) — Package mentions provides unified mention resolution for all entity types.

## pkg/mentions/resolver/ (~36.1K tokens)

- **ai_coordinator_provider_test.go** (159 lines, ~1.7K tok) — Go package: resolver
- **ai_coordinator_types.go** (205 lines, ~1.8K tok) — Go package: resolver
- **ai_provider.go** (182 lines, ~1.5K tok) — Package resolver provides LLM-driven mention resolution using a multi-stage pipeline.
- **ai_provider_test.go** (397 lines, ~3.1K tok) — Go package: resolver
- **candidates.go** (269 lines, ~2.3K tok) — Go package: resolver
- **config.go** (118 lines, ~929 tok) — Go package: resolver
- **entity_type_propagation_test.go** (91 lines, ~938 tok) — Go package: resolver
- **llm.go** (128 lines, ~1.0K tok) — Go package: resolver
- **resolver.go** (450 lines, ~4.2K tok) — Package resolver provides LLM-driven mention resolution using a multi-stage pipeline.
- **resolver_test.go** (603 lines, ~5.3K tok) — Go package: resolver
- **stages.go** (556 lines, ~5.0K tok) — Go package: resolver
- **stages_test.go** (618 lines, ~5.5K tok) — Go package: resolver
- **types.go** (269 lines, ~2.8K tok) — Package resolver provides LLM-driven mention resolution using a multi-stage pipeline.

## pkg/metrics/ (~8.4K tokens)

- **metrics.go** (218 lines, ~1.8K tok) — Package metrics provides shared Prometheus metrics functionality for Go microservices.
- **metrics_test.go** (261 lines, ~1.8K tok) — Go package: metrics
- **middleware.go** (203 lines, ~1.5K tok) — Go package: metrics
- **middleware_test.go** (441 lines, ~3.3K tok) — Go package: metrics

## pkg/migration/validation/ (~28.8K tokens)

- **checklist.go** (447 lines, ~3.9K tok) — Package validation provides migration validation checklist functionality.
- **checklist_test.go** (585 lines, ~4.4K tok) — Go package: validation
- **report.go** (611 lines, ~5.2K tok) — Package validation provides validation reporting functionality.
- **report_test.go** (786 lines, ~5.9K tok) — Go package: validation
- **validator.go** (585 lines, ~4.7K tok) — Package validation provides a comprehensive validation suite to verify
- **validator_test.go** (651 lines, ~4.6K tok) — Go package: validation

## pkg/models/ (~4.4K tokens)

- **config_repository.go** (136 lines, ~1.0K tok) — Go package: models
- **config_repository_test.go** (370 lines, ~3.3K tok) — Go package: models

## pkg/parse/ (~22.6K tokens)

- **boilerplate_test.go** (200 lines, ~2.5K tok) — Go package: parse
- **email.go** (542 lines, ~5.5K tok) — Package parse provides deterministic parsing utilities for content processing.
- **email_test.go** (935 lines, ~9.0K tok) — Go package: parse
- **transcript.go** (372 lines, ~2.7K tok) — Package parse provides deterministic parsing utilities for the SLM/LLM pipeline Stage 0.
- **transcript_test.go** (467 lines, ~3.0K tok) — Go package: parse

## pkg/pipeline/ (~31.1K tokens)

- **batch_test.go** (260 lines, ~2.2K tok) — Package pipeline provides types and repository for pipeline status and job tracking.
- **definitions.go** (393 lines, ~3.9K tok) — Go package: pipeline
- **definitions_test.go** (32 lines, ~313 tok) — Go package: pipeline
- **repository.go** (1311 lines, ~10.9K tok) — Package pipeline provides types and repository for pipeline status and job tracking.
- **repository_diff_test.go** (401 lines, ~3.5K tok) — Package pipeline provides types and repository for pipeline status and job tracking.
- **repository_stage_io_test.go** (141 lines, ~1.3K tok) — Package pipeline provides types and repository for pipeline status and job tracking.
- **status.go** (325 lines, ~3.2K tok) — Package pipeline provides types and repository for pipeline status and job tracking.
- **status_test.go** (370 lines, ~3.5K tok) — Go package: pipeline
- **types.go** (153 lines, ~1.2K tok) — Package pipeline provides types and repository for pipeline status and job tracking.
- **validation.go** (43 lines, ~335 tok) — Go package: pipeline
- **validation_test.go** (103 lines, ~850 tok) — Go package: pipeline

## pkg/products/ (~36.7K tokens)

- **event_repository.go** (520 lines, ~4.0K tok) — Go package: products
- **event_repository_test.go** (420 lines, ~3.4K tok) — Go package: products
- **navigation_test.go** (493 lines, ~4.1K tok) — Go package: products
- **queries_test.go** (435 lines, ~4.0K tok) — Go package: products
- **query.go** (500 lines, ~4.5K tok) — Package products provides product management functionality.
- **query_test.go** (384 lines, ~2.8K tok) — Go package: products
- **repository.go** (523 lines, ~3.9K tok) — Go package: products
- **repository_test.go** (229 lines, ~1.8K tok) — Go package: products
- **team_repository.go** (516 lines, ~4.3K tok) — Go package: products
- **team_repository_test.go** (261 lines, ~1.8K tok) — Go package: products
- **types.go** (215 lines, ~2.2K tok) — Package products provides types and repository for product management.

## pkg/projects/ (~5.7K tokens)

- **repository.go** (562 lines, ~4.1K tok) — Go package: projects
- **repository_test.go** (134 lines, ~986 tok) — Go package: projects
- **types.go** (60 lines, ~637 tok) — Package projects provides types and repository for project management.

## pkg/relationships/ (~24.1K tokens)

- **repository.go** (1642 lines, ~13.7K tok) — Go package: relationships
- **repository_dedup_test.go** (532 lines, ~4.3K tok) — Go package: relationships
- **repository_metadata_test.go** (286 lines, ~3.0K tok) — Go package: relationships
- **types.go** (289 lines, ~3.1K tok) — Package relationships provides domain types and repository for relationship graph operations.

## pkg/repository/ (~11.7K tokens)

- **series_repository.go** (812 lines, ~5.9K tok) — Go package: repository
- **series_repository_test.go** (736 lines, ~5.8K tok) — Go package: repository

## pkg/reviewqueue/ (~11.8K tokens)

- **repository.go** (405 lines, ~3.2K tok) — Go package: reviewqueue
- **repository_test.go** (281 lines, ~2.4K tok) — Go package: reviewqueue
- **session.go** (430 lines, ~4.4K tok) — Go package: reviewqueue
- **types.go** (191 lines, ~1.8K tok) — Package reviewqueue provides a queue for AI questions requiring human review.

## pkg/schedule/ (~6.8K tokens)

- **repository.go** (290 lines, ~2.7K tok) — Package schedule provides the repository and Temporal wrapper for DB-driven schedule management.
- **repository_execution_test.go** (81 lines, ~594 tok) — Go package: schedule
- **temporal.go** (214 lines, ~2.0K tok) — Go package: schedule
- **temporal_trigger_test.go** (180 lines, ~1.6K tok) — Go package: schedule

## pkg/source_mappings/ (~2.0K tokens)

- **repository.go** (247 lines, ~2.0K tok) — Package source_mappings provides database operations for project source mappings.

## pkg/sources/ (~1.1K tokens)

- **repository.go** (138 lines, ~1.1K tok) — Package sources provides database access for content sources.

## pkg/temporal/ (~73.1K tokens)

- **activities.go** (295 lines, ~3.4K tok) — Go package: temporal
- **activities_test.go** (203 lines, ~1.9K tok) — Go package: temporal
- **client.go** (109 lines, ~852 tok) — Package temporal provides Temporal client factory and configuration for Penfold services.
- **client_test.go** (81 lines, ~604 tok) — Go package: temporal
- **config.go** (51 lines, ~542 tok) — Go package: temporal
- **config_test.go** (176 lines, ~1.6K tok) — Go package: temporal
- **dispatch_loop.go** (134 lines, ~1.1K tok) — Go package: temporal
- **dispatch_loop_test.go** (362 lines, ~3.4K tok) — Go package: temporal
- **executor_code_only.go** (100 lines, ~988 tok) — Go package: temporal
- **executor_code_only_test.go** (238 lines, ~2.0K tok) — Go package: temporal
- **executor_embedding.go** (117 lines, ~1.3K tok) — Go package: temporal
- **executor_embedding_test.go** (185 lines, ~1.8K tok) — Go package: temporal
- **executor_llm.go** (210 lines, ~2.1K tok) — Go package: temporal
- **executor_llm_test.go** (601 lines, ~6.0K tok) — Go package: temporal
- **executor_registry.go** (124 lines, ~1.2K tok) — Go package: temporal
- **executor_registry_test.go** (262 lines, ~2.3K tok) — Go package: temporal
- **executor_structured_extract.go** (206 lines, ~2.1K tok) — Go package: temporal
- **executor_structured_extract_test.go** (496 lines, ~4.6K tok) — Go package: temporal
- **options.go** (196 lines, ~1.9K tok) — Package temporal provides activity and workflow option presets for Penfold services.
- **options_test.go** (388 lines, ~3.6K tok) — Go package: temporal
- **pipeline.go** (102 lines, ~1.1K tok) — Go package: temporal
- **pipeline_dispatch_integration_test.go** (513 lines, ~6.1K tok) — Go package: temporal
- **pipeline_test.go** (116 lines, ~904 tok) — Go package: temporal
- **stage_dependencies.go** (100 lines, ~858 tok) — Go package: temporal
- **stage_dependencies_test.go** (236 lines, ~2.0K tok) — Go package: temporal
- **stage_executor.go** (101 lines, ~1.4K tok) — Go package: temporal
- **stage_executor_test.go** (242 lines, ~2.0K tok) — Go package: temporal
- **starter.go** (142 lines, ~1.4K tok) — Go package: temporal
- **starter_test.go** (464 lines, ~3.8K tok) — Go package: temporal
- **types.go** (486 lines, ~6.0K tok) — Go package: temporal
- **worker.go** (150 lines, ~1.4K tok) — Package temporal provides Temporal worker creation and configuration for Penfold services.
- **worker_test.go** (329 lines, ~2.9K tok) — Go package: temporal
- **workflows.go** (35 lines, ~410 tok) — Go package: temporal

## pkg/temporal/observability/ (~34.1K tokens)

- **dashboard.go** (581 lines, ~5.6K tok) — Go package: observability
- **dashboard_test.go** (619 lines, ~4.5K tok) — Go package: observability
- **interceptors.go** (648 lines, ~5.6K tok) — Go package: observability
- **interceptors_test.go** (480 lines, ~3.5K tok) — Go package: observability
- **logging.go** (245 lines, ~2.3K tok) — Go package: observability
- **logging_test.go** (167 lines, ~1.1K tok) — Go package: observability
- **metrics.go** (347 lines, ~3.0K tok) — Package observability provides shared metrics, tracing, and logging interceptors for Temporal workflows.
- **metrics_test.go** (320 lines, ~2.7K tok) — Go package: observability
- **tracing.go** (304 lines, ~2.6K tok) — Go package: observability
- **tracing_test.go** (455 lines, ~3.2K tok) — Go package: observability

## pkg/tenant/ (~2.8K tokens)

- **repository.go** (351 lines, ~2.4K tok) — Go package: tenant
- **types.go** (36 lines, ~386 tok) — Package tenant provides multi-tenant management for Penfold.

## pkg/testfixtures/ (~4.8K tokens)

- **loader.go** (336 lines, ~3.1K tok) — Package testfixtures provides data types and loaders for test fixtures.
- **types.go** (82 lines, ~723 tok) — Package testfixtures provides data types and loaders for test fixtures.
- **validate_test.go** (130 lines, ~988 tok) — Go package: testfixtures

## pkg/timeout/ (~5.4K tokens)

- **config.go** (277 lines, ~2.1K tok) — Package timeout provides runtime configuration management for pipeline timeouts.
- **config_test.go** (496 lines, ~3.3K tok) — Go package: timeout

## pkg/timeouts/ (~1.3K tokens)

- **timeouts.go** (56 lines, ~523 tok) — Package timeouts provides centralized timeout constants for the AI/LLM request path.
- **timeouts_test.go** (96 lines, ~744 tok) — Go package: timeouts

## pkg/topics/ (~3.2K tokens)

- **repository.go** (396 lines, ~2.8K tok) — Go package: topics
- **types.go** (32 lines, ~323 tok) — Package topics provides types and repository for topic management.

## pkg/tracing/ (~20.6K tokens)

- **ai.go** (402 lines, ~3.7K tok) — Package tracing provides AI-specific tracing helpers for OTel integration.
- **ai_test.go** (424 lines, ~3.7K tok) — Go package: tracing
- **helpers.go** (197 lines, ~1.7K tok) — Go package: tracing
- **middleware.go** (500 lines, ~3.8K tok) — Go package: tracing
- **tracing.go** (213 lines, ~1.7K tok) — Package tracing provides shared OpenTelemetry tracing functionality for Go microservices.
- **tracing_test.go** (861 lines, ~5.9K tok) — Go package: tracing

## pkg/watchlist/ (~5.9K tokens)

- **repository.go** (396 lines, ~3.0K tok) — Go package: watchlist
- **repository_test.go** (261 lines, ~2.3K tok) — Go package: watchlist
- **types.go** (56 lines, ~626 tok) — Package watchlist provides types and repository for watch list and priority management.

## processes/ (~1.5K tokens)

- **acronym-review.md** (179 lines, ~1.5K tok) — Workflow: Acronym Review

## project-lifecycle/ (~2.5K tokens)

- **agent-management.md** (281 lines, ~2.5K tok) — Agent Lifecycle Management

## review/arch/2026-01-16T13-32-45Z/ (~30.1K tokens)

- **pass-00-context.md** (300 lines, ~3.2K tok) — Architecture Review: Context & Goals
- **pass-01-structure.md** (291 lines, ~2.9K tok) — Architecture Review: Structure & Patterns
- **pass-02-security.md** (432 lines, ~3.6K tok) — Architecture Review: Security & Data Flow
- **pass-03-scalability.md** (422 lines, ~4.0K tok) — Architecture Review: Scalability & Performance
- **pass-04-maintainability.md** (442 lines, ~3.9K tok) — Architecture Review: Maintainability & Testing
- **pass-05-docs-audit.md** (350 lines, ~2.6K tok) — Documentation Audit Report
- **pass-06-meta-review.md** (652 lines, ~6.7K tok) — Architecture Review: Meta-Review & Consolidation
- **pass-07-synthesis.md** (253 lines, ~3.2K tok) — Architecture Review: Synthesis & Action Plan

## review/arch/2026-01-23T08-47-58Z/ (~29.3K tokens)

- **pass-00-context.md** (276 lines, ~3.3K tok) — Architecture Review: Context & Goals
- **pass-01-structure.md** (292 lines, ~3.5K tok) — Architecture Review: Structure & Patterns
- **pass-02-security.md** (363 lines, ~3.8K tok) — Architecture Review: Security & Data Flow
- **pass-03-scalability.md** (461 lines, ~4.2K tok) — Architecture Review: Scalability & Performance
- **pass-04-maintainability.md** (429 lines, ~4.3K tok) — Architecture Review: Maintainability & Testing
- **pass-05-docs-audit.md** (245 lines, ~3.0K tok) — Architecture Review: Documentation Audit
- **pass-06-meta-review.md** (397 lines, ~4.5K tok) — Architecture Review: Meta-Review & Consolidation
- **pass-07-synthesis.md** (214 lines, ~2.8K tok) — Architecture Review: Synthesis & Action Plan

## scripts/ (~27.2K tokens)

- **audit-context-injection.sh** (130 lines, ~1.3K tok) — audit-context-injection.sh in scripts/
- **cleanup_orphan_tenants.sql** (267 lines, ~2.8K tok) — Penfold: Orphan Tenant Cleanup Script
- **deploy-ai-coordinator.sh** (169 lines, ~1.4K tok) — deploy-ai-coordinator.sh in scripts/
- **deploy-gateway.sh** (221 lines, ~1.7K tok) — deploy-gateway.sh in scripts/
- **deploy-mcp.sh** (156 lines, ~1.2K tok) — deploy-mcp.sh in scripts/
- **deploy-worker.sh** (194 lines, ~1.5K tok) — deploy-worker.sh in scripts/
- **deploy.sh** (183 lines, ~1.6K tok) — deploy.sh in scripts/
- **docker-compose.temporal-dev02.yml** (52 lines, ~465 tok) — Temporal Server for Penfold - Deployment on dev02
- **rollback.sh** (23 lines, ~154 tok) — rollback.sh in scripts/
- **run-tests.sh** (182 lines, ~1.4K tok) — run-tests.sh in scripts/
- **services.sh** (245 lines, ~2.1K tok) — services.sh in scripts/
- **setup-test-tenant.sh** (253 lines, ~2.0K tok) — setup-test-tenant.sh in scripts/
- **setup_test_db.sh** (146 lines, ~1.5K tok) — setup_test_db.sh in scripts/
- **test-migrations.sh** (155 lines, ~1.2K tok) — test-migrations.sh in scripts/
- **verify-deployment.sh** (543 lines, ~4.9K tok) — verify-deployment.sh in scripts/
- **verify-mcp.sh** (199 lines, ~2.0K tok) — verify-mcp.sh in scripts/

## scripts/certs/ (~3.7K tokens)

- **create-ca.sh** (204 lines, ~1.5K tok) — create-ca.sh in scripts/certs/
- **create-client-cert.sh** (286 lines, ~2.2K tok) — create-client-cert.sh in scripts/certs/

## scripts/lib/ (~2.3K tokens)

- **deploy-common.sh** (238 lines, ~2.3K tok) — deploy-common.sh in scripts/lib/

## services/ai/ (~9.9K tokens)

- **Makefile** (84 lines, ~432 tok) — Makefile in services/ai/
- **go.mod** (58 lines, ~662 tok) — go.mod in services/ai/
- **go.sum** (116 lines, ~2.8K tok) — go.sum in services/ai/
- **main.go** (437 lines, ~4.1K tok) — Package main provides the entry point for the AI Coordinator service.
- **main_test.go** (219 lines, ~1.9K tok) — Go package: main

## services/ai/backend/ (~46.8K tokens)

- **anthropic.go** (367 lines, ~3.1K tok) — Package backend provides backend connectors for AI services.
- **anthropic_test.go** (162 lines, ~1.5K tok) — Go package: backend
- **backend.go** (131 lines, ~1.2K tok) — Package backend provides backend connectors for AI services.
- **composite.go** (177 lines, ~1.6K tok) — Package backend provides backend connectors for AI services.
- **composite_test.go** (293 lines, ~2.9K tok) — Go package: backend_test
- **config.go** (63 lines, ~557 tok) — Go package: backend
- **errors.go** (35 lines, ~371 tok) — Package backend provides backend connectors for AI services.
- **gemini.go** (560 lines, ~4.7K tok) — Package backend provides backend connectors for AI services.
- **gemini_test.go** (1022 lines, ~8.5K tok) — Go package: backend
- **mlx.go** (739 lines, ~6.2K tok) — Package backend provides backend connectors for AI services.
- **mlx_test.go** (753 lines, ~6.4K tok) — Go package: backend
- **openai.go** (501 lines, ~4.1K tok) — Package backend provides backend connectors for AI services.
- **openai_test.go** (693 lines, ~5.7K tok) — Go package: backend

## services/ai/config/ (~12.9K tokens)

- **config.go** (328 lines, ~2.7K tok) — Package config provides service-specific configuration for the AI Coordinator service.
- **config_test.go** (511 lines, ~4.4K tok) — Package config provides unit tests for service configuration.
- **db_config.go** (181 lines, ~1.7K tok) — Go package: config
- **db_config_test.go** (440 lines, ~4.0K tok) — Package config provides unit tests for database-backed dynamic model config resolution.

## services/ai/cost/ (~28.2K tokens)

- **alerts.go** (464 lines, ~3.8K tok) — Go package: cost
- **alerts_test.go** (382 lines, ~2.6K tok) — Go package: cost
- **models.go** (404 lines, ~3.4K tok) — Package cost provides cost tracking and budget management for AI operations.
- **models_test.go** (268 lines, ~2.0K tok) — Go package: cost
- **pricing.go** (339 lines, ~3.0K tok) — Go package: cost
- **pricing_test.go** (239 lines, ~1.8K tok) — Go package: cost
- **storage.go** (624 lines, ~5.4K tok) — Go package: cost
- **tracker.go** (552 lines, ~4.3K tok) — Go package: cost
- **tracker_test.go** (275 lines, ~2.0K tok) — Go package: cost

## services/ai/ensemble/ (~28.1K tokens)

- **aggregation.go** (574 lines, ~4.3K tok) — Go package: ensemble
- **config.go** (448 lines, ~3.6K tok) — Go package: ensemble
- **ensemble_test.go** (818 lines, ~5.9K tok) — Go package: ensemble
- **orchestration.go** (620 lines, ~4.4K tok) — Go package: ensemble
- **processor.go** (708 lines, ~5.8K tok) — Package ensemble provides ensemble processing capabilities for AI model coordination.
- **strategies.go** (506 lines, ~4.2K tok) — Go package: ensemble

## services/ai/escalation/ (~30.0K tokens)

- **escalation_test.go** (832 lines, ~7.0K tok) — Go package: escalation
- **manager.go** (718 lines, ~5.2K tok) — Go package: escalation
- **metrics.go** (619 lines, ~4.9K tok) — Go package: escalation
- **policies.go** (651 lines, ~5.4K tok) — Go package: escalation
- **tiers.go** (334 lines, ~2.7K tok) — Package escalation provides automatic escalation management for AI model requests.
- **triggers.go** (561 lines, ~4.7K tok) — Go package: escalation

## services/ai/integration/ (~7.2K tokens)

- **integration_test.go** (863 lines, ~7.2K tok) — Package integration provides integration tests for the AI Coordinator service.

## services/ai/registry/ (~31.4K tokens)

- **db_registry.go** (333 lines, ~2.6K tok) — Go package: registry
- **defaults.go** (359 lines, ~2.9K tok) — Go package: registry
- **errors.go** (43 lines, ~473 tok) — Go package: registry
- **model.go** (272 lines, ~2.7K tok) — Package registry provides model registration and discovery for the AI Coordinator.
- **registry.go** (548 lines, ~3.7K tok) — Go package: registry
- **registry_test.go** (762 lines, ~5.5K tok) — Go package: registry
- **repository.go** (808 lines, ~5.7K tok) — Go package: registry
- **repository_test.go** (702 lines, ~4.8K tok) — Go package: registry
- **routing.go** (191 lines, ~1.8K tok) — Go package: registry
- **routing_test.go** (144 lines, ~1.2K tok) — Go package: registry

## services/ai/router/ (~17.9K tokens)

- **circuit.go** (403 lines, ~2.9K tok) — Package router provides AI model routing with circuit breaker fault tolerance.
- **router.go** (966 lines, ~6.8K tok) — Go package: router
- **router_test.go** (1165 lines, ~8.2K tok) — Go package: router

## services/ai/selector/ (~15.3K tokens)

- **criteria.go** (431 lines, ~3.7K tok) — Package selector provides AI model selection logic based on task requirements,
- **selector.go** (745 lines, ~6.1K tok) — Go package: selector
- **selector_test.go** (682 lines, ~5.5K tok) — Go package: selector

## services/ai/server/ (~102.2K tokens)

- **analyze.go** (732 lines, ~6.7K tok) — Go package: server
- **analyze_test.go** (562 lines, ~4.3K tok) — Go package: server
- **bug_pf_16edb8_test.go** (179 lines, ~2.2K tok) — Package server contains reproduction tests for bug pf-16edb8.
- **bug_pf_37ff52_test.go** (389 lines, ~4.6K tok) — Package server contains reproduction tests for bug pf-37ff52.
- **bug_pf_5a2e1a_test.go** (404 lines, ~4.4K tok) — Package server — reproduction tests for pf-5a2e1a: model selection routing bug.
- **bug_pf_63609f_test.go** (180 lines, ~2.2K tok) — Package server contains reproduction tests for bug pf-63609f.
- **bug_pf_6455b9_test.go** (117 lines, ~1.2K tok) — Package server contains a reproduction test for bug pf-6455b9.
- **bug_pf_73ed30_test.go** (208 lines, ~2.4K tok) — Package server contains reproduction tests for pf-73ed30:
- **bug_pf_93373b_test.go** (221 lines, ~2.3K tok) — Package server contains a reproduction test for bug pf-93373b.
- **bug_pf_9b64d2_test.go** (221 lines, ~2.6K tok) — Package server contains a reproduction test for bug pf-9b64d2.
- **bug_pf_cbed3a_test.go** (264 lines, ~2.7K tok) — Package server contains a reproduction test for bug pf-cbed3a.
- **embedding_routing_test.go** (301 lines, ~3.3K tok) — Go package: server
- **extract.go** (684 lines, ~7.2K tok) — Go package: server
- **extract_test.go** (495 lines, ~4.3K tok) — Go package: server
- **langfuse_generation_test.go** (742 lines, ~7.8K tok) — Package server provides acceptance tests for pf-775cab:
- **model_selection_test.go** (569 lines, ~5.7K tok) — Package server provides unit tests for per-stage model selection and backend routing.
- **server.go** (2164 lines, ~20.1K tok) — Package server provides the gRPC server implementation for the AI Coordinator service.
- **server_test.go** (1457 lines, ~12.0K tok) — Package server provides unit tests for the AI Coordinator gRPC server.
- **triage.go** (145 lines, ~1.7K tok) — Go package: server
- **triage_test.go** (531 lines, ~4.4K tok) — Go package: server

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

## services/gateway/contentservice/ (~44.2K tokens)

- **concurrency_reprocess_test.go** (501 lines, ~4.5K tok) — Go package: contentservice
- **langfuse_trace_test.go** (103 lines, ~904 tok) — Go package: contentservice
- **null_confidence_test.go** (120 lines, ~1.3K tok) — Go package: contentservice
- **purge_test.go** (117 lines, ~1.2K tok) — Go package: contentservice
- **reprocess_test.go** (247 lines, ~2.3K tok) — Go package: contentservice
- **service.go** (2765 lines, ~24.6K tok) — Package contentservice implements the ContentProcessorService gRPC server.
- **service_test.go** (994 lines, ~9.5K tok) — Go package: contentservice

## services/gateway/conversationservice/ (~57.0K tokens)

- **audit_repository.go** (257 lines, ~2.2K tok) — Go package: conversationservice
- **audit_test.go** (410 lines, ~3.9K tok) — Go package: conversationservice
- **merge_split_test.go** (557 lines, ~5.2K tok) — Go package: conversationservice
- **processing_status_test.go** (929 lines, ~10.7K tok) — Go package: conversationservice
- **repository.go** (915 lines, ~7.6K tok) — Package conversationservice repository
- **repository_test.go** (1099 lines, ~10.3K tok) — Go package: conversationservice
- **service.go** (783 lines, ~8.2K tok) — Package conversationservice implements the ConversationService gRPC server.
- **service_test.go** (819 lines, ~7.9K tok) — Go package: conversationservice
- **types.go** (122 lines, ~899 tok) — Package conversationservice types

## services/gateway/digestservice/ (~2.9K tokens)

- **service.go** (328 lines, ~2.9K tok) — Package digestservice implements the DigestService gRPC server.

## services/gateway/entitymanagementservice/ (~23.1K tokens)

- **service.go** (692 lines, ~6.8K tok) — Package entitymanagementservice implements the EntityManagementService gRPC server.
- **service_test.go** (1911 lines, ~16.3K tok) — Go package: entitymanagementservice

## services/gateway/entityservice/ (~15.9K tokens)

- **service.go** (491 lines, ~4.1K tok) — Package entityservice implements the EntityService gRPC server for bulk entity seeding.
- **service_test.go** (1389 lines, ~11.7K tok) — Go package: entityservice

## services/gateway/glossaryservice/ (~5.9K tokens)

- **service.go** (503 lines, ~4.4K tok) — Package glossaryservice implements the GlossaryService gRPC server.
- **service_test.go** (121 lines, ~1.5K tok) — Go package: glossaryservice

## services/gateway/ (~15.3K tokens)

- **go.mod** (77 lines, ~921 tok) — go.mod in services/gateway/
- **go.sum** (163 lines, ~4.0K tok) — go.sum in services/gateway/
- **main.go** (871 lines, ~10.1K tok) — Package main is the entry point for the API Gateway service.
- **tls.go** (49 lines, ~338 tok) — Package main provides TLS configuration helpers for the API Gateway service.

## services/gateway/graphservice/ (~7.2K tokens)

- **service.go** (492 lines, ~4.2K tok) — Package graphservice implements the GraphConnectorService gRPC server.
- **service_test.go** (379 lines, ~3.0K tok) — Go package: graphservice

## services/gateway/health/ (~10.1K tokens)

- **aggregator.go** (422 lines, ~3.3K tok) — Package health provides health check aggregation for the API Gateway.
- **aggregator_test.go** (806 lines, ~5.9K tok) — Go package: health
- **http_client.go** (101 lines, ~844 tok) — Package health provides health check clients for the API Gateway.

## services/gateway/ingestservice/ (~31.2K tokens)

- **series_test.go** (200 lines, ~1.8K tok) — Go package: ingestservice
- **service.go** (1888 lines, ~16.4K tok) — Package ingestservice implements the IngestService gRPC server.
- **service_test.go** (1563 lines, ~13.0K tok) — Go package: ingestservice

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

- **auth.go** (415 lines, ~3.5K tok) — Package middleware provides gRPC interceptors for the API Gateway.
- **auth_test.go** (962 lines, ~7.2K tok) — Go package: middleware
- **csrf.go** (283 lines, ~2.4K tok) — Package middleware provides HTTP middleware for the API Gateway.
- **csrf_test.go** (757 lines, ~5.3K tok) — Go package: middleware

## services/gateway/modelservice/ (~16.6K tokens)

- **service.go** (1506 lines, ~12.8K tok) — Package modelservice implements the model management gRPC service proxy.
- **service_test.go** (459 lines, ~3.8K tok) — Go package: modelservice

## services/gateway/orchestrator/ (~10.5K tokens)

- **starter.go** (522 lines, ~4.8K tok) — Package orchestrator provides a bridge between gRPC services and Temporal workflows.
- **starter_test.go** (646 lines, ~5.6K tok) — Go package: orchestrator

## services/gateway/pipelineservice/ (~60.1K tokens)

- **backpressure_test.go** (69 lines, ~671 tok) — Go package: pipelineservice
- **batch.go** (351 lines, ~3.3K tok) — Go package: pipelineservice
- **batch_test.go** (123 lines, ~1.1K tok) — Go package: pipelineservice
- **concurrency_test.go** (462 lines, ~4.4K tok) — Go package: pipelineservice
- **concurrency_unit_test.go** (233 lines, ~2.0K tok) — Go package: pipelineservice
- **definitions.go** (400 lines, ~3.7K tok) — Go package: pipelineservice
- **definitions_test.go** (160 lines, ~1.3K tok) — Go package: pipelineservice
- **diff_test.go** (160 lines, ~1.4K tok) — Go package: pipelineservice
- **errors.go** (164 lines, ~1.2K tok) — Go package: pipelineservice
- **errors_source_test.go** (136 lines, ~1.3K tok) — Go package: pipelineservice
- **errors_test.go** (151 lines, ~1.6K tok) — Go package: pipelineservice
- **inspect_test.go** (210 lines, ~1.9K tok) — Go package: pipelineservice
- **list_models_test.go** (203 lines, ~1.9K tok) — Go package: pipelineservice
- **operational_config_test.go** (118 lines, ~1.1K tok) — Go package: pipelineservice
- **service.go** (2759 lines, ~24.4K tok) — Package pipelineservice implements the PipelineService gRPC server.
- **service_test.go** (304 lines, ~2.9K tok) — Go package: pipelineservice
- **stage_config_test.go** (267 lines, ~2.5K tok) — Go package: pipelineservice
- **workflow_input_bug_test.go** (163 lines, ~2.0K tok) — Go package: pipelineservice
- **workflow_input_test.go** (149 lines, ~1.6K tok) — Go package: pipelineservice

## services/gateway/productservice/ (~14.6K tokens)

- **service.go** (1619 lines, ~14.6K tok) — Package productservice implements the ProductService gRPC server.

## services/gateway/projectservice/ (~4.3K tokens)

- **service.go** (477 lines, ~4.3K tok) — Package projectservice implements the ProjectService gRPC server.

## services/gateway/proxy/ (~13.2K tokens)

- **python.go** (815 lines, ~6.1K tok) — Package proxy provides HTTP proxying functionality for forwarding requests
- **python_test.go** (948 lines, ~7.1K tok) — Go package: proxy

## services/gateway/qualityservice/ (~7.6K tokens)

- **service.go** (277 lines, ~2.5K tok) — Package qualityservice implements the QualityService gRPC server.
- **service_test.go** (605 lines, ~5.1K tok) — Go package: qualityservice

## services/gateway/questionsservice/ (~5.4K tokens)

- **service.go** (603 lines, ~5.4K tok) — Package questionsservice implements the QuestionsService gRPC server.

## services/gateway/ratelimit/ (~13.2K tokens)

- **limiter.go** (420 lines, ~3.1K tok) — Package ratelimit provides rate limiting functionality for the API Gateway.
- **limiter_test.go** (467 lines, ~3.5K tok) — Go package: ratelimit
- **middleware.go** (342 lines, ~2.8K tok) — Go package: ratelimit
- **middleware_test.go** (471 lines, ~3.8K tok) — Go package: ratelimit

## services/gateway/relationshipservice/ (~21.1K tokens)

- **service.go** (1122 lines, ~11.1K tok) — Package relationshipservice implements the RelationshipService gRPC server.
- **service_dedup_test.go** (606 lines, ~4.9K tok) — Go package: relationshipservice
- **service_test.go** (542 lines, ~5.1K tok) — Go package: relationshipservice

## services/gateway/reviewservice/ (~7.8K tokens)

- **service.go** (722 lines, ~6.4K tok) — Package reviewservice implements the ReviewService gRPC server.
- **service_test.go** (154 lines, ~1.4K tok) — Go package: reviewservice

## services/gateway/router/ (~13.6K tokens)

- **router.go** (782 lines, ~6.3K tok) — Package router provides request routing functionality for the API Gateway.
- **router_test.go** (1002 lines, ~7.3K tok) — Go package: router

## services/gateway/scheduleservice/ (~6.5K tokens)

- **service.go** (439 lines, ~4.2K tok) — Package scheduleservice implements the ScheduleService gRPC server.
- **service_test.go** (243 lines, ~2.3K tok) — Go package: scheduleservice

## services/gateway/searchservice/ (~17.4K tokens)

- **service.go** (1138 lines, ~10.2K tok) — Package searchservice implements the SearchService gRPC for the Gateway.
- **service_test.go** (799 lines, ~7.2K tok) — Go package: searchservice

## services/gateway/server/ (~15.3K tokens)

- **server.go** (139 lines, ~1.5K tok) — Package server provides the gRPC server implementation for the API Gateway.
- **server_test.go** (1670 lines, ~13.8K tok) — Package server provides comprehensive tests for the API Gateway server.

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

- **config.go** (187 lines, ~1.6K tok) — Package workflows provides a bridge between the Gateway API and Temporal workflows.
- **config_test.go** (159 lines, ~1.5K tok) — Go package: workflows
- **handlers.go** (389 lines, ~3.2K tok) — Go package: workflows
- **handlers_test.go** (444 lines, ~3.3K tok) — Go package: workflows
- **mapping.go** (267 lines, ~2.4K tok) — Go package: workflows
- **mapping_test.go** (446 lines, ~3.6K tok) — Go package: workflows
- **starter.go** (393 lines, ~3.3K tok) — Go package: workflows
- **starter_test.go** (481 lines, ~3.6K tok) — Go package: workflows
- **status.go** (442 lines, ~3.5K tok) — Go package: workflows
- **status_test.go** (358 lines, ~2.8K tok) — Go package: workflows

## services/gateway/workflowservice/ (~3.6K tokens)

- **service.go** (380 lines, ~3.6K tok) — Package workflowservice implements the WorkflowService gRPC server.

## services/gmail/attachment/ (~16.6K tokens)

- **extractors.go** (676 lines, ~4.9K tok) — Package attachment provides text extraction from various attachment formats.
- **processor.go** (558 lines, ~5.1K tok) — Package attachment provides attachment processing for Gmail emails.
- **processor_test.go** (915 lines, ~6.6K tok) — Go package: attachment

## services/gmail/config/ (~5.0K tokens)

- **config.go** (153 lines, ~1.1K tok) — Package config provides Gmail Connector service-specific configuration.
- **config_test.go** (515 lines, ~3.8K tok) — Go package: config

## services/gmail/ (~4.1K tokens)

- **go.mod** (43 lines, ~418 tok) — go.mod in services/gmail/
- **go.sum** (93 lines, ~2.2K tok) — go.sum in services/gmail/
- **main.go** (196 lines, ~1.6K tok) — Package main is the entry point for the Gmail Connector gRPC service.

## services/gmail/oauth/ (~24.2K tokens)

- **encryption.go** (125 lines, ~1.1K tok) — Package oauth provides OAuth2 authentication with PKCE for Gmail API access.
- **encryption_test.go** (344 lines, ~2.3K tok) — Go package: oauth
- **oauth.go** (614 lines, ~5.1K tok) — Package oauth provides OAuth2 authentication with PKCE for Gmail API access.
- **oauth_test.go** (1301 lines, ~10.5K tok) — Go package: oauth
- **storage.go** (342 lines, ~2.6K tok) — Package oauth provides OAuth2 authentication with PKCE for Gmail API access.
- **storage_test.go** (354 lines, ~2.6K tok) — Go package: oauth

## services/gmail/privacy/ (~19.2K tokens)

- **filter.go** (832 lines, ~6.7K tok) — Package privacy provides privacy filtering and PII detection for Gmail content.
- **filter_test.go** (1237 lines, ~8.4K tok) — Go package: privacy
- **rules.go** (544 lines, ~4.2K tok) — Package privacy provides privacy filtering and PII detection for Gmail content.

## services/gmail/push/ (~31.0K tokens)

- **handler.go** (230 lines, ~1.9K tok) — Package push provides Gmail push notification handling via Cloud Pub/Sub.
- **metrics.go** (231 lines, ~2.5K tok) — Package push provides Gmail push notification handling via Cloud Pub/Sub.
- **processor.go** (439 lines, ~3.0K tok) — Package push provides Gmail push notification handling via Cloud Pub/Sub.
- **push_test.go** (1427 lines, ~11.6K tok) — Go package: push
- **server.go** (401 lines, ~3.3K tok) — Package push provides Gmail push notification handling via Cloud Pub/Sub.
- **storage.go** (547 lines, ~4.5K tok) — Package push provides Gmail push notification handling via Cloud Pub/Sub.
- **subscription.go** (495 lines, ~4.2K tok) — Package push provides Gmail push notification handling via Cloud Pub/Sub.

## services/gmail/scheduler/ (~27.7K tokens)

- **priority.go** (368 lines, ~2.9K tok) — Package scheduler provides intelligent task scheduling for Gmail sync operations.
- **scheduler.go** (837 lines, ~6.3K tok) — Package scheduler provides intelligent task scheduling for Gmail sync operations.
- **scheduler_test.go** (1055 lines, ~7.5K tok) — Go package: scheduler
- **throttle.go** (554 lines, ~3.7K tok) — Package scheduler provides intelligent task scheduling for Gmail sync operations.
- **timing.go** (429 lines, ~3.5K tok) — Package scheduler provides intelligent task scheduling for Gmail sync operations.
- **workers.go** (571 lines, ~3.9K tok) — Package scheduler provides intelligent task scheduling for Gmail sync operations.

## services/gmail/server/ (~968 tokens)

- **server.go** (90 lines, ~968 tok) — Package server provides the gRPC server implementation for the Gmail Connector service.

## services/gmail/sync/ (~27.5K tokens)

- **engine.go** (1726 lines, ~13.2K tok) — Package sync provides Gmail synchronization capabilities including
- **engine_test.go** (1140 lines, ~8.8K tok) — Go package: sync
- **state.go** (658 lines, ~5.4K tok) — Package sync provides Gmail synchronization capabilities including

## services/gmail/tests/ (~7.7K tokens)

- **integration_test.go** (954 lines, ~7.7K tok) — Package tests provides integration tests for the Gmail connector components.

## services/mcp/ (~31.3K tokens)

- **errors.go** (51 lines, ~441 tok) — Go package: main
- **errors_test.go** (120 lines, ~1.0K tok) — Go package: main
- **format.go** (142 lines, ~1.2K tok) — Go package: main
- **format_test.go** (189 lines, ~1.5K tok) — Go package: main
- **go.mod** (26 lines, ~239 tok) — go.mod in services/mcp/
- **go.sum** (53 lines, ~1.2K tok) — go.sum in services/mcp/
- **handler.go** (59 lines, ~539 tok) — Go package: main
- **health.go** (37 lines, ~363 tok) — Go package: main
- **logging.go** (31 lines, ~181 tok) — Go package: main
- **main.go** (200 lines, ~2.0K tok) — Go package: main
- **metatools.go** (110 lines, ~1.3K tok) — Go package: main
- **middleware.go** (67 lines, ~542 tok) — Go package: main
- **schema.go** (135 lines, ~1.0K tok) — Go package: main
- **schema_test.go** (181 lines, ~1.7K tok) — Go package: main
- **toolset.go** (297 lines, ~2.5K tok) — Go package: main
- **toolset_test.go** (256 lines, ~2.3K tok) — Go package: main
- **toolsets_content.go** (108 lines, ~1.1K tok) — Go package: main
- **toolsets_content_test.go** (98 lines, ~982 tok) — Go package: main
- **toolsets_entities.go** (71 lines, ~692 tok) — Go package: main
- **toolsets_entities_test.go** (86 lines, ~811 tok) — Go package: main
- **toolsets_knowledge.go** (73 lines, ~712 tok) — Go package: main
- **toolsets_knowledge_test.go** (83 lines, ~773 tok) — Go package: main
- **toolsets_ops.go** (82 lines, ~796 tok) — Go package: main
- **toolsets_ops_test.go** (84 lines, ~742 tok) — Go package: main
- **toolsets_search.go** (101 lines, ~1.1K tok) — Go package: main
- **toolsets_search_integration_test.go** (290 lines, ~2.4K tok) — Go package: main
- **toolsets_search_test.go** (131 lines, ~1.2K tok) — Go package: main
- **toolsets_workflow.go** (95 lines, ~907 tok) — Go package: main
- **toolsets_workflow_test.go** (91 lines, ~864 tok) — Go package: main

## services/worker/activities/ (~505.4K tokens)

- **activities.go** (489 lines, ~4.5K tok) — Package activities provides activity implementations for the Temporal worker.
- **activities_test.go** (377 lines, ~3.4K tok) — Package activities provides activity tests.
- **ai_activities.go** (617 lines, ~5.7K tok) — Package activities provides activity implementations for the Temporal worker.
- **analysis.go** (369 lines, ~3.2K tok) — Package activities provides activity implementations for the Temporal worker.
- **analysis_test.go** (422 lines, ~4.2K tok) — Package activities provides tests for analysis activities.
- **assertion_attribution_test.go** (476 lines, ~4.3K tok) — Go package: activities
- **assertion_context_bug_test.go** (245 lines, ~3.0K tok) — Package activities contains a reproduction test for bug pf-90b749.
- **assertion_dedup_edge_test.go** (263 lines, ~2.6K tok) — Go package: activities
- **assertion_dedup_test.go** (560 lines, ~5.1K tok) — Go package: activities
- **assertion_repo.go** (273 lines, ~2.1K tok) — Package activities provides activity implementations for the Temporal worker.
- **assertion_repo_integration_test.go** (230 lines, ~2.1K tok) — Go package: activities
- **assertion_repo_stale_test.go** (308 lines, ~3.1K tok) — Go package: activities
- **assertion_repo_test.go** (56 lines, ~339 tok) — Go package: activities
- **attribution.go** (299 lines, ~3.0K tok) — Package activities provides activity implementations for the Temporal worker.
- **bug_pf_16edb8_test.go** (143 lines, ~1.5K tok) — Package activities contains a reproduction test for bug pf-16edb8.
- **build_stage_context_integration_test.go** (551 lines, ~5.8K tok) — Go package: activities
- **build_stage_context_test.go** (305 lines, ~3.1K tok) — Go package: activities
- **calendar_empty_body_test.go** (313 lines, ~3.2K tok) — Package activities provides tests for calendar invite processing with empty body (pf-479452).
- **classify_source.go** (187 lines, ~1.7K tok) — Package activities provides activity implementations for the Temporal worker.
- **classify_source_test.go** (350 lines, ~3.4K tok) — Package activities provides activity tests for source classification.
- **consolidation_activities.go** (333 lines, ~2.9K tok) — Go package: activities
- **consolidation_activities_test.go** (160 lines, ~1.2K tok) — Go package: activities
- **constructor_test.go** (348 lines, ~3.6K tok) — Package activities provides tests for activity constructor nil checks.
- **content_activities.go** (773 lines, ~6.5K tok) — Package activities provides activity implementations for the Temporal worker.
- **context_builder.go** (1003 lines, ~10.1K tok) — Go package: activities
- **context_builder_parity_test.go** (465 lines, ~4.8K tok) — Go package: activities
- **context_builder_test.go** (1899 lines, ~17.2K tok) — Go package: activities
- **context_provider.go** (69 lines, ~778 tok) — Go package: activities
- **context_provider_test.go** (35 lines, ~242 tok) — Go package: activities
- **context_repo.go** (437 lines, ~3.3K tok) — Go package: activities
- **context_repo_test.go** (394 lines, ~3.1K tok) — Go package: activities
- **conversation_activities.go** (932 lines, ~8.4K tok) — Go package: activities
- **conversation_activities_test.go** (1891 lines, ~20.8K tok) — Package activities provides tests for conversation auto-linking activities.
- **conversation_repo.go** (332 lines, ~3.3K tok) — Go package: activities
- **digest.go** (964 lines, ~9.7K tok) — Go package: activities
- **digest_rollup_activities.go** (594 lines, ~5.9K tok) — Package activities provides activity implementations for the Temporal worker.
- **digest_rollup_activities_test.go** (251 lines, ~2.4K tok) — Go package: activities
- **domain_company_repo.go** (48 lines, ~398 tok) — Go package: activities
- **email.go** (111 lines, ~958 tok) — Package activities provides activity implementations for the Temporal worker.
- **email_test.go** (138 lines, ~1.2K tok) — Go package: activities
- **embedding.go** (481 lines, ~4.5K tok) — Package activities provides activity implementations for the Temporal worker.
- **embedding_chunk_config_test.go** (94 lines, ~817 tok) — Go package: activities
- **embedding_empty_content_test.go** (94 lines, ~838 tok) — Go package: activities
- **embedding_repo.go** (525 lines, ~3.9K tok) — Package activities provides activity implementations for the Temporal worker.
- **embedding_repo_test.go** (241 lines, ~2.1K tok) — Go package: activities
- **enrich_entities_activity.go** (201 lines, ~1.8K tok) — Go package: activities
- **enrichment_activities.go** (161 lines, ~1.5K tok) — Package activities provides activity implementations for the Temporal worker.
- **enrichment_activities_test.go** (197 lines, ~1.7K tok) — Go package: activities
- **enrichment_repository.go** (74 lines, ~628 tok) — Package activities provides activity implementations for the Temporal worker.
- **entity_enrichment.go** (353 lines, ~3.3K tok) — Go package: activities
- **entity_enrichment_test.go** (1151 lines, ~10.5K tok) — Go package: activities
- **entity_repo.go** (82 lines, ~776 tok) — Package activities provides activity implementations for the Temporal worker.
- **error_classification_test.go** (215 lines, ~1.8K tok) — Go package: activities
- **error_type_bug_test.go** (184 lines, ~1.9K tok) — Package activities provides activity tests.
- **errors.go** (181 lines, ~1.8K tok) — Package activities provides activity implementations for the Temporal worker.
- **errors_test.go** (210 lines, ~2.0K tok) — Go package: activities
- **extraction.go** (1304 lines, ~13.3K tok) — Package activities provides activity implementations for the Temporal worker.
- **extraction_heartbeat_test.go** (219 lines, ~2.6K tok) — Package activities provides tests for the heartbeat and detached context fixes (pf-04a2de).
- **extraction_test.go** (1680 lines, ~18.5K tok) — Package activities provides tests for extraction activities.
- **gmail_activities.go** (727 lines, ~6.6K tok) — Package activities provides activity implementations for the Temporal worker.
- **graph_activities.go** (1576 lines, ~15.0K tok) — Package activities provides activity implementations for the Temporal worker.
- **header_mentions.go** (305 lines, ~2.6K tok) — Package activities provides activity implementations for the Temporal worker.
- **heartbeat_activities.go** (139 lines, ~1.4K tok) — Go package: activities
- **heartbeat_querier.go** (54 lines, ~455 tok) — Go package: activities
- **instruction_evaluation.go** (381 lines, ~3.3K tok) — Go package: activities
- **instruction_evaluation_repo.go** (94 lines, ~754 tok) — Go package: activities
- **interfaces.go** (583 lines, ~7.0K tok) — Package activities provides activity implementations for the Temporal worker.
- **journal_rollup_activities.go** (661 lines, ~6.5K tok) — Package activities provides activity implementations for the Temporal worker.
- **journal_rollup_activities_test.go** (224 lines, ~2.2K tok) — Go package: activities
- **langfuse_activities.go** (241 lines, ~2.3K tok) — Package activities provides Langfuse ingestion activities for the pipeline workflow.
- **langfuse_activities_test.go** (477 lines, ~5.5K tok) — Package activities provides tests for Langfuse trace enrichment (pf-427836).
- **langfuse_metadata_test.go** (292 lines, ~3.4K tok) — Package activities provides acceptance tests for pf-37ebe8:
- **mentions_activities.go** (306 lines, ~2.8K tok) — Package activities provides activity implementations for the Temporal worker.
- **message_counts_test.go** (451 lines, ~4.4K tok) — Go package: activities
- **multilevel_embedding.go** (532 lines, ~4.8K tok) — Package activities provides activity implementations for the Temporal worker.
- **multilevel_embedding_test.go** (858 lines, ~9.0K tok) — Go package: activities
- **newsletter_context_repo.go** (82 lines, ~727 tok) — Go package: activities
- **newsletter_extract_activities.go** (232 lines, ~2.2K tok) — Package activities provides activity implementations for the Temporal worker.
- **newsletter_helpers.go** (47 lines, ~431 tok) — Go package: activities
- **notification.go** (363 lines, ~3.3K tok) — Package activities provides activity implementations for the Temporal worker.
- **notification_contribution_cap_test.go** (229 lines, ~2.8K tok) — Package activities provides a reproduction test for bug pf-1c083d.
- **notification_extract_activities.go** (159 lines, ~1.5K tok) — Package activities provides activity implementations for the Temporal worker.
- **operational_config_repo.go** (74 lines, ~571 tok) — Go package: activities
- **parse_activities.go** (191 lines, ~1.7K tok) — Package activities provides activity implementations for the Temporal worker.
- **parse_activities_test.go** (209 lines, ~1.6K tok) — Package activities provides tests for parse activities.
- **persist_activities.go** (182 lines, ~1.7K tok) — Package activities provides activity implementations for the Temporal worker.
- **persist_activities_test.go** (258 lines, ~2.3K tok) — Package activities provides tests for persist activities.
- **persist_lifecycle_validation_test.go** (271 lines, ~2.9K tok) — Go package: activities
- **persist_pf197a0f_test.go** (212 lines, ~1.8K tok) — Go package: activities
- **persist_repo.go** (1244 lines, ~12.1K tok) — Go package: activities
- **persist_repo_confidence_test.go** (120 lines, ~1.4K tok) — Go package: activities
- **persist_repo_cross_thread_dedup_test.go** (477 lines, ~4.6K tok) — Go package: activities
- **persist_repo_significance_bug_test.go** (86 lines, ~568 tok) — Go package: activities
- **persist_repo_stale_assertions_test.go** (315 lines, ~3.4K tok) — Go package: activities
- **persist_repo_test.go** (1625 lines, ~18.1K tok) — Go package: activities
- **person_lookup.go** (51 lines, ~513 tok) — Package activities provides activity implementations for the Temporal worker.
- **pipeline_activities.go** (387 lines, ~3.8K tok) — Package activities provides activity implementations for the Temporal worker.
- **pipeline_activities_test.go** (622 lines, ~7.9K tok) — Go package: activities
- **pipeline_repo.go** (52 lines, ~539 tok) — Package activities provides repository wrappers for activity implementations.
- **pipeline_run_wiring_test.go** (342 lines, ~3.9K tok) — Package activities provides tests verifying model_id and token counts are correctly
- **preclassify_activities.go** (103 lines, ~955 tok) — Package activities provides activity implementations for the Temporal worker.
- **preclassify_activities_test.go** (186 lines, ~1.6K tok) — Go package: activities
- **project_tagging.go** (179 lines, ~1.8K tok) — Package activities provides activity implementations for the Temporal worker.
- **project_tagging_test.go** (623 lines, ~5.8K tok) — Go package: activities
- **provider_active_projects.go** (43 lines, ~365 tok) — Go package: activities
- **provider_glossary.go** (77 lines, ~583 tok) — Go package: activities
- **provider_test.go** (474 lines, ~4.8K tok) — Go package: activities
- **provider_topics.go** (98 lines, ~742 tok) — Go package: activities
- **provider_tracked_products.go** (54 lines, ~499 tok) — Go package: activities
- **provider_tracked_products_test.go** (73 lines, ~683 tok) — Go package: activities
- **provider_user_context.go** (53 lines, ~438 tok) — Go package: activities
- **register.go** (932 lines, ~10.2K tok) — Package activities provides activity registration for the Temporal worker.
- **register_test.go** (233 lines, ~2.0K tok) — Package activities provides tests for activity registration.
- **register_verification_test.go** (554 lines, ~5.6K tok) — Package activities provides tests for activity registration verification.
- **source.go** (244 lines, ~2.2K tok) — Package activities provides activity implementations for the Temporal worker.
- **source_repo.go** (140 lines, ~1.3K tok) — Go package: activities
- **source_system_integration_test.go** (268 lines, ~3.0K tok) — Go package: activities
- **source_test.go** (532 lines, ~6.0K tok) — Package activities provides activity tests.
- **stage_io_test.go** (208 lines, ~1.8K tok) — Go package: activities
- **storage.go** (432 lines, ~3.9K tok) — Package activities provides activity implementations for the Temporal worker.
- **structured_extract_activities.go** (193 lines, ~1.9K tok) — Package activities provides activity implementations for the Temporal worker.
- **structured_extract_test.go** (282 lines, ~3.1K tok) — Package activities provides unit tests for the StructuredExtract activity.
- **summarization.go** (350 lines, ~3.0K tok) — Package activities provides activity implementations for the Temporal worker.
- **summary_repo.go** (156 lines, ~1.2K tok) — Package activities provides activity implementations for the Temporal worker.
- **thread_activities.go** (279 lines, ~2.4K tok) — Go package: activities
- **thread_activities_test.go** (706 lines, ~7.6K tok) — Package activities provides tests for email threading activities.
- **thread_repo.go** (203 lines, ~1.7K tok) — Go package: activities
- **topic_adapter.go** (75 lines, ~566 tok) — Go package: activities
- **triage_activities.go** (760 lines, ~7.9K tok) — Package activities provides activity implementations for the Temporal worker.
- **triage_activities_test.go** (1503 lines, ~15.6K tok) — Package activities provides tests for triage activities.
- **validation.go** (79 lines, ~704 tok) — Package activities provides activity implementations for the Temporal worker.
- **validation_test.go** (130 lines, ~1.1K tok) — Go package: activities

## services/worker/config/ (~2.2K tokens)

- **config.go** (283 lines, ~2.2K tok) — Package config provides configuration for the Temporal worker service.

## services/worker/ (~21.7K tokens)

- **go.mod** (85 lines, ~1.0K tok) — go.mod in services/worker/
- **go.sum** (218 lines, ~5.4K tok) — go.sum in services/worker/
- **main.go** (1269 lines, ~14.1K tok) — Package main provides the entry point for the Penfold Temporal worker.
- **main_test.go** (58 lines, ~601 tok) — Go package: main
- **reconcile.go** (63 lines, ~560 tok) — Go package: main

## services/worker/integration/ (~4.8K tokens)

- **workflow_integration_test.go** (460 lines, ~4.8K tok) — Package integration provides integration tests for Temporal workflows.

## services/worker/workflows/ (~224.0K tokens)

- **ad_sync.go** (356 lines, ~3.3K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **ad_sync_test.go** (282 lines, ~2.5K tok) — Go package: workflows
- **analysis.go** (468 lines, ~4.3K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **assertion_activity_options_test.go** (154 lines, ~2.2K tok) — Go package: workflows
- **auto_drain_test.go** (329 lines, ~3.8K tok) — Go package: workflows
- **batch_pipeline.go** (272 lines, ~2.3K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **batch_pipeline_test.go** (212 lines, ~2.1K tok) — Go package: workflows
- **consolidation.go** (83 lines, ~822 tok) — Go package: workflows
- **content.go** (581 lines, ~6.1K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **content_test.go** (586 lines, ~6.2K tok) — Package workflows provides workflow tests using Temporal's test framework.
- **context_package_test.go** (185 lines, ~1.9K tok) — Go package: workflows
- **contribution_gating_bug_test.go** (157 lines, ~1.8K tok) — Package workflows provides workflow tests.
- **conversation_maintenance.go** (140 lines, ~1.3K tok) — Go package: workflows
- **conversation_maintenance_test.go** (147 lines, ~1.4K tok) — Go package: workflows
- **digest.go** (217 lines, ~2.3K tok) — Go package: workflows
- **digest_rollup.go** (227 lines, ~2.6K tok) — Go package: workflows
- **digest_rollup_test.go** (216 lines, ~2.2K tok) — Go package: workflows
- **email.go** (277 lines, ~3.1K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **email_test.go** (610 lines, ~6.4K tok) — Package workflows provides workflow tests using Temporal's test framework.
- **gmail_sync.go** (471 lines, ~4.4K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **heartbeat.go** (145 lines, ~1.4K tok) — Go package: workflows
- **journal_digest.go** (156 lines, ~1.6K tok) — Go package: workflows
- **journal_rollup.go** (197 lines, ~2.3K tok) — Go package: workflows
- **journal_rollup_test.go** (293 lines, ~3.2K tok) — Go package: workflows
- **langfuse_helpers_test.go** (59 lines, ~415 tok) — Go package: workflows
- **langfuse_metadata_pipeline_test.go** (280 lines, ~3.9K tok) — Package workflows provides acceptance tests for pf-37ebe8:
- **langfuse_types.go** (86 lines, ~1.2K tok) — Package workflows provides Langfuse activity input/output types used by the pipeline workflow.
- **ner_activity_options_test.go** (238 lines, ~3.3K tok) — Go package: workflows
- **outlook_sync.go** (386 lines, ~3.6K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **pipeline.go** (3747 lines, ~41.9K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **pipeline_contract_test.go** (1322 lines, ~14.2K tok) — Go package: workflows
- **pipeline_contribution_gating_test.go** (414 lines, ~5.1K tok) — Go package: workflows
- **pipeline_definitions_test.go** (373 lines, ~4.3K tok) — Go package: workflows
- **pipeline_fallback_removal_test.go** (278 lines, ~3.6K tok) — Go package: workflows
- **pipeline_overrides_test.go** (338 lines, ~4.0K tok) — Go package: workflows
- **pipeline_status_bug_test.go** (100 lines, ~1.1K tok) — Go package: workflows
- **pipeline_test.go** (2440 lines, ~28.3K tok) — Go package: workflows
- **pipeline_trace_name_test.go** (26 lines, ~166 tok) — Go package: workflows
- **register.go** (173 lines, ~1.7K tok) — Package workflows provides workflow registration for the Temporal worker.
- **register_test.go** (185 lines, ~1.6K tok) — Package workflows provides tests for workflow registration.
- **relationship.go** (498 lines, ~5.0K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **reprocess_threading_test.go** (427 lines, ~5.2K tok) — Go package: workflows
- **review.go** (466 lines, ~4.6K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **review_test.go** (709 lines, ~7.2K tok) — Package workflows provides workflow tests using Temporal's test framework.
- **teams_sync.go** (441 lines, ~4.1K tok) — Package workflows provides workflow definitions for the Temporal worker.
- **temporal_errors.go** (41 lines, ~332 tok) — Package workflows provides Temporal error classification utilities.
- **temporal_errors_test.go** (101 lines, ~1.0K tok) — Package workflows provides tests for Temporal error classification.
- **transcript_sync.go** (377 lines, ~3.6K tok) — Go package: workflows
- **triage_activity_options_test.go** (140 lines, ~1.9K tok) — Go package: workflows
- **triage_metadata_persistence_test.go** (93 lines, ~1.1K tok) — Package workflows provides workflow tests.
- **weekly_digest.go** (221 lines, ~2.4K tok) — Go package: workflows

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

- **ARCHIVE.md** (389 lines, ~4.2K tok) — AI Coordination Framework Implementation Archive
- **LESSONS_LEARNED.md** (343 lines, ~3.5K tok) — AI Coordination Framework - Lessons Learned
- **implementation.md** (317 lines, ~3.2K tok) — AI Coordination Framework - Implementation Checklist
- **spec.md** (248 lines, ~2.5K tok) — AI Coordination Framework - Specification 003

## specs/003-ai-coordination/checklists/ (~293 tokens)

- **requirements.md** (34 lines, ~293 tok) — Specification Quality Checklist: Multi-Model AI Coordination

## specs/005-meeting-pipeline/ (~17.7K tokens)

- **ARCHIVE.md** (302 lines, ~3.1K tok) — Meeting Pipeline Implementation Archive
- **data-model.md** (344 lines, ~2.9K tok) — Meeting Pipeline Data Model
- **implementation-plan.md** (135 lines, ~2.0K tok) — Implementation Plan: Meeting Upload and Processing Pipeline
- **plan.md** (135 lines, ~2.0K tok) — Implementation Plan: Meeting Upload and Processing Pipeline
- **quickstart.md** (412 lines, ~2.5K tok) — Meeting Pipeline Quickstart Guide
- **research.md** (157 lines, ~1.9K tok) — Meeting Pipeline Research & Technical Decisions
- **spec.md** (177 lines, ~3.3K tok) — Feature Specification: Meeting Upload and Processing Pipeline

## specs/005-meeting-pipeline/checklists/ (~552 tokens)

- **requirements.md** (48 lines, ~552 tok) — Specification Quality Checklist: Meeting Upload and Processing Pipeline

## specs/005-meeting-pipeline/contracts/ (~7.9K tokens)

- **api-spec.yaml** (797 lines, ~7.9K tok) — api-spec.yaml in specs/005-meeting-pipeline/contracts/

## specs/006-daily-review/checklists/ (~294 tokens)

- **requirements.md** (34 lines, ~294 tok) — Specification Quality Checklist: Daily Review Workflow Interface

## specs/006-daily-review/contracts/ (~3.2K tokens)

- **cli-api.md** (579 lines, ~3.2K tok) — CLI API Contracts: Daily Review Workflow

## specs/006-daily-review/ (~11.4K tokens)

- **data-model.md** (305 lines, ~3.5K tok) — Data Model: Daily Review Workflow Interface
- **plan.md** (117 lines, ~1.7K tok) — Implementation Plan: Daily Review Workflow Interface
- **quickstart.md** (276 lines, ~1.3K tok) — Quickstart: Daily Review Workflow
- **research.md** (214 lines, ~2.1K tok) — Research: Daily Review Workflow Interface
- **spec.md** (166 lines, ~2.8K tok) — Feature Specification: Daily Review Workflow Interface

## specs/007-search-interface/ (~17.6K tokens)

- **ARCHIVE.md** (134 lines, ~1.4K tok) — Search Interface Specification - ARCHIVED
- **data-model.md** (567 lines, ~5.4K tok) — Data Model: Search and Query Interface
- **plan.md** (105 lines, ~1.7K tok) — Implementation Plan: Search and Query Interface
- **quickstart.md** (367 lines, ~2.1K tok) — Quickstart: Search and Query Interface
- **research.md** (352 lines, ~3.5K tok) — Research: Search and Query Interface
- **spec.md** (190 lines, ~3.4K tok) — Feature Specification: Search and Query Interface

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

- **data-model.md** (352 lines, ~3.7K tok) — Data Model: Automation Rules Engine
- **plan.md** (178 lines, ~2.0K tok) — Implementation Plan: Automation Rules Engine
- **quickstart.md** (274 lines, ~1.9K tok) — Quickstart: Automation Rules Engine
- **research.md** (227 lines, ~1.9K tok) — Research: Automation Rules Engine
- **spec.md** (269 lines, ~4.4K tok) — Feature Specification: Automation Rules Engine

## specs/009-relationship-discovery-and-management/archive/ (~1.2K tokens)

- **lessons-learned.md** (112 lines, ~1.2K tok) — Lessons Learned: 009-Relationship-Discovery-and-Management

## specs/009-relationship-discovery-and-management/checklists/ (~295 tokens)

- **requirements.md** (34 lines, ~295 tok) — Specification Quality Checklist: Relationship Discovery and Management

## specs/009-relationship-discovery-and-management/contracts/ (~6.7K tokens)

- **relationship-api.yaml** (482 lines, ~4.0K tok) — relationship-api.yaml in specs/009-relationship-discovery-and-management/contracts/
- **relationship-events.yaml** (301 lines, ~2.7K tok) — Event Contracts for Relationship Discovery and Management

## specs/009-relationship-discovery-and-management/ (~13.3K tokens)

- **data-model.md** (350 lines, ~4.2K tok) — Data Model: Relationship Discovery and Management
- **plan.md** (101 lines, ~1.3K tok) — Implementation Plan: Relationship Discovery and Management
- **quickstart.md** (265 lines, ~1.6K tok) — Quickstart: Relationship Discovery and Management
- **research.md** (240 lines, ~2.2K tok) — Research: Relationship Discovery and Management
- **spec.md** (195 lines, ~4.1K tok) — Feature Specification: Relationship Discovery and Management

## specs/010-testing-framework/ (~17.1K tokens)

- **ARCHIVE.md** (115 lines, ~1.3K tok) — Testing Framework Specification - ARCHIVED
- **ai-mocking-framework.md** (433 lines, ~4.0K tok) — AI Model Mocking Framework
- **environment-isolation.md** (700 lines, ~4.3K tok) — Environment Isolation Strategy
- **spec.md** (335 lines, ~3.8K tok) — Feature Specification: AI-First Testing Framework
- **test-data-strategy.md** (355 lines, ~3.6K tok) — Test Data Strategy: Realistic Business Corpus

## specs/011-observability-framework/ (~19.8K tokens)

- **ARCHIVE.md** (99 lines, ~1.0K tok) — Observability Framework Specification - ARCHIVED
- **coordination.md** (142 lines, ~1.5K tok) — Penfold Production Agent Observability Coordination
- **data-model.md** (444 lines, ~3.8K tok) — Observability Framework Data Model
- **plan.md** (178 lines, ~2.5K tok) — Implementation Plan: Penfold Production Agent Observability
- **quickstart.md** (601 lines, ~5.2K tok) — Observability Framework Quickstart Guide
- **research.md** (204 lines, ~2.4K tok) — Observability Framework Research & Technical Decisions
- **spec.md** (216 lines, ~3.3K tok) — Feature Specification: Penfold Production Agent Observability

## specs/011-observability-framework/checklists/ (~458 tokens)

- **requirements.md** (45 lines, ~458 tok) — Specification Quality Checklist: Penfold Production Agent Observability

## specs/011-observability-framework/contracts/ (~13.8K tokens)

- **instrumentation_interface.py** (545 lines, ~5.6K tok) — instrumentation_interface.py in specs/011-observability-framework/contracts/
- **monitoring_api.yaml** (896 lines, ~8.2K tok) — monitoring_api.yaml in specs/011-observability-framework/contracts/

## specs/020-slm-llm-architecture/ (~76.0K tokens)

- **00-overview.md** (235 lines, ~4.0K tok) — Penfold: Project Overview
- **01-architecture.md** (158 lines, ~2.7K tok) — Penfold: System Architecture
- **02-data-model.md** (268 lines, ~3.7K tok) — Penfold: Data Model
- **03-entities.md** (244 lines, ~2.7K tok) — Penfold: Entity Model
- **04-ai-services.md** (243 lines, ~3.1K tok) — Penfold: AI Services
- **05-content-pipeline.md** (256 lines, ~3.1K tok) — Penfold: Content Processing Pipeline
- **06-constraints.md** (145 lines, ~1.6K tok) — Penfold: Constraints and Current Limitations
- **07-session-bootstrap.md** (312 lines, ~3.3K tok) — Penfold: Session Bootstrap
- **README.md** (81 lines, ~1.8K tok) — SLM/LLM Architecture — Context for AI Advisors
- **cost-model.md** (55 lines, ~634 tok) — Cost Model and Performance Expectations
- **design.md** (2319 lines, ~31.3K tok) — Penfold: SLM/LLM Architecture Design
- **implementation.md** (111 lines, ~2.1K tok) — Implementation Mapping
- **model-selection.md** (136 lines, ~2.1K tok) — Model Selection: 7B vs 14B vs 32B on Apple Silicon
- **prompt-engineering.md** (97 lines, ~1.7K tok) — Prompt Engineering and Output Validation
- **test-data-validation.md** (151 lines, ~2.3K tok) — Validation Against Real Test Data
- **work-packages.md** (571 lines, ~9.9K tok) — SLM/LLM Pipeline — Work Packages

## specs/020-slm-llm-architecture/feedback/ (~15.2K tokens)

- **architecture-review.md** (307 lines, ~5.9K tok) — Architecture Review: SLM/LLM Architecture Design
- **consolidated-feedback-2026-02-04.md** (253 lines, ~5.0K tok) — Consolidated Architecture & Documentation Review
- **docs-review-agent-mycroft-2026-02-04.md** (89 lines, ~1.4K tok) — Architecture Review: SLM/LLM Split & Human-AI Collaboration
- **docs-review-mycroft-2026-02-04.md** (194 lines, ~2.9K tok) — Doc Review (Mycroft) — SLM/LLM Architecture Spec

## specs/020-slm-llm-architecture/feedback2/ (~3.1K tokens)

- **architecture-review-2026-02-04.md** (152 lines, ~3.1K tok) — SLM/LLM Architecture Review

## tests/benchmark/ (~4.5K tokens)

- **helpers.go** (254 lines, ~2.0K tok) — Package benchmark provides benchmarking tests for Penfold components.
- **llm_test.go** (326 lines, ~2.5K tok) — Go package: benchmark

## tests/e2e/ (~205.2K tokens)

- **account_pattern_externalize_test.go** (399 lines, ~4.2K tok) — account_pattern_externalize_test.go in tests/e2e/
- **alert_model_test.go** (206 lines, ~2.2K tok) — Go package: e2e
- **assertions.go** (188 lines, ~1.5K tok) — Go package: e2e
- **assertions_list_test.go** (126 lines, ~1.4K tok) — Go package: e2e
- **attribution_pipeline_test.go** (374 lines, ~4.1K tok) — Go package: e2e
- **bridge_test.go** (162 lines, ~1.8K tok) — bridge_test.go in tests/e2e/
- **briefing_test.go** (190 lines, ~2.1K tok) — Go package: e2e
- **classification_routing_epic_test.go** (358 lines, ~4.0K tok) — Go package: e2e
- **classify_llm_fallback_test.go** (113 lines, ~1.1K tok) — Go package: e2e
- **classify_reprocess_cli_test.go** (179 lines, ~2.0K tok) — Go package: e2e
- **classify_stats_reprocess_threading_test.go** (332 lines, ~3.6K tok) — Go package: e2e
- **cleanup_test.go** (423 lines, ~4.3K tok) — Go package: e2e
- **cli_runner.go** (244 lines, ~2.0K tok) — Go package: e2e
- **content_enrichment_test.go** (175 lines, ~2.0K tok) — Go package: e2e
- **conversation_audit_test.go** (144 lines, ~1.5K tok) — Go package: e2e
- **conversation_cli_test.go** (383 lines, ~3.8K tok) — Go package: e2e
- **conversation_summary_state_test.go** (231 lines, ~2.4K tok) — Go package: e2e
- **conversation_test.go** (240 lines, ~2.4K tok) — Go package: e2e
- **digest_search_test.go** (148 lines, ~1.5K tok) — Go package: e2e
- **digest_test.go** (414 lines, ~4.3K tok) — Go package: e2e
- **domain_company_externalize_test.go** (397 lines, ~4.3K tok) — Go package: e2e
- **email_threading_classification_test.go** (592 lines, ~6.2K tok) — Go package: e2e
- **embedding_chunk_config_test.go** (218 lines, ~2.8K tok) — Go package: e2e
- **entity_enrichment_pipeline_test.go** (327 lines, ~3.9K tok) — Go package: e2e
- **entity_model_extensions_test.go** (353 lines, ~4.2K tok) — Go package: e2e
- **entity_role_association_test.go** (365 lines, ~4.0K tok) — Go package: e2e
- **entity_role_extraction_test.go** (201 lines, ~2.0K tok) — Go package: e2e
- **entity_show_test.go** (142 lines, ~1.4K tok) — Go package: e2e
- **environment_test.go** (88 lines, ~637 tok) — Go package: e2e
- **glossary_bugs_test.go** (220 lines, ~2.2K tok) — Go package: e2e
- **glossary_test.go** (245 lines, ~2.4K tok) — Go package: e2e
- **graph_auth_test.go** (233 lines, ~2.7K tok) — Go package: e2e
- **heartbeat_test.go** (237 lines, ~2.7K tok) — heartbeat_test.go in tests/e2e/
- **helpers.go** (656 lines, ~6.4K tok) — Package e2e provides helpers for end-to-end tests.
- **ingest_test.go** (331 lines, ~2.8K tok) — Go package: e2e
- **instructions_test.go** (662 lines, ~7.1K tok) — Go package: e2e
- **journal_digest_test.go** (323 lines, ~3.3K tok) — Go package: e2e
- **langfuse_client_test.go** (255 lines, ~2.6K tok) — Go package: e2e
- **langfuse_trace_test.go** (539 lines, ~5.8K tok) — Go package: e2e
- **llm_config_externalize_test.go** (871 lines, ~9.0K tok) — llm_config_externalize_test.go in tests/e2e/
- **mention_prompt_externalize_test.go** (446 lines, ~4.5K tok) — mention_prompt_externalize_test.go in tests/e2e/
- **mention_resolution_test.go** (359 lines, ~2.7K tok) — Go package: e2e
- **model_management_test.go** (267 lines, ~2.6K tok) — Go package: e2e
- **model_registry_externalize_test.go** (564 lines, ~6.0K tok) — model_registry_externalize_test.go in tests/e2e/
- **per_stage_model_test.go** (81 lines, ~829 tok) — Go package: e2e
- **pipeline_concurrency_test.go** (502 lines, ~5.1K tok) — Go package: e2e
- **pipeline_test.go** (364 lines, ~2.9K tok) — Go package: e2e
- **pipeline_test.sh** (300 lines, ~2.2K tok) — pipeline_test.sh in tests/e2e/
- **project_observability_test.go** (275 lines, ~2.9K tok) — Go package: e2e
- **project_tagging_test.go** (367 lines, ~4.0K tok) — Go package: e2e
- **prompt_override_test.go** (166 lines, ~2.0K tok) — Go package: e2e
- **safe_cli_runner.go** (227 lines, ~2.2K tok) — Go package: e2e
- **safe_cli_runner_test.go** (195 lines, ~2.2K tok) — Go package: e2e
- **scheduled_digest_test.go** (224 lines, ~2.2K tok) — Go package: e2e
- **scheduling_test.go** (451 lines, ~5.1K tok) — scheduling_test.go in tests/e2e/
- **search_role_filter_test.go** (227 lines, ~2.4K tok) — Go package: e2e
- **search_test.go** (365 lines, ~2.9K tok) — Go package: e2e
- **slm_pipeline_helpers_test.go** (500 lines, ~3.9K tok) — Go package: e2e
- **slm_pipeline_outcome_test.go** (144 lines, ~1.6K tok) — Go package: e2e
- **slm_pipeline_test.go** (845 lines, ~7.6K tok) — Go package: e2e
- **source_mapping_test.go** (202 lines, ~2.3K tok) — Go package: e2e
- **tenant_id_externalize_test.go** (389 lines, ~3.5K tok) — tenant_id_externalize_test.go in tests/e2e/
- **tenant_isolation_test.go** (110 lines, ~1.1K tok) — Go package: e2e_test
- **weekly_digest_test.go** (363 lines, ~3.9K tok) — Go package: e2e

## tests/fixtures/acme-corp/emails/ (~3.2K tokens)

- **001-project-update.eml** (18 lines, ~112 tok) — 001-project-update.eml in tests/fixtures/acme-corp/emails/
- **002-incident-response.eml** (24 lines, ~187 tok) — 002-incident-response.eml in tests/fixtures/acme-corp/emails/
- **003-meeting-invite.eml** (21 lines, ~152 tok) — 003-meeting-invite.eml in tests/fixtures/acme-corp/emails/
- **004-code-review.eml** (20 lines, ~177 tok) — 004-code-review.eml in tests/fixtures/acme-corp/emails/
- **005-project-kickoff.eml** (23 lines, ~214 tok) — 005-project-kickoff.eml in tests/fixtures/acme-corp/emails/
- **006-sales-update.eml** (21 lines, ~205 tok) — 006-sales-update.eml in tests/fixtures/acme-corp/emails/
- **007-documentation.eml** (24 lines, ~193 tok) — 007-documentation.eml in tests/fixtures/acme-corp/emails/
- **008-security-review.eml** (28 lines, ~216 tok) — 008-security-review.eml in tests/fixtures/acme-corp/emails/
- **009-mobile-update.eml** (27 lines, ~241 tok) — 009-mobile-update.eml in tests/fixtures/acme-corp/emails/
- **010-postmortem.eml** (30 lines, ~260 tok) — 010-postmortem.eml in tests/fixtures/acme-corp/emails/
- **010-role-test.eml** (18 lines, ~120 tok) — 010-role-test.eml in tests/fixtures/acme-corp/emails/
- **011-risk-escalation.eml** (31 lines, ~454 tok) — 011-risk-escalation.eml in tests/fixtures/acme-corp/emails/
- **011-role-test-reply.eml** (19 lines, ~132 tok) — 011-role-test-reply.eml in tests/fixtures/acme-corp/emails/
- **012-low-priority-fyi.eml** (15 lines, ~90 tok) — 012-low-priority-fyi.eml in tests/fixtures/acme-corp/emails/
- **013-thread-with-decisions.eml** (38 lines, ~463 tok) — 013-thread-with-decisions.eml in tests/fixtures/acme-corp/emails/

## tests/fixtures/acme-corp/emails/newsletter/ (~236.1K tokens)

- **001-ctg-post-its.eml** (3870 lines, ~74.5K tok) — 001-ctg-post-its.eml in tests/fixtures/acme-corp/emails/newsletter/
- **004-dynamic-signal.eml** (1049 lines, ~17.3K tok) — 004-dynamic-signal.eml in tests/fixtures/acme-corp/emails/newsletter/
- **005-eng-learning.eml** (7392 lines, ~144.3K tok) — 005-eng-learning.eml in tests/fixtures/acme-corp/emails/newsletter/

## tests/fixtures/acme-corp/emails/notification/ (~4.6K tokens)

- **001-aha-daily-todos.eml** (30 lines, ~312 tok) — 001-aha-daily-todos.eml in tests/fixtures/acme-corp/emails/notification/
- **002-aha-digest-compute.eml** (37 lines, ~365 tok) — 002-aha-digest-compute.eml in tests/fixtures/acme-corp/emails/notification/
- **003-aha-digest-compute-2.eml** (28 lines, ~282 tok) — 003-aha-digest-compute-2.eml in tests/fixtures/acme-corp/emails/notification/
- **004-jira-track-updates.eml** (29 lines, ~201 tok) — 004-jira-track-updates.eml in tests/fixtures/acme-corp/emails/notification/
- **005-oracle-antibribery.eml** (21 lines, ~212 tok) — 005-oracle-antibribery.eml in tests/fixtures/acme-corp/emails/notification/
- **006-google-signin-alert.eml** (25 lines, ~356 tok) — 006-google-signin-alert.eml in tests/fixtures/acme-corp/emails/notification/
- **007-globalsecops-malicious-dns.eml** (141 lines, ~1.0K tok) — 007-globalsecops-malicious-dns.eml in tests/fixtures/acme-corp/emails/notification/
- **008-bitmovin-action-required.eml** (51 lines, ~1.2K tok) — 008-bitmovin-action-required.eml in tests/fixtures/acme-corp/emails/notification/
- **009-internal-a360-cleanup.eml** (66 lines, ~699 tok) — 009-internal-a360-cleanup.eml in tests/fixtures/acme-corp/emails/notification/

## tests/fixtures/acme-corp/ (~6.4K tokens)

- **glossary.yaml** (422 lines, ~3.2K tok) — Acme Corp Glossary Fixtures
- **people.yaml** (223 lines, ~1.4K tok) — Acme Corp People Fixtures
- **products.yaml** (34 lines, ~298 tok) — Acme Corp Products Fixtures
- **projects.yaml** (104 lines, ~935 tok) — Acme Corp Projects Fixtures
- **teams.yaml** (60 lines, ~485 tok) — Acme Corp Teams Fixtures

## tests/fixtures/acme-corp/meetings/ (~2.9K tokens)

- **001-weekly-standup.txt** (41 lines, ~578 tok) — 001-weekly-standup.txt in tests/fixtures/acme-corp/meetings/
- **002-project-review.vtt** (83 lines, ~1.2K tok) — 002-project-review.vtt in tests/fixtures/acme-corp/meetings/
- **003-incident-retro.srt** (92 lines, ~1.1K tok) — 003-incident-retro.srt in tests/fixtures/acme-corp/meetings/

## tests/ (~6.0K tokens)

- **go.mod** (75 lines, ~893 tok) — go.mod in tests/
- **go.sum** (203 lines, ~5.1K tok) — go.sum in tests/

## tests/integration/ (~123.0K tokens)

- **ai_service_test.go** (508 lines, ~4.6K tok) — Go package: integration
- **cli_ai_test.go** (104 lines, ~1.1K tok) — Package integration contains integration tests that require real services.
- **cli_content_test.go** (262 lines, ~2.5K tok) — Go package: integration
- **cli_glossary_test.go** (383 lines, ~4.0K tok) — Go package: integration
- **cli_ingest_test.go** (120 lines, ~1.3K tok) — Package integration contains integration tests that require real services.
- **cli_logs_test.go** (92 lines, ~838 tok) — Package integration contains integration tests that require real services.
- **cli_meeting_test.go** (368 lines, ~3.5K tok) — Go package: integration
- **cli_mentions_test.go** (167 lines, ~1.8K tok) — Go package: integration
- **cli_relationship_test.go** (73 lines, ~754 tok) — Package integration contains integration tests that require real services.
- **db_test.go** (122 lines, ~917 tok) — Go package: integration
- **email_threading_test.go** (537 lines, ~5.6K tok) — Go package: integration
- **empty_content_bug_test.go** (175 lines, ~2.0K tok) — Go package: integration
- **entity_group_test.go** (148 lines, ~1.3K tok) — Go package: integration
- **entity_test.go** (1272 lines, ~9.9K tok) — Go package: integration
- **error_classification_test.go** (173 lines, ~1.5K tok) — Go package: integration
- **fixtures_test.go** (193 lines, ~1.6K tok) — Go package: integration
- **glossary_scan_bug_test.go** (295 lines, ~3.5K tok) — Go package: integration
- **glossary_tenant_test.go** (347 lines, ~3.8K tok) — Go package: integration
- **glossary_test.go** (556 lines, ~4.1K tok) — Go package: integration
- **header_preservation_test.go** (568 lines, ~6.3K tok) — Go package: integration
- **helpers.go** (462 lines, ~4.1K tok) — Package integration provides test helpers for integration tests.
- **ingest_test.go** (1035 lines, ~8.3K tok) — Go package: integration
- **ledger_test.go** (332 lines, ~2.9K tok) — Go package: integration
- **meeting_series_test.go** (383 lines, ~3.0K tok) — Go package: integration
- **mentions_test.go** (649 lines, ~5.7K tok) — Go package: integration
- **migrations_test.go** (360 lines, ~2.6K tok) — Go package: integration
- **newsletter_variant_digest_test.go** (168 lines, ~1.9K tok) — Go package: integration
- **newsletter_variant_pattern_test.go** (236 lines, ~2.4K tok) — Go package: integration
- **pipeline_config_test.go** (217 lines, ~2.0K tok) — Go package: integration
- **pre_triage_classification_test.go** (495 lines, ~5.2K tok) — Go package: integration
- **project_test.go** (562 lines, ~4.1K tok) — Go package: integration
- **reviewqueue_tenant_test.go** (223 lines, ~2.4K tok) — Go package: integration
- **search_role_filter_test.go** (421 lines, ~4.0K tok) — Go package: integration
- **search_test.go** (326 lines, ~2.2K tok) — Go package: integration
- **tenant_test.go** (433 lines, ~3.0K tok) — Go package: integration
- **timeout_config_test.go** (229 lines, ~2.0K tok) — Go package: integration
- **tls_test.go** (555 lines, ~5.3K tok) — Package integration provides integration tests for the Penfold API Gateway.
- **topic_keyword_test.go** (107 lines, ~1.1K tok) — Go package: integration

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

## tests/quality/ (~29.5K tokens)

- **README.md** (133 lines, ~1.4K tok) — Quality Tests — Pipeline Extraction Accuracy
- **helpers.go** (803 lines, ~7.5K tok) — Go package: quality
- **langfuse_eval.go** (108 lines, ~872 tok) — Go package: quality
- **matchers.go** (308 lines, ~2.5K tok) — Go package: quality
- **newsletter_eval_test.go** (116 lines, ~1.1K tok) — Go package: quality
- **newsletter_matchers.go** (241 lines, ~2.2K tok) — Go package: quality
- **newsletter_matchers_test.go** (94 lines, ~709 tok) — Go package: quality
- **notification_eval_test.go** (116 lines, ~1.1K tok) — Go package: quality
- **notification_matchers.go** (246 lines, ~2.5K tok) — Go package: quality
- **notification_matchers_test.go** (123 lines, ~1.1K tok) — Go package: quality
- **quality_test.go** (131 lines, ~1.2K tok) — Go package: quality
- **routing_matchers.go** (212 lines, ~1.9K tok) — Go package: quality
- **routing_matchers_test.go** (45 lines, ~400 tok) — Go package: quality
- **standard_eval_test.go** (133 lines, ~1.2K tok) — Go package: quality
- **types.go** (364 lines, ~3.8K tok) — Package quality provides extraction quality tests for the Penfold pipeline.

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

- **001-aha-daily-todos.yaml** (27 lines, ~233 tok) — 001-aha-daily-todos.yaml in tests/quality/golden/notification/
- **002-aha-digest-compute.yaml** (27 lines, ~226 tok) — 002-aha-digest-compute.yaml in tests/quality/golden/notification/
- **003-aha-digest-compute-2.yaml** (27 lines, ~217 tok) — 003-aha-digest-compute-2.yaml in tests/quality/golden/notification/
- **004-jira-track-updates.yaml** (27 lines, ~230 tok) — 004-jira-track-updates.yaml in tests/quality/golden/notification/
- **005-oracle-antibribery.yaml** (26 lines, ~249 tok) — 005-oracle-antibribery.yaml in tests/quality/golden/notification/
- **006-google-signin-alert.yaml** (27 lines, ~248 tok) — 006-google-signin-alert.yaml in tests/quality/golden/notification/
- **007-globalsecops-malicious-dns.yaml** (26 lines, ~222 tok) — 007-globalsecops-malicious-dns.yaml in tests/quality/golden/notification/
- **008-bitmovin-action-required.yaml** (27 lines, ~240 tok) — 008-bitmovin-action-required.yaml in tests/quality/golden/notification/
- **009-internal-a360-cleanup.yaml** (27 lines, ~230 tok) — 009-internal-a360-cleanup.yaml in tests/quality/golden/notification/

## tests/quality/golden/standard/ (~2.2K tokens)

- **001-project-update.yaml** (30 lines, ~218 tok) — 001-project-update.yaml in tests/quality/golden/standard/
- **002-incident-response.yaml** (36 lines, ~274 tok) — 002-incident-response.yaml in tests/quality/golden/standard/
- **005-project-kickoff.yaml** (33 lines, ~259 tok) — 005-project-kickoff.yaml in tests/quality/golden/standard/
- **008-security-review.yaml** (31 lines, ~232 tok) — 008-security-review.yaml in tests/quality/golden/standard/
- **010-postmortem.yaml** (34 lines, ~259 tok) — 010-postmortem.yaml in tests/quality/golden/standard/
- **011-risk-escalation.yaml** (42 lines, ~343 tok) — 011-risk-escalation.yaml in tests/quality/golden/standard/
- **012-low-priority-fyi.yaml** (30 lines, ~237 tok) — 012-low-priority-fyi.yaml in tests/quality/golden/standard/
- **013-thread-with-decisions.yaml** (42 lines, ~353 tok) — 013-thread-with-decisions.yaml in tests/quality/golden/standard/

---

1706 files, ~6240.5K tokens total
