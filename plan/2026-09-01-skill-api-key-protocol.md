# Skill API Key Protocol

## Objective

Make the documented Skill flow work for scoped WeKnora clients without issuing
a full-access tenant key. Read-only Agent clients need to discover usable
Skills, while an explicit operator credential needs to manage tenant Sandbox
configuration and Skill image lifecycle.

## Implementation

1. Declare the existing Agent read/manage capability policy on Skill read
   routes.
2. Add one exact `manage_sandboxes` capability for Sandbox configuration and
   Skill image lifecycle routes; preserve full-access authority as an
   alternative.
3. Keep the permanent Web runtime key unchanged and issue a separate operator
   credential through the existing external-tenant control-plane endpoint.
4. Add the smallest parser and route-policy regressions for both scopes.

## Affected Areas

- `internal/router/routes_agent.go`
- `internal/router/routes_infra.go`
- `internal/router/rbac.go`
- `internal/router/router_api_key_capabilities_test.go`
- `internal/types/tenant_api_key.go`
- `internal/types/tenant_api_key_capability.go`
- `internal/types/tenant_api_key_scope_test.go`

## Verification

- Focused Go router tests.
- Rebuild the WeKnora server.
- Real `GET /skills` using the isolated tenant runtime key and signed external
  principal.
- Real Sandbox creation, Skill installation, and `GET /skills` using a separate
  scoped operator key.
- Final repository-aware code check with material findings resolved.

## Progress

- Reproduced HTTP 403 from the documented `GET /skills` request.
- Confirmed the API-key gate is default-deny and the route has no declared
  policy, while the tenant runtime already has `manage_agents`.
- Declared the Agent capabilities on Skill reads, rebuilt the server, and
  verified the documented request now returns HTTP 200.
- Confirmed from `SkillHandler.ListSkills` that usable Skills must be installed,
  ready, and enabled on the requested `sandbox_config_id`; host preloads are not
  a valid substitute.
