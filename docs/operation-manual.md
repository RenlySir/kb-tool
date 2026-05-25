# kb-tool Operation Manual

This manual explains how to install, configure, run, verify, and troubleshoot `kb-tool`.

## 1. System Overview

`kb-tool` ingests content into a TiDB-backed knowledge base. The current release supports local text files, local directories, GitHub repository URLs, and GitLab repository URLs.

The runtime flow is:

```text
source input
  → collect text documents
  → generate rule-based tags
  → migrate TiDB schema if needed
  → upsert documents
  → upsert tags
  → link documents and tags
  → search with CLI
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

Binary files and files larger than the collector limit are skipped.

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

Documents are matched by `(content_hash, path)`. If content changes, a new content hash is generated. The current schema keeps the changed document as a new logical row when the content hash changes.

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

## 12. Troubleshooting

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

## 13. Recommended Expansion Plan

The intended production-grade roadmap is:

### Phase 1: Minimum Viable Pipeline

- Use Unstructured for Markdown and TXT parsing through a Python worker.
- Keep rule-based tagging.
- Add chunk tables and an embedding pipeline interface.
- Use TiDB as the only planned relation and vector storage backend. For local development, start with exact vector search without HNSW indexing so TiFlash is not required.

### Phase 2: More Sources and Metadata

- Add GitHub API ingestion for README files and code comments.
- Add LangExtract-style structured extraction for authors, dates, entities, and relations.
- Add AI-generated tags with confidence and evidence.

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
- Add CocoIndex-style incremental indexing and lineage.
- Add OpenTagging/MetaLabel-style tag lifecycle management.
- Add Dify Knowledge Pipeline and Airweave adapters.
- Add MCP tools for AI agents such as Cursor and Claude Code.

## 14. Operational Checklist

For a fresh local run:

```bash
docker compose up -d tidb
go build ./cmd/kb-tool
./kb-tool migrate
./kb-tool ingest ./README.md
./kb-tool search kb-tool
go test ./...
```

For committing changes:

```bash
gofmt -w cmd internal
go test ./...
go build ./cmd/kb-tool
git status --short
```
