# KB Tool Design

## Goal

Build a Go command-line tool that ingests local files, directories, GitHub repositories, and GitLab repositories into a TiDB-backed knowledge base with automatic tagging.

## Architecture

The tool is a small Go CLI with four internal packages. `source` collects documents from local paths or shallow-cloned Git repositories. `tagger` applies deterministic rule-based tags. `ingest` coordinates collection, tagging, migration, and persistence. `store` owns TiDB connectivity, schema migration, upsert, tag linking, and search.

## Storage

TiDB is accessed through the MySQL protocol with `github.com/go-sql-driver/mysql`. The schema uses `kb_documents`, `kb_tags`, and `kb_document_tags`. Documents are upserted by content hash and path, while tags are normalized and linked through a many-to-many table.

## Commands

- `kb-tool migrate`: create or update schema.
- `kb-tool ingest <input>`: ingest a file, directory, GitHub URL, or GitLab URL.
- `kb-tool search <query>`: search title, path, and content.

## Tagging

The first version uses deterministic rules instead of an LLM so results are reproducible and the tool works offline. Tags are derived from language, path, and content keywords. The tagger boundary allows future replacement with an embedding model or LLM classifier.

## Error Handling

The CLI exits with code `2` for argument errors and code `1` for runtime failures. Repository ingestion requires the local `git` command. Binary files, oversized files, and dependency/build folders are skipped.

## Testing

Unit tests cover local file collection, Git remote parsing, Git clone orchestration, tagging, ingest orchestration, TiDB DSN construction, migration statements, and CLI parsing.
