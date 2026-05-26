# kb-tool Operation Manual

This manual explains how to install, configure, run, verify, and troubleshoot `kb-tool`.

## 1. System Overview

`kb-tool` ingests content into a TiDB-backed knowledge base. The current release supports local text files, local image assets, local directories, GitHub repository URLs, and GitLab repository URLs.

The runtime flow is:

```text
source input
  → collect text documents
  → generate rule-based tags
  → migrate TiDB schema if needed
  → upsert documents
  → upsert tags
  → link documents and tags
  → search with CLI or Web UI
```

## Correct Usage Steps

Use this sequence for a fresh local setup with a local binary:

1. Start TiDB:

```bash
docker compose up -d tidb
```

2. Build the binary:

```bash
go build ./cmd/kb-tool
```

3. Initialize the schema:

```bash
./kb-tool migrate
```

4. Ingest content from CLI or Web UI:

```bash
./kb-tool ingest ./README.md
./kb-tool ingest ./docs
./kb-tool ingest https://github.com/RenlySir/kb-tool.git
```

5. Search from CLI:

```bash
./kb-tool search tidb
```

6. Start the Web UI:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

7. Open the workbench:

```text
http://127.0.0.1:8080
```

8. In the browser:

- Paste multiple sources into the import panel, one per line.
- Click `开始导入`.
- Search or filter the document list.
- Select a document to inspect text, tags, and metadata.
- Add manual tags with comma-separated input.
- Select image documents to view them as images instead of binary text.

Use this sequence for a fresh Docker setup:

1. Build the image:

```bash
docker compose build kb-tool
```

2. Start TiDB:

```bash
docker compose up -d tidb
```

3. Initialize the schema:

```bash
docker compose run --rm kb-tool migrate
```

4. Start the Web UI and API:

```bash
docker compose up -d kb-tool
```

5. Open the workbench:

```text
http://127.0.0.1:8080
```

6. Log in to the Web UI. The default Docker Compose account is:

```text
username: admin
password: admin123
```

For real use, set your own API token and Web login:

```bash
KB_TOOL_API_TOKEN=<strong-token> \
KB_TOOL_ADMIN_USER=<user> \
KB_TOOL_ADMIN_PASSWORD=<password> \
docker compose up -d kb-tool
```

## 2. Prerequisites

Install these tools:

- Go 1.24 or newer.
- Git CLI.
- Docker Compose if using local TiDB.

Check versions:

```bash
go version
git --version
docker compose version
```

## 3. Start TiDB Locally

The repository includes a single-node TiDB service for local use:

```bash
docker compose up -d tidb
```

Check that the container is running:

```bash
docker compose ps
```

Stop it when finished:

```bash
docker compose down
```

## 4. Build the Tool

From the repository root:

```bash
go build ./cmd/kb-tool
```

This creates `./kb-tool`. The binary is ignored by Git.

## 4.1 Build and Run with Docker

Build only the image:

```bash
docker build -t kb-tool:local .
```

Run the full local stack:

```bash
docker compose build kb-tool
docker compose up -d tidb
docker compose run --rm kb-tool migrate
docker compose up -d kb-tool
```

The `kb-tool` service uses these defaults:

```text
TIDB_HOST=tidb
TIDB_PORT=4000
TIDB_USER=root
TIDB_DATABASE=kb
KB_TOOL_API_TOKEN=dev-token
KB_TOOL_ADMIN_USER=admin
KB_TOOL_ADMIN_PASSWORD=admin123
KB_TOOL_MAX_FILE_BYTES=512MiB
KB_TOOL_MAX_TEXT_BYTES=2MiB
```

The service listens on `0.0.0.0:8080` inside the container and is published to `127.0.0.1:8080` through Docker. Because the server is externally bound, an API token is required. The browser obtains that token through the configured Web login.

Run CLI commands through Compose:

```bash
docker compose run --rm kb-tool ingest /workspace/README.md
docker compose run --rm kb-tool ingest https://github.com/RenlySir/kb-tool.git
docker compose run --rm kb-tool search tidb
```

The Compose service mounts the repository read-only at `/workspace`, so local paths should use `/workspace/...` when commands run in the container. The runtime image includes the Git CLI for GitHub and GitLab ingestion. Private repositories still require credentials or SSH configuration to be available inside the container.

Override large-file limits for Docker runs:

