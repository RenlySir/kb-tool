# kb-tool

`kb-tool` is a Go tool for building a small TiDB-backed knowledge base from local files, directories, GitHub repositories, and GitLab repositories. It collects content, generates deterministic rule-based tags, stores documents and tag mappings in TiDB, provides CLI search, and includes a built-in Web UI for batch import and browsing.

The current implementation is the first usable core. It is intentionally lightweight: TiDB is the storage backend, Git is used for repository ingestion, and the tagger is rule based so the tool works offline. The architecture leaves room for later Unstructured parsing, AI metadata extraction, embeddings, MinIO, Dify, Airweave, and MCP integrations.

## What It Does Today

- Ingest a single text file, a directory tree, a GitHub repository URL, or a GitLab repository URL.
- Shallow-clone Git repositories with `git clone --depth 1`.
- Skip binary files and noisy folders such as `.git`, `node_modules`, `vendor`, `dist`, `build`, and `target`.
- Infer a simple language value from file extension.
- Generate stable tags from language, path, title, and content keywords.
- Create the TiDB schema automatically.
- Upsert documents by `(content_hash, path)` to avoid duplicates on repeated ingestion.
- Search document title, path, and content from the CLI.
- Start a Web UI and REST API with `kb-tool server`.
- Batch import sources, browse documents, add manual tags, and preview image assets from the browser.

## Roadmap

Planned extensions are documented in [docs/operation-manual.md](docs/operation-manual.md):

- Unstructured-based parsing for PDF, Word, PPT, Markdown, TXT, images, and video-derived text.
- LangExtract-style structured metadata extraction for authors, dates, entities, and relations.
- Manual tags, rule tags, and LLM-generated AI tags with review status, confidence, and evidence.
- Chunking, embedding, and TiDB vector search.
- MinIO for raw object storage.
- Dify Knowledge Pipeline and Airweave adapters.
- MCP server for AI agents.

## Requirements

- Go 1.24 or newer.
- Git CLI for GitHub/GitLab repository ingestion.
- TiDB reachable through the MySQL protocol.
- Docker and Docker Compose, if you want to build the image or run the included local stack.

## Quick Start

Start a local TiDB:

```bash
docker compose up -d tidb
```

Build the CLI:

```bash
go build ./cmd/kb-tool
```

Create the schema:

```bash
./kb-tool migrate
```

Ingest a local directory:

```bash
./kb-tool ingest ./docs
```

Ingest a GitHub repository:

```bash
./kb-tool ingest https://github.com/RenlySir/kb-tool.git
```

Search:

```bash
./kb-tool search tidb
```

Start the Web UI:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080).

## Docker Quick Start

Build the application image:

```bash
docker build -t kb-tool:local .
```

Run the full local stack with TiDB and the Web UI:

```bash
docker compose build kb-tool
docker compose up -d tidb
docker compose run --rm kb-tool migrate
docker compose up -d kb-tool
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). Docker Compose binds the service on `0.0.0.0:8080`, so it sets `KB_TOOL_API_TOKEN` to `dev-token` by default. The Web UI prompts for the token on the first API request. Override it for real use:

```bash
KB_TOOL_API_TOKEN=<strong-token> docker compose up -d kb-tool
```

Run one-off commands inside the image:

```bash
docker compose run --rm kb-tool ingest /workspace/docs
docker compose run --rm kb-tool search tidb
```

The Compose service mounts the repository read-only at `/workspace`, so paths inside the container should use `/workspace/...`. The runtime image includes Git, so GitHub and GitLab repository ingestion works from the container when network access and credentials are available.

## Configuration

The CLI reads TiDB settings from environment variables and command flags. Flags override environment variables.

| Setting | Environment Variable | Flag | Default |
| --- | --- | --- | --- |
| TiDB host | `TIDB_HOST` | `-tidb-host` | `127.0.0.1` |
| TiDB port | `TIDB_PORT` | `-tidb-port` | `4000` |
| TiDB user | `TIDB_USER` | `-tidb-user` | `root` |
| TiDB password | `TIDB_PASSWORD` | `-tidb-password` | empty |
| TiDB database | `TIDB_DATABASE` | `-tidb-database` | `kb` |
| Max file size | `KB_TOOL_MAX_FILE_BYTES` | `-max-file-bytes` | `512MiB` |
| Max extracted text per document | `KB_TOOL_MAX_TEXT_BYTES` | `-max-text-bytes` | `2MiB` |
| Search limit | none | `-limit` | `20` |

Example:

```bash
TIDB_HOST=127.0.0.1 \
TIDB_PORT=4000 \
TIDB_USER=root \
TIDB_DATABASE=kb \
./kb-tool migrate
```

The same configuration with flags:

```bash
./kb-tool migrate \
  -tidb-host 127.0.0.1 \
  -tidb-port 4000 \
  -tidb-user root \
  -tidb-database kb
