# Project structure

```text
rag-template/
├── cmd/
│   ├── eval/              # score retrieval and compare reranking
│   ├── ingest/            # chunk a file into the documents table
│   └── rag/               # answer a question from the stored documents
├── internal/
│   ├── answerability/     # ask the chat model whether the evidence answers the question
│   ├── chunking/          # sections -> chunks
│   ├── citations/         # parse/validate/repair [source - section] citations (RAG-013)
│   ├── config/            # settings + Ollama/Postgres setup
│   ├── contextbudget/     # deterministic byte-estimate context budget (RAG-010)
│   ├── document/          # parse markdown into sections
│   ├── embedding/         # text -> vector (Ollama /api/embed)
│   ├── generation/        # prompt -> answer (Ollama /api/generate)
│   ├── ingestion/         # replace a source's chunks + embeddings atomically
│   ├── loader/            # pluggable format loaders + Registry/Dispatch (RAG-006)
│   ├── observability/     # per-run reporter: human output or JSON slog (RAG-014)
│   ├── ollama/            # shared JSON client for the Ollama server
│   ├── provider/          # embedder/generator provider registry (RAG-004)
│   ├── rag/               # rewrite the question, build the answer prompt
│   ├── reranking/         # lexical + LLM rerankers (with fallback + guard, RAG-012)
│   └── retrieval/         # pgvector search, RRF fusion, section expansion, filters
├── migrations/
│   ├── 001_init.sql                # pgvector extension + documents table + index
│   ├── 002_ingestion_metadata.sql  # provenance columns (hash/model/dim/chunker/ingested_at) (RAG-003)
│   ├── 003_add_language.sql        # nullable per-document language (RAG-009)
│   └── 004_page.sql                # nullable per-chunk page number (RAG-017)
├── evals/
│   └── retrieval.json     # questions + expected source/section
├── scripts/
│   ├── sweep.sh           # compare cmd/eval metrics across settings
│   └── eval-model-sweep.sh # per-model MIN_SIMILARITY sweep on a calibration split
├── docs/
│   ├── README.md          # documentation index and authority model
│   ├── requirements/      # PRD.md (canonical), PRD-Phase-23b.md (proposed)
│   ├── architecture/      # OVERVIEW.md (pipeline, packages, boundaries)
│   ├── reference/         # commands, configuration, capabilities, testing, experiments
│   ├── guides/            # usage, evaluation, git hooks + agent scripts
│   ├── operations/        # operator-run procedures (embedding dimension/models, page provenance, prompt injection)
│   ├── plans/             # PLAN.md and focused, SOP-compatible task plans (RAG-*)
│   ├── history/           # LESSONS.md (retrospective)
│   └── experiments-eval-sweep.md  # recorded embedding-model sweep (script output path)
├── examples/
│   ├── loan-policy.md                  # sample corpus for cmd/ingest
│   ├── large-loan-policy.md            # longer corpus; several chunks per section
│   ├── member-services-guide.md        # distractor corpus for the evaluation
│   ├── commercial-servicing-manual.md  # long sections; exercises CHUNK_SIZE / CHUNK_OVERLAP
│   └── vi/                             # Vietnamese corpora for the multilingual evaluation
├── .agents/
│   └── scripts/                        # agent scripts; project-agnostic
│       ├── audit-agent.sh              # over-engineering audit, writes AUDIT.md
│       ├── integration-test-agent.sh   # run the integration tests
│       └── review-agent.sh             # code review agent
├── .githooks/
│   ├── pre-commit         # tidy / fmt / jq / vet / staticcheck / test / build
│   └── pre-push           # integration tests
├── .env.example
├── .gitignore
├── docker-compose.yml
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

The commands only wire these packages together. See
[`../README.md`](../README.md) for the documentation index.