```bash
KB_TOOL_MAX_FILE_BYTES=1GiB KB_TOOL_MAX_TEXT_BYTES=4MiB docker compose up -d kb-tool
```

## 5. Configure TiDB

Default connection:

```text
host: 127.0.0.1
port: 4000
user: root
password: empty
database: kb
```

Use environment variables for repeated commands:

```bash
export TIDB_HOST=127.0.0.1
export TIDB_PORT=4000
export TIDB_USER=root
export TIDB_PASSWORD=
export TIDB_DATABASE=kb
export KB_TOOL_MAX_FILE_BYTES=512MiB
export KB_TOOL_MAX_TEXT_BYTES=2MiB
```

Or pass flags per command:

```bash
./kb-tool migrate \
  -tidb-host 127.0.0.1 \
  -tidb-port 4000 \
  -tidb-user root \
  -tidb-password "" \
  -tidb-database kb
```

File-size settings:

```bash
./kb-tool ingest -max-file-bytes 512MiB -max-text-bytes 2MiB ./docs
```

Accepted suffixes are `KB`, `KiB`, `MB`, `MiB`, `GB`, and `GiB`. Raw byte counts are also accepted.

## 6. Initialize Schema

Run migration before the first ingest:

```bash
./kb-tool migrate
```

Expected output:

```text
migration complete
```

The command creates the database if needed and then creates these tables:

- `kb_documents`
- `kb_tags`
- `kb_document_tags`

`kb_documents` includes text fields plus MIME type, binary marker, and optional asset bytes for image preview.

## 7. Ingest Content

### 7.1 Ingest One File

```bash
./kb-tool ingest ./README.md
```

Expected output:

```text
ingested 1 documents with N tag assignments
```

### 7.2 Ingest a Directory

```bash
./kb-tool ingest ./docs
```

Directory ingestion walks files recursively and skips known noisy folders:

- `.git`
- `.idea`
- `.vscode`
- `.terraform`
- `.cache`
- `__pycache__`
- `node_modules`
- `vendor`
- `dist`
- `build`
- `target`

Large-file and binary behavior:

- The default single-file limit is `512MiB`.
- Files above `KB_TOOL_MAX_FILE_BYTES` are skipped.
- Text and Markdown files are read with streaming hashing; only the first `KB_TOOL_MAX_TEXT_BYTES` bytes are stored as searchable content.
- `.docx`, `.xlsx`, and `.pptx` are parsed as Office Open XML ZIP packages. The tool extracts text from XML entries and does not store the original package bytes in TiDB.
- `.doc`, `.xls`, and `.ppt` are recognized as legacy Office binary files and recorded as metadata-only asset documents. Full text parsing for these formats is planned through LibreOffice or Unstructured.
- Supported image assets are stored with bytes and previewed through the Web UI.

### 7.3 Ingest a GitHub Repository

```bash
./kb-tool ingest https://github.com/RenlySir/kb-tool.git
```

The tool runs:

```bash
git clone --depth 1 <repo-url> <temporary-directory>
```

Then it collects text files from the cloned directory.

### 7.4 Ingest a GitLab Repository

```bash
./kb-tool ingest git@gitlab.com:group/project.git
```

Private repositories require local Git credentials or SSH keys to already be configured.

## 8. Search

Search content:

```bash
./kb-tool search tidb
```

Limit result count:

```bash
./kb-tool search -limit 5 "knowledge base"
```

Output format:

```text
[document_id] source_type path tags=tag1,tag2
    snippet
```

The current search is SQL keyword search over document title, path, and content. Vector search is planned but not implemented yet.

## 9. Tagging Behavior

The current tagger is deterministic and rule based.

Signals used:

- File language inferred from extension.
- File path.
- File title.
- File content.

Examples of generated tags:

- `go`
- `markdown`
- `database`
- `mysql`
- `tidb`
- `github`
- `gitlab`
- `knowledge-base`
- `backend`
- `documentation`
- `devops`

Repeated ingestion recalculates tags and replaces the existing tag links for each document.

Manual tags added from Web UI or REST API are appended to the existing document/tag links.

## 10. Verify Data in TiDB

Connect to TiDB with any MySQL-compatible client:

```bash
mysql -h 127.0.0.1 -P 4000 -u root kb
```

Check document count:

```sql
SELECT COUNT(*) FROM kb_documents;
```

Check tags:

```sql
SELECT name FROM kb_tags ORDER BY name;
```

