# DashScope Embedding Cutover

## Objective

Configure tenant `10001` to use the remote Alibaba Cloud Model Studio
`text-embedding-v4` model at 1024 dimensions. Keep every embedding request on
one explicit provider, model, region, and endpoint. Do not start Ollama, infer a
provider from a URL, or fall back to another model.

## Implementation Steps

- [x] Confirm the selected production model and dimension from Alibaba Cloud's
  current RAG guidance.
- [x] Make the shared OpenAI-compatible embedding request contain only fields
  defined by the selected provider protocol.
- [x] Replace model-name inspection in Aliyun embedding routing with an explicit
  typed modality field.
- [x] Add focused protocol checks for request serialization and routing.
- [x] Reuse the same typed provider and endpoint validation before persistence
  and at runtime.
- [x] Start the existing WeKnora API without Ollama.
- [x] Store the supplied credential through the existing secret/configuration
  path without logging or committing it.
- [x] Create and promote one active remote `text-embedding-v4` model at 1024
  dimensions for tenant `10001`.
- [x] Run the WeKnora model debug operation and a direct remote embedding request.
- [x] Build or rebuild a small knowledge-base index and prove retrieval through
  the configured model.
- [x] Run `ai-code-check` and Ponytail acceptance for source changes.

## Affected Areas

- `internal/models/embedding`: remote request serialization and explicit
  provider routing.
- `internal/application/repository`: fail-closed remote Embedding provider
  validation before persistence.
- `.env.local`: one exact DashScope host in the SSRF allowlist.
- Tenant `10001` model configuration and its encrypted API credential.
- The existing WeKnora API runtime and knowledge-base vector index.

## Verification Approach

- Unit checks must prove no non-standard truncation field is emitted unless the
  selected provider contract explicitly supports it.
- Routing checks must use typed configuration rather than model-name matching.
- Runtime evidence must show the exact non-secret provider, model, endpoint
  region, dimension, active/default status, response dimension, and retrieval
  result.
- Port `11434` and local Ollama model directories must remain absent.
- Logs and evidence must never contain API credentials.

## Progress

The exact Beijing endpoint and 1024-dimensional response were verified with the
supplied credential. Tenant `10001` now has exactly one default Embedding model:
remote provider `aliyun`, model `text-embedding-v4`, dimension 1024. The API
credential is stored as an encrypted envelope. A dedicated knowledge base was
indexed successfully and returned its only chunk through vector-only search.
Focused checks, race detection, vet, the production build, and two independent
`ai-code-check` passes succeed. The reviewed runtime returned a 1024-dimensional
embedding and the vector-only query returned the indexed verification chunk.
Ponytail review found no removable abstraction or dependency.

## Final Outcome

Completed. Tenant `10001` now uses the Beijing DashScope OpenAI-compatible
endpoint with remote provider `aliyun`, model `text-embedding-v4`, and 1024
dimensions as its single default Embedding model. The credential is stored by
the encrypted model-credential path. The latest reviewed binary is healthy,
model debug succeeds, and real vector-only retrieval succeeds without Ollama.

The separate KnowledgeQA default is still a local Qwen model. It must be
replaced with a remote DeepSeek model before the broader requirement that every
LLM request use DeepSeek can be declared complete.
