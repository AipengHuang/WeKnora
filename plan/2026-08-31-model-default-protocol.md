# Model Default Protocol

## Objective

Add one explicit API operation that promotes an active tenant model to the
single default model for its type. This closes the control-plane gap blocking
Platform tenant provisioning without direct database writes or fallback
selection.

## Implementation steps

- [ ] Add `PUT /api/v1/models/:id/default` to the existing model route group.
- [ ] Reuse the model repository and `ClearDefaultByType` behavior so the
  selected model is the only default in its tenant and type.
- [ ] Reject missing, inactive, cross-tenant, and unauthorized built-in models.
- [ ] Add focused service and HTTP checks for the protocol.
- [ ] Configure and debug the tenant `10001` Ollama chat and embedding models.
- [ ] Re-run Platform knowledge-space sync, then create a project-owned Agent
  with explicit model and knowledge-base bindings.

## Affected areas

- WeKnora model API route, handler, service, and repository contract.
- Local Platform E2E control-plane configuration and tenant `10001` only.

## Verification

- Focused Go tests for default promotion and HTTP routing.
- Real API calls proving one active default per type for tenant `10001`.
- Real model debug calls for `qwen3:0.6b` and `nomic-embed-text` (768 dimensions).
- Platform sync evidence for a ready project knowledge-base binding.
- Real WeKnora session/stream evidence through the project-owned Agent.
- `ai-code-check` and Ponytail review after implementation.

## Progress

- [x] Root cause confirmed: current model create/update APIs cannot set
  `is_default`, while Platform requires active default chat and embedding
  models.
- [ ] Implementation in progress.

## Final outcome

Pending live protocol verification.