Check document/tag links:

```sql
SELECT d.path, GROUP_CONCAT(t.name ORDER BY t.name) AS tags
FROM kb_documents d
JOIN kb_document_tags dt ON dt.document_id = d.id
JOIN kb_tags t ON t.id = dt.tag_id
GROUP BY d.id, d.path
ORDER BY d.updated_at DESC
LIMIT 20;
```

## 11. Common Operations

### Re-ingest Updated Content

Run the same ingest command again:

```bash
./kb-tool ingest ./docs
```

Documents are matched by `(content_hash, path_hash)`. If content changes, a new content hash is generated. The current schema keeps the changed document as a new logical row when the content hash changes, while `path_hash` avoids oversized unique indexes for long file paths.

### Reset Local Data

For local development only:

```sql
DROP DATABASE kb;
```

Then run:

```bash
./kb-tool migrate
```

### Use a Remote TiDB

```bash
./kb-tool ingest ./docs \
  -tidb-host <host> \
  -tidb-port 4000 \
  -tidb-user <user> \
  -tidb-password <password> \
  -tidb-database kb
```

### Start the Web UI

```bash
./kb-tool server -addr 127.0.0.1:8080
```

Then open:

```text
http://127.0.0.1:8080
```

### Bind Web UI for External Access

When binding to a non-local address, an API token is required:

```bash
KB_TOOL_API_TOKEN=<token> \
KB_TOOL_ADMIN_USER=admin \
KB_TOOL_ADMIN_PASSWORD=<password> \
./kb-tool server -addr 0.0.0.0:8080
```

REST clients must send:

```http
Authorization: Bearer <token>
```

## 12. Web UI, REST API, and Planned MCP Access

This section describes the current Web UI and REST API, plus the planned MCP surface.

### 12.1 Server Mode

The command is:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

One process exposes Web UI and REST API now, with MCP planned:

```text
browser users
  → Web UI

external systems
  → REST API

AI agents and coding tools
  → MCP tools (planned)
```

The server reuses the same TiDB configuration flags and environment variables as `migrate`, `ingest`, and `search`.

When `KB_TOOL_API_TOKEN` is configured, the Web UI uses `/api/login` for username/password login and stores the returned token in browser `localStorage` under `kbToolApiToken`. Docker Compose defaults are:

```text
username: admin
password: admin123
api token: dev-token
```

Change all three values before exposing the service outside a trusted local network.

### 12.2 Web UI Capabilities

The Web UI is a knowledge-base workbench, not a marketing page.

Primary screens:

- Management navigation:
  - The Web UI uses an admin-system layout.
  - Functional modules are collapsed by default in the left navigation.
  - Click Database Management, Import, Tag Management, or Tag Filter to expand the matching drawer.
- Knowledge overview:
  - Show total knowledge items, total tags, image/binary assets, selected items, and current filter.
  - Show source distribution so operators can see whether content came from files, GitHub, GitLab, Office documents, images, or other sources.
  - Show content type distribution based on parsed language or MIME type.
  - Show recently ingested documents and allow opening detail from the overview.
- Database management:
  - Create TiDB connection profiles with name, IP/host, port, username, password, and database.
  - Test a connection before using it.
  - Activate one connection as the current knowledge-base database.
  - Store connection profiles in `KB_TOOL_CONNECTIONS_FILE`; password values are not returned by list APIs.
- Import screen:
  - Choose the project/source type: auto, local file/directory, GitHub, GitLab, Office document, or image asset.
  - Batch import local file paths, local directories, GitHub URLs, and GitLab URLs.
  - Accept one source per line.
  - Show import result counts for documents, tags, skipped files, and failures.
- Document list:
  - Search by keyword.
  - Filter by tags.
  - Select one or more documents for batch tagging.
  - Show title, path, source type, MIME type, tags, and update time.
- Document detail:
  - Render text documents as readable text.
  - Render image documents as visible images.
  - Show source URI, path, content hash, language, MIME type, and size.
  - Add manual tags to the current document.
- Tag view:
  - List all tags.
  - Show document counts per tag.
  - Filter documents by tag.
- Batch tag panel:
  - Add one or more comma-separated tags to all selected documents.
  - Reuse the same REST endpoint as external automation.
- Planned AI tagging panel:
  - Select one or more documents.
  - Run AI tag generation.
  - Show generated tags with confidence and evidence.
  - Allow users to approve, reject, or edit generated tags before final use.

