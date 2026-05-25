# kb-tool

`kb-tool` is a Go CLI for ingesting local files, directories, GitHub repositories, and GitLab repositories into a TiDB-backed knowledge base. It extracts text documents, applies deterministic tags, and stores documents plus tag relationships in TiDB through the MySQL protocol.

## Features

- Ingest a single file, a directory tree, a GitHub URL, or a GitLab URL.
- Skip binary files and noisy folders such as `.git`, `node_modules`, `vendor`, `dist`, and `target`.
- Assign stable rule-based tags such as `go`, `tidb`, `mysql`, `database`, `github`, and `knowledge-base`.
- Create and migrate TiDB tables automatically.
- Search ingested content from the CLI.

## Requirements

- Go 1.24 or newer.
- Git CLI for repository ingestion.
- TiDB reachable through the MySQL protocol.

For local testing, start TiDB with:

```bash
docker compose up -d tidb
```

## Build

```bash
go build ./cmd/kb-tool
```

## Configure TiDB

Defaults target a local TiDB instance:

```text
TIDB_HOST=127.0.0.1
TIDB_PORT=4000
TIDB_USER=root
TIDB_PASSWORD=
TIDB_DATABASE=kb
```

You can use environment variables or flags:

```bash
kb-tool migrate \
  -tidb-host 127.0.0.1 \
  -tidb-port 4000 \
  -tidb-user root \
  -tidb-database kb
```

## Usage

Migrate schema:

```bash
kb-tool migrate
```

Ingest a local directory:

```bash
kb-tool ingest /path/to/docs
```

Ingest GitHub or GitLab repositories:

```bash
kb-tool ingest https://github.com/RenlySir/kb-tool.git
kb-tool ingest git@gitlab.com:group/project.git
```

Search:

```bash
kb-tool search tidb
```

## Schema

The tool creates three tables:

- `kb_documents`: document metadata and full text.
- `kb_tags`: normalized tag names.
- `kb_document_tags`: many-to-many document/tag links.

Documents are upserted by `(content_hash, path)`, so repeated ingestion updates existing rows instead of duplicating unchanged files.
