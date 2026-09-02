# DeepSeek Provider Cutover

## Objective

Remove the local Ollama runtime and model storage, then route every WeKnora
chat, reasoning, tool-calling, and summarization LLM request through the current
official DeepSeek OpenAI-compatible API. No Ollama or provider fallback is
allowed.

## Implementation Steps

- [x] Stop the verified Ollama process and close port 11434.
- [x] Remove the Homebrew Ollama installation and its dedicated local models.
- [ ] Validate the supplied DeepSeek credential against the official model API.
- [ ] Create or update one `deepseek-v4-flash` remote Chat model through the
  typed WeKnora model API and make it the tenant default.
- [ ] Rebind the project Agent and knowledge-base summarization to that exact
  model, then delete local Ollama Chat model records.
- [ ] Verify a real streaming response with thinking and tool calls.
- [ ] Run repository-aware `ai-code-check` and Ponytail review if source code is
  changed.

## Affected Areas

- Local Ollama process, Homebrew formula, and model storage.
- Tenant 10001 WeKnora model, Agent, and knowledge-base configuration.
- The existing Platform Web conversation path that calls this tenant Agent.

## Verification Approach

- Require official DeepSeek `/models` to authenticate and include the exact
  current model identifier before storing the credential.
- Read model/Agent/knowledge-base state back through WeKnora APIs with secrets
  redacted.
- Prove port 11434 and the former model directories are absent.
- Submit and capture a real Browser conversation after the cutover.

## Progress

Ollama has been stopped and uninstalled. Its 2.0 GB dedicated model store,
default user directory, and test log were permanently removed after exact path
validation. DeepSeek Chat configuration is in progress.

## Final Outcome

Pending live DeepSeek configuration and conversation verification. DeepSeek
does not expose an Embedding API, so vector retrieval requires a separate
online Embedding provider; that gap must fail closed and must not revive an
Ollama fallback.