### 12.3 REST API

The REST API supports external systems and automation.

Endpoints:

```http
GET    /api/health
GET    /api/session
POST   /api/login
GET    /api/overview
GET    /api/connections
POST   /api/connections
POST   /api/connections/test
POST   /api/connections/{id}/activate
GET    /api/documents
GET    /api/documents/{id}
GET    /api/documents/{id}/asset
GET    /api/tags
GET    /api/search?q=tidb&tags=database,vector
POST   /api/ingest
POST   /api/documents/tags
POST   /api/documents/{id}/tags
```

`DELETE /api/documents/{id}/tags/{tag}` is planned and not implemented yet.

`GET /api/overview` returns a management summary used by the Web UI dashboard:

```json
{
  "overview": {
    "total_documents": 128,
    "total_tags": 36,
    "binary_documents": 14,
    "image_documents": 9,
    "source_types": [{"name": "file", "count": 80}],
    "languages": [{"name": "markdown", "count": 42}],
    "recent_documents": [{"id": 12, "title": "README.md", "source_type": "github"}]
  }
}
```

Example connection request:

```json
{
  "name": "生产知识库",
  "host": "10.0.0.8",
  "port": 4000,
  "user": "kb_user",
  "password": "secret",
  "database": "kb_prod"
}
```

`GET /api/connections` returns `has_password` but never returns password values.

Example ingest request:

```json
{
  "source_type": "github",
  "sources": [
    "https://github.com/RenlySir/kb-tool.git"
  ]
}
```

Supported `source_type` values are `auto`, `file`, `github`, `gitlab`, `office`, and `image`. The server validates the obvious cases before sending each source to the collector, which helps catch selecting a GitHub project type for a local Office document.

Example batch tag request:

```json
{
  "document_ids": [1, 2, 3],
  "tags": ["AI", "产品手册"]
}
```

Example search response:

```json
{
  "results": [
    {
      "id": 1,
      "title": "README.md",
      "path": "README.md",
      "source_type": "file",
      "mime_type": "text/markdown",
      "tags": ["documentation", "knowledge-base"],
      "snippet": "kb-tool is a Go command-line tool..."
    }
  ]
}
```

### 12.4 Planned MCP Tools

The planned MCP surface should let AI agents access the knowledge base without scraping the Web UI.

Tools:

- `search_knowledge`: keyword or hybrid search with optional tag filters.
- `get_document`: fetch document detail and text content.
- `list_documents`: list recent or filtered documents.
- `list_tags`: list tags and counts.
- `ingest_source`: ingest a local path or Git URL.
- `generate_document_tags`: call the configured LLM provider and return proposed tags with confidence and evidence.
- `add_document_tags`: add manual tags to a document.
- `get_document_asset_info`: return asset MIME type, size, and fetch URL for images or binary files.

Image assets should not be embedded as garbled text in MCP results. MCP should return metadata and an asset URL so clients can fetch or preview the image appropriately.

### 12.5 Image and Binary Asset Handling

The current release supports image ingestion and visual preview for common image MIME types.

Storage rules:

- Text content goes into text fields for search and display.
- Image bytes are stored as an asset in TiDB `LONGBLOB`. A future release can move raw assets to MinIO.
- TiDB stores metadata such as MIME type, size, hash, path, source URI, and tags.
- Non-image binary files are tracked as assets but not rendered as text.

Preview rules:

```http
GET /api/documents/{id}/asset
```

For an image, the response should include:

```http
Content-Type: image/png
Content-Disposition: inline
```

The Web UI should render it with:

```html
<img src="/api/documents/123/asset" alt="document image preview">
```

This prevents image bytes from being displayed as unreadable text.

### 12.6 Access Control

Default local mode:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

External access requires an API token:

```bash
./kb-tool server \
  -addr 0.0.0.0:8080 \
  -api-token <token> \
  -admin-user admin \
  -admin-password <password>
```

Rules:

- Binding to `127.0.0.1` is allowed without a token for local development.
- Binding to `0.0.0.0` requires `-api-token` or `KB_TOOL_API_TOKEN`.
- Web login uses `-admin-user` / `KB_TOOL_ADMIN_USER` and `-admin-password` / `KB_TOOL_ADMIN_PASSWORD`.
- REST API endpoints accept:

```http
Authorization: Bearer <token>
```

