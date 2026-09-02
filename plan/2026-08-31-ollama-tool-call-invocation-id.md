# Ollama Tool Call Provider Identity

## Objective

Normalize Ollama tool calls at the provider boundary by preserving the official
`tool_calls[].id` returned by the current Ollama protocol. The adapter must fail
closed when that required identity is absent instead of presenting a function
index or locally synthesized value as a provider ID.

## Implementation Steps

- [x] Trace every Ollama streaming and non-streaming tool-call conversion.
- [x] Preserve each non-empty provider ToolCall ID at the shared Ollama response
      conversion boundary.
- [x] Reject missing provider identity; do not add local synthesis, collector
      inference, or UI recovery.
- [x] Reject malformed Ollama tool schemas during request construction and
      delete the unused reverse tool-definition converter.
- [x] Add focused tests proving provider IDs remain exact, missing IDs fail
      closed, and function indexes survive history through typed metadata.
- [x] Remove the `execute_skill_script.args` string decoder so the declared
      `[]string` schema is the only accepted protocol.
- [x] Remove completion-time answer synthesis so clients receive only answer
      events emitted by the formal agent stream.
- [x] Run focused Go tests, formatting, diff checks, `ai-code-check`, and
      Ponytail acceptance.
- [ ] Build the strict server binary, inherit the current local process
      configuration without exposing secrets, and explicitly set
      `RETRIEVE_DRIVER=postgres`.
- [ ] Replace only the backend listening on `127.0.0.1:18080`, then verify
      health, authenticated Agent SSE framing, and strict official tool-call
      identity behavior without sending a business prompt.

## Affected Areas

- `internal/models/chat/ollama.go`
- `internal/agent/tools/skill_execute.go`
- `internal/handler/session/agent_stream_completion.go`
- Focused adapter, Skill input, and completion-stream tests.
- Tenant `10001` runtime configuration through existing public APIs after the
  provider fix is verified.

## Verification Approach

- Two official provider IDs with the same function index remain distinct.
- A response without an official provider ID fails before entering agent state.
- Streamed and non-streamed Ollama responses use the same conversion rule.
- Official provider-issued IDs remain exact; no string matching, aliases,
  synthesis, or compatibility fallback is added.
- Skill argument strings fail JSON decoding while string arrays remain valid.
- Completion appends only the formal complete event when no answer event was
  emitted; it does not manufacture an answer delta.

## Progress

- [x] Probed Ollama v0.23.2 with the configured Qwen models and confirmed native
      responses include unique `tool_calls[].id` values.
- [x] Confirmed the current adapter discards that ID and serializes the function
      index as `"0"`, causing later agent rounds to preserve and reuse the same
      apparent provider ID.
- [x] Identified two additional response-repair paths in the same live flow:
      string-to-args coercion and completion-time answer synthesis.
- [x] Preserved official Ollama IDs and typed function-index metadata; missing
      provider identity now fails closed in response and history conversion.
- [x] Removed both compatibility repairs and verified their strict boundaries.
- [x] Made tool-schema conversion fail closed and deleted the unused reverse
      converter found by the final repository review.
- [x] Created the tenant `10001` Docker sandbox, installed the proof Skill to
      `ready`, merged the complete Agent document with Skill and MCP selections,
      and revoked the ephemeral full-access key with an absent-from-list check.

## Final Outcome

The provider, Skill-input, and completion-stream boundaries now accept only
their formal typed protocols. Focused package tests, compilation, repository
review, and Ponytail acceptance pass. Runtime configuration is ready for a
fresh process. Runtime replacement and protocol probes are in progress; no
business prompt has been sent.