```

File-size values accept raw bytes or suffixes such as `KB`, `KiB`, `MB`, `MiB`, `GB`, and `GiB`.

## Commands

### `migrate`

Creates the TiDB database if it does not exist, then creates the knowledge-base tables.

```bash
./kb-tool migrate
```

### `ingest`

Collects documents from a file, directory, GitHub URL, or GitLab URL, generates tags, and writes everything to TiDB.

```bash
./kb-tool ingest <file|directory|github-url|gitlab-url>
```

Examples:

```bash
./kb-tool ingest ./README.md
./kb-tool ingest ./docs
./kb-tool ingest https://github.com/RenlySir/kb-tool.git
./kb-tool ingest git@gitlab.com:group/project.git
```

Large file behavior:

- The default single-file collection limit is `512MiB`.
- Text and Markdown files are read with streaming SHA-256 hashing; only the first `KB_TOOL_MAX_TEXT_BYTES` bytes of text are stored in `kb_documents.content`.
- `.docx`, `.xlsx`, and `.pptx` are parsed as Office Open XML ZIP packages and text is extracted from XML entries without storing the original package bytes.
- Legacy `.doc`, `.xls`, and `.ppt` files are recognized as Office binary assets and stored as metadata-only records for now. Full text extraction for these formats should be added through LibreOffice or Unstructured.
- Image assets continue to store bytes for browser preview.

Example for a 1GiB source limit and 4MiB text cap:

```bash
./kb-tool ingest -max-file-bytes 1GiB -max-text-bytes 4MiB ./enterprise-docs
```

### `search`

Searches title, path, and content with a SQL `LIKE` query and prints matching snippets.

```bash
./kb-tool search "knowledge base"
./kb-tool search -limit 5 tidb
```

### `server`

Starts the built-in Web UI and REST API from one process:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

Access surfaces:

- Web UI: batch import local paths and Git repositories, inspect documents, edit tags, filter by tags, and preview images directly in the browser.
- REST API: external systems can ingest sources, list documents, search, read document detail, fetch image assets, and manage tags.

For external binding, configure an API token:

```bash
KB_TOOL_API_TOKEN=<token> ./kb-tool server -addr 0.0.0.0:8080
```

The Web UI prompts for this token when the REST API returns `401 Unauthorized` and stores it in browser `localStorage` under `kbToolApiToken`. REST clients should send `Authorization: Bearer <token>`. Image previews use the same token through the asset endpoint so images render visually in the browser.

Planned MCP and LLM access:

- MCP tools will let AI agents call `search_knowledge`, `get_document`, `list_documents`, `list_tags`, `ingest_source`, `generate_document_tags`, and `add_document_tags`.
- The LLM tagger will connect to OpenAI-compatible APIs, Ollama, or an internal model gateway to generate tags from document text and metadata.

Planned LLM configuration:

```bash
KB_TOOL_LLM_PROVIDER=openai-compatible
KB_TOOL_LLM_BASE_URL=https://api.openai.com/v1
KB_TOOL_LLM_API_KEY=<token>
KB_TOOL_LLM_MODEL=gpt-4.1-mini
```

For local models:

```bash
KB_TOOL_LLM_PROVIDER=ollama
KB_TOOL_LLM_BASE_URL=http://127.0.0.1:11434
KB_TOOL_LLM_MODEL=qwen2.5:7b
```

AI tag output should be stored with source, confidence, and evidence so humans can review or reject model-generated labels.

Image behavior:

- Image files are stored as assets with `image/png`, `image/jpeg`, `image/gif`, or `image/webp` MIME types.
- Images are not forced into text fields.
- Browser preview uses an asset endpoint that returns the correct `Content-Type`, so images render visually instead of appearing as garbled binary text.

Office behavior:

- `.docx`, `.xlsx`, and `.pptx` files are searchable because text is extracted from their XML parts.
- `.doc`, `.xls`, and `.ppt` files are recognized but kept as metadata-only binary asset records until an external parser is integrated.

## Schema

The current version creates three tables:

- `kb_documents`: document metadata, full text, MIME type, binary marker, and optional asset bytes.
- `kb_tags`: normalized tag names.
- `kb_document_tags`: many-to-many document/tag links.

Documents are upserted by `(content_hash, path)`, so repeated ingestion updates existing rows instead of duplicating unchanged files.

## Architecture

```text
Input source
  ├─ local file
  ├─ local directory
  ├─ GitHub repository
  └─ GitLab repository
        ↓
source collector
        ↓
rule-based tagger
        ↓
ingest service
        ↓
TiDB store
        ↓
CLI search
```

Server architecture:

```text
Browser Web UI ─┐
External REST ──┼─ kb-tool server ─ TiDB knowledge store
MCP clients  ───┘
                  ├─ ingest service
                  ├─ document/tag/search API
                  └─ asset endpoint for images and binary files
```

Main packages:

- `cmd/kb-tool`: CLI parsing and command execution.
- `internal/source`: local file collector, Git remote parser, and Git repository collector.
- `internal/tagger`: deterministic rule-based tag generator.
- `internal/ingest`: orchestration of collect, tag, migrate, and save.
- `internal/store`: TiDB schema, upsert, tag linking, and search.
- `internal/web`: Web UI static assets, REST API, batch ingest endpoint, document/tag API, and image asset endpoint.

Planned packages:

- `internal/llm`: provider abstraction for OpenAI-compatible APIs, Ollama, and internal model gateways.
- `internal/aitagger`: prompt construction, JSON tag parsing, confidence/evidence handling, and review-state defaults.

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build ./cmd/kb-tool
```

Format:

```bash
gofmt -w cmd internal
```

## Current Limits

- No PDF, old binary Office (`.doc`, `.xls`, `.ppt`), or video parsing yet.
- No chunking or embedding yet.
- No TiDB vector search yet.
- No MinIO object storage yet.
- No MCP server yet.
- No LLM connection or AI tagger yet.
- No manual tag review workflow yet.
- Git repository ingestion uses a temporary shallow clone and requires local Git credentials for private repositories.

See [docs/operation-manual.md](docs/operation-manual.md) for operating steps, troubleshooting, and the recommended expansion plan.