The Web UI obtains the API token through `/api/login` and sends it through request headers. Image preview requests use the token only on `/api/documents/{id}/asset`, so protected image assets render as images instead of unreadable binary text.

## 13. Planned LLM Connection and AI Tagging

This section describes the planned LLM integration for AI-assisted tagging. It is not implemented in the current CLI-only release.

### 13.1 Goals

The AI tagger should add intelligent tags without replacing deterministic rule tags or human review.

Required behavior:

- Connect to different model providers through a stable provider interface.
- Generate tags from document text, source metadata, existing rule tags, and optional extracted entities.
- Return structured JSON instead of free-form prose.
- Store every AI-generated tag assignment with source, model, confidence, evidence, and review status.
- Let users approve, reject, or edit model-generated tags from Web UI or API.

### 13.2 Provider Types

The planned provider abstraction should support:

- OpenAI-compatible APIs:
  - OpenAI
  - TiDB Cloud AI gateway if exposed through an OpenAI-compatible endpoint
  - Internal enterprise LLM gateways
  - Any service exposing `/v1/chat/completions`
- Ollama:
  - Local models for private/offline tagging
  - Example models: Qwen, Llama, Gemma
- Future providers:
  - Native SDK integrations
  - Batch tagging services
  - Fine-tuned classification endpoints

### 13.3 Configuration

Environment variables:

```bash
export KB_TOOL_LLM_PROVIDER=openai-compatible
export KB_TOOL_LLM_BASE_URL=https://api.openai.com/v1
export KB_TOOL_LLM_API_KEY=<token>
export KB_TOOL_LLM_MODEL=gpt-4.1-mini
export KB_TOOL_LLM_TIMEOUT_SECONDS=60
```

Local Ollama example:

```bash
export KB_TOOL_LLM_PROVIDER=ollama
export KB_TOOL_LLM_BASE_URL=http://127.0.0.1:11434
export KB_TOOL_LLM_MODEL=qwen2.5:7b
```

Planned server flags:

```bash
./kb-tool server \
  -llm-provider openai-compatible \
  -llm-base-url https://api.openai.com/v1 \
  -llm-model gpt-4.1-mini
```

The API key should come from `KB_TOOL_LLM_API_KEY` or a secret manager, not from command history.

### 13.4 AI Tagging Request

The AI tagger should receive a compact input:

```json
{
  "document": {
    "id": 123,
    "title": "TiDB Vector Search Notes",
    "path": "docs/tidb-vector.md",
    "source_type": "file",
    "mime_type": "text/markdown",
    "language": "markdown"
  },
  "text": "shortened document or selected chunks",
  "existing_tags": ["tidb", "database"],
  "metadata": {
    "author": "optional",
    "created_at": "optional"
  }
}
```

The prompt should ask the model to produce concise, normalized tags and evidence. It should avoid sending full oversized documents when selected chunks are enough.

### 13.5 AI Tagging Response

The model response must be parsed as JSON:

```json
{
  "tags": [
    {
      "name": "vector-search",
      "confidence": 0.92,
      "evidence": "The document explains VECTOR(D), VEC_COSINE_DISTANCE, and HNSW indexes.",
      "category": "technical-topic"
    }
  ]
}
```

Tag assignments should be stored with:

```text
source = ai
review_status = pending
confidence = model confidence or calibrated score
evidence = short model-provided explanation
model = configured model name
provider = configured provider name
```

Human-approved AI tags can later move to:

```text
review_status = approved
```

### 13.6 API and MCP Surface

Planned REST endpoint:

```http
POST /api/documents/{id}/tags/generate
```

Request:

```json
{
  "mode": "ai",
  "max_tags": 8,
  "review_status": "pending"
}
```

Response:

```json
{
  "document_id": 123,
  "provider": "openai-compatible",
  "model": "gpt-4.1-mini",
  "tags": [
    {
      "name": "vector-search",
      "confidence": 0.92,
      "evidence": "Mentions TiDB VECTOR and cosine distance.",
      "review_status": "pending"
    }
  ]
}
```

Planned MCP tool:

```text
generate_document_tags
```

Inputs:

```json
{
  "document_id": 123,
  "max_tags": 8
}
```

Outputs should mirror the REST response.

### 13.7 Security and Privacy

Rules:

