# MCP Result Metadata Protocol Preservation

## Objective

Preserve the standard MCP tool definition metadata and `CallToolResult` fields from the MCP server through WeKnora streaming and persisted message history, so Platform Web receives typed `structuredContent` and `_meta` data without tool-name or text inference.

## Implementation steps

1. Preserve the MCP SDK tool definition JSON, including `outputSchema`, `annotations`, and `_meta`, when converting `tools/list` results.
2. Preserve `structuredContent` and result `_meta` when converting `tools/call` results.
3. Add the standard fields and the exact tool definition to MCP tool-result data for both successful and failed calls.
4. Extend the strict Platform WeKnora chat and history contracts so live events and persisted history retain the same protocol fields.
5. Add focused regression checks for definition conversion, call-result conversion, live event parsing, and history presentation.

## Affected areas

- WeKnora MCP client conversion and MCP tool execution.
- WeKnora tool-result stream and persisted `agent_steps` data.
- Platform shared WeKnora chat contracts, Service history presenter, and Web chat state.

## Verification

- Run focused Go tests for MCP client and MCP tool result preservation.
- Run focused Platform data-contract, Service schema, and Web chat-state tests.
- Run repository type checks and build checks for changed projects.
- Call the configured standard MCP server and inspect a real WeKnora/Web tool event after restarting the changed runtime.
- Run `ai-code-check` and address all material findings.

## Progress

- [x] Located the first loss boundary in WeKnora MCP SDK conversion.
- [ ] Preserve standard fields in WeKnora.
- [ ] Preserve standard fields through Platform contracts and UI state.
- [ ] Complete automated checks.
- [ ] Complete live protocol verification.

## Final outcome

Pending implementation and live verification.