- Do not log `KB_TOOL_LLM_API_KEY`.
- Do not store model API keys in TiDB.
- Redact API keys from errors and debug output.
- Allow AI tagging to be disabled by default in production.
- Prefer sending chunks or summaries instead of entire sensitive documents.
- Record which provider and model generated each tag for auditability.
- If external LLM calls are not allowed, use an internal OpenAI-compatible gateway or Ollama.

## 14. Troubleshooting

### `connection refused`

TiDB is not reachable.

Check:

```bash
docker compose ps
```

Confirm host and port:

```bash
nc -vz 127.0.0.1 4000
```

### `Access denied`

The TiDB username or password is wrong. Check `TIDB_USER`, `TIDB_PASSWORD`, and command flags.

### `git clone` failed

Check that Git is installed:

```bash
git --version
```

For private repositories, confirm credentials:

```bash
ssh -T git@github.com
ssh -T git@gitlab.com
```

### Ingest Count Is Lower Than Expected

Possible reasons:

- Binary files are skipped.
- Known dependency/build directories are skipped.
- Files over the collector size limit are skipped.
- Unsupported file types may still be skipped if they are not valid UTF-8 text.

### Search Returns Nothing

Check that content was ingested:

```sql
SELECT id, path, title FROM kb_documents ORDER BY updated_at DESC LIMIT 10;
```

Then search for a term visible in `title`, `path`, or `content`.

### Web UI Cannot Connect to API

Check the server is running:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

Open:

```text
http://127.0.0.1:8080/api/health
```

Expected response:

```json
{"status":"ok"}
```

If Docker Compose is used, the API token defaults to `dev-token` and the Web login defaults to `admin` / `admin123`. Set your own credentials before starting the service:

```bash
KB_TOOL_API_TOKEN=<strong-token> \
KB_TOOL_ADMIN_USER=<user> \
KB_TOOL_ADMIN_PASSWORD=<password> \
docker compose up -d kb-tool
```

### Image Preview Shows Broken Image

Confirm the document is an image asset:

```sql
SELECT id, path, mime_type, is_binary, size_bytes
FROM kb_documents
WHERE id = <document_id>;
```

Supported preview MIME types:

- `image/png`
- `image/jpeg`
- `image/gif`
- `image/webp`

## 15. Recommended Expansion Plan

The intended production-grade roadmap is:

### Phase 1: Minimum Viable Pipeline

- Use Unstructured for Markdown and TXT parsing through a Python worker.
- Keep rule-based tagging.
- Add chunk tables and an embedding pipeline interface.
- Use TiDB as the only planned relation and vector storage backend. For local development, start with exact vector search without HNSW indexing so TiFlash is not required.

### Phase 2: More Sources and Metadata

- Add GitHub API ingestion for README files and code comments.
- Add LangExtract-style structured extraction for authors, dates, entities, and relations.
- Add LLM-generated AI tags with confidence, evidence, provider name, model name, and pending review status.

### Phase 3: Search and Storage Upgrade

- Use TiDB as unified relation and vector storage.
- Add chunk tables and embedding tables.
- Store embeddings in TiDB `VECTOR(D)` columns, where `D` is the embedding model dimension.
- Use `VEC_COSINE_DISTANCE` for text semantic retrieval by default.
- Add TiFlash replicas and HNSW vector indexes in production environments that need lower-latency approximate nearest-neighbor search.
- Add tag-filtered search.
- Add hybrid retrieval: keyword + tag filter + vector similarity.

### Phase 4: Production Extensions

- Add MinIO object storage for raw files and extracted artifacts.
- Add MCP server mode for AI agents such as Cursor and Claude Code.
- Add OpenAI-compatible and Ollama LLM providers for AI tagging and metadata enrichment.
- Add CocoIndex-style incremental indexing and lineage.
- Add OpenTagging/MetaLabel-style tag lifecycle management.
- Add Dify Knowledge Pipeline and Airweave adapters.

## 16. Operational Checklist

For a fresh local run:

```bash
docker compose up -d tidb
go build ./cmd/kb-tool
./kb-tool migrate
./kb-tool ingest ./README.md
./kb-tool search kb-tool
go test ./...
```

For a fresh Docker run:

```bash
docker compose build kb-tool
docker compose up -d tidb
docker compose run --rm kb-tool migrate
docker compose up -d kb-tool
docker compose ps
```

For committing changes:

```bash
gofmt -w cmd internal
go test ./...
go build ./cmd/kb-tool
docker build -t kb-tool:local .
git status --short
```
