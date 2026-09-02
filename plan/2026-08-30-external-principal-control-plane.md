# External Principal Control-Plane Patch

## Objective

Add the smallest upstream-friendly WeKnora patch required for Platform to
provision one tenant runtime API key and use signed external principals for
interactive chat. Tenant keys must never gain tenant-key administration.

## Implementation Steps

- [x] Add one strict platform-only `PUT /system/admin/external-tenants/:external_ref`
      protocol. The path value is a canonical UUID and is the sole idempotency
      identity; request names are never used for lookup or reconciliation.
- [x] Persist `tenants.external_ref` as nullable for upstream-created tenants and
      unique for Platform-created tenants in PostgreSQL and SQLite migrations.
- [x] Make repeated and concurrent PUT requests return the same tenant. A request
      arriving after the first database commit but after a lost HTTP response must
      resolve the persisted tenant without creating or deleting another tenant.
- [x] Keep the existing generic tenant POST unchanged. The Platform integration
      uses only the strict PUT protocol and does not add a POST retry fallback.
- [x] Add one strict platform-only
      `PUT /system/admin/external-tenants/:external_ref/api-keys/:external_credential_ref`
      protocol. Both path values are canonical UUIDs; the credential reference is
      the sole API-key idempotency identity and generic API-key POST is not used by
      Platform provisioning.
- [x] Persist `tenant_api_keys.external_ref` as nullable for upstream-created keys
      and globally unique for Platform-created keys in PostgreSQL and SQLite.
- [x] Make repeated and concurrent credential PUT requests return the same key ID
      and the same one-time token. A retry after a committed database transaction
      and lost HTTP response must recover the existing encrypted token without
      creating, naming, searching, or revoking another key.
- [x] Cover credential PUT replay, concurrency, rollback, strict platform
      capability enforcement, and protocol validation at repository/service/HTTP
      seams before marking this control-plane boundary complete.
- [x] Add migration, repository, service, handler, and route tests, then run
      affected packages, vet, source-size checks, diff checks, and `ai-code-check`.

- [x] Add route tests proving only platform keys with
  `system_tenants_read` can list/get tenant API keys.
- [x] Add route tests proving only platform keys with
  `system_tenants_manage` can create/delete tenant API keys.
- [x] Register the exact GET/POST/DELETE tenant API-key routes for platform
  control-plane access while retaining tenant-key denial.
- [x] Add focused route tests proving a tenant runtime key with `chat` can
  resolve/cancel tool approvals and MCP OAuth resolutions.
- [x] Add `chat` only to tool-approval resolution and MCP OAuth
  resolution/cancellation routes.
- [x] Ensure tenant-key list responses omit reusable API-key secret material.
- [x] Fail closed when tenant runtime keys or signed-principal secrets cannot be encrypted.
- [x] Propagate signed-principal secret decryption failures instead of treating them as unconfigured.
- [x] Restrict decrypted tenant API-key material to the one-time create response.
- [x] Correct route authorization comments and test the real authentication and authorization chain.
- [x] Reject missing, empty, unknown, or incomplete external-principal configuration without tenant-principal fallback.
- [x] Require an explicit principal mode on configuration writes and preserve
  the unconfigured state on reads; omitted modes and redaction sentinels are
  not interpreted as compatibility commands.
- [x] Replace capability normalization with an exact parser that rejects unknown values, aliases, and duplicates.
- [x] Reject unknown, whitespace, case-alias, and empty API-key scope values at
  creation, authentication, context, and principal-validation boundaries.
- [x] Require every production API-key creation caller to provide an explicit
  typed scope; tenant and platform callers never rely on an empty-value default.
- [x] Remove the effective `scope_type = tenant` database default through the
  PostgreSQL and SQLite migration chains and the GORM model without changing
  existing row values.
- [x] Reject malformed stored capability sets during API-key authentication.
- [x] Reject stored platform keys with full access, a tenant binding, or no
  explicit capability, and reject stored tenant keys without a tenant ID.
- [x] Move the exact capability parser into one additive domain file so the
  modified API-key domain file remains below 400 lines without touching a
  second upstream source file.
- [x] Reject plaintext reads for API-principal and tenant API-key credentials;
  accept only empty values or exact `enc:v1` envelopes.
- [x] Restore oversized upstream handlers by enforcing exact capabilities and
  response-secret removal in the service/domain boundary.
- [x] Mechanically split the tenant handler by responsibility so its two tenant
  API-key creation paths can pass the explicit tenant scope without leaving any
  modified Go source file above 400 lines.
- [x] Restore oversized auth code and enforce principal-mode invariants in one
  typed post-auth middleware registered by the router.
- [x] Reduce the oversized tenant-model diff to the smallest strict Value/Scan
  change after verifying that a Tenant hook cannot inspect the repository's
  `Model(&Tenant{}).Updates(tenant)` destination and is not an equivalent guard.
- [x] Mechanically split parser-engine, storage-engine, and sandbox
  configuration from `tenant.go` so every modified or added Go source file is
  below 400 lines without changing those moved behaviors.
- [x] Run formatting, router and middleware package tests, the focused handler
  secret-omission test, source-size checks, and `ai-code-check`.

## Affected Areas

- Tenant API-key control-plane routes.
- Tenant API-key persistence and create/non-create response projection.
- Signed external-principal secret persistence and restoration.
- Interactive tool-approval and MCP OAuth resolution routes.
- Router authentication and authorization tests.
- PostgreSQL and SQLite tenant API-key schema migrations and the GORM model.

The latest repository migrations remove the `scope_type` default while leaving
the immutable historical migrations unchanged. The down migrations restore the
old default only when an operator explicitly rolls back. No token format or
tenant-creation response is changed.
Non-create API-key responses intentionally stop returning reusable secrets, so
clients must capture the one-time create response. There is no route alias,
capability fallback, or runtime protocol downgrade.

## Verification Approach

- Exercise platform read/manage capability success and missing-capability
  denial through the public HTTP router.
- Exercise a tenant full-access key denial on every tenant API-key management
  method.
- Exercise `chat` capability success and missing-capability denial on each
  interactive route.
- Exercise missing `SYSTEM_AES_KEY`, encrypted round trips, strict decrypt
  failure, plaintext rejection, and database rollback.
- Exercise create and update handlers to prove only create returns the
  one-time token.
- Exercise `X-Tenant-ID` through authentication, route capability gate,
  path-tenant matching, role short-circuit, and the final handler.
- Exercise tenant runtime-key authentication with missing principal config and
  require an HTTP 401 before the handler can run.
- Exercise exact capability parsing at the handler, service, and authentication
  boundaries, including unknown, whitespace, case, and duplicate values.
- Exercise empty-scope rejection in the service and audit all three production
  creation paths for an explicit typed tenant or platform scope.
- Exercise PostgreSQL and SQLite migration definitions to prove the column has
  no default while existing tenant and platform rows keep their scope values.
- Exercise restored scope invariants before an API key can attach a principal.
- Run the complete router package test suite after focused tests pass.

## Progress

Remote tenant exactly-once provisioning is now the remaining control-plane
boundary. The local Platform claim/CAS prevents ordinary duplicate attempts but
cannot prove what happened when WeKnora commits and the HTTP response is lost.
The strict external-tenant PUT protocol above closes that gap at the owning
database. It is additive, uses one typed UUID identity, and leaves upstream
interactive tenant creation untouched.

Remote runtime-key exactly-once provisioning is complete. The platform-only
credential PUT accepts two canonical UUID path identities and one exact typed
name/capability body. The repository uses the globally unique credential
reference as the database arbitration point, encrypts the reusable token before
commit, and returns that same key ID and token on every replay. Protocol drift,
deleted or missing tenants, cross-tenant reference reuse, unknown request fields,
non-canonical UUIDs, and broader stored scope all fail closed. Generic API-key
POST, name lookup, revoke compensation, and fallback paths are not part of the
Platform caller chain.

The route policy, secret persistence, response boundary, and full middleware
chain verification are complete. External-principal resolution now accepts a
tenant principal only through the explicit `tenant` mode; all implicit or
incomplete configurations fail closed. The configuration API also requires
that mode explicitly and uses field presence, not a magic redaction value, to
decide whether a signing secret changes. Capability values now pass through one
exact enum parser at request, service, and authentication boundaries. Unknown,
case/whitespace aliases, and duplicates return errors without rewriting data.
The parser lives in an additive domain file, keeping `tenant_api_key.go` below
400 lines. API-principal and tenant
API-key credentials now accept only empty values or exact `enc:v1` envelopes;
plaintext rows fail explicitly. The oversized system handler and auth code are
restored to HEAD; the tenant handler is mechanically split by responsibility.
Exact capability validation, response-secret removal,
and strict principal-mode enforcement now live in smaller service, domain, and
post-auth middleware files. API-key scope values also use an exact typed parser;
empty, unknown, or alias values can no longer normalize into tenant or platform
access. All three production creation paths now send an explicit typed scope.
The tenant configuration model is mechanically split by parser, storage, and
sandbox responsibility so no changed Go source file exceeds 400 lines. Stored
key validation also enforces the scope invariants created by the service:
platform keys are capability-only, never tenant-bound or full-access, while
tenant keys must carry one non-zero tenant ID.
PostgreSQL migration `000091` now drops the column default directly. SQLite
migration `000013` uses the required table-rebuild operation, copies every
column and row, recreates all three explicit indexes, and preserves the unique,
foreign-key, and scope constraints. The GORM model also has no default tag, so
test and development schemas reject omitted scopes instead of creating tenant
keys implicitly.

## Final Outcome

Completed. Focused encryption, persistence, response, and route-chain tests pass.
`go test` passes for `internal/types`, `internal/application/repository`,
`internal/middleware`, `internal/router`, and all handler tests except one
pre-existing parser URL-validation failure reproduced on the clean base commit.
Focused utility-envelope tests pass; the two full utility-suite SSRF failures
caused by this host resolving `example.com` to `198.18.0.194` also reproduce on
the clean base commit.
The application-service suite passes when excluding one pre-existing MCP SSRF
test whose environment resolves `example.com` into a restricted test range;
that same failure was reproduced on the clean base commit. `go vet` passes for
all affected packages. The oversized system handler and auth middleware are
byte-for-byte restored to HEAD; the tenant handler is split into
responsibility-specific files without changing its existing behavior.
`internal/types/tenant.go` retains only the strict
APIPrincipalConfig Value/Scan difference because moving it to a Tenant hook
would not cover the repository update shape or direct driver.Valuer calls;
unrelated parser, storage, and sandbox declarations were mechanically moved
into responsibility-specific files. Tenant keys remain denied on tenant-key
and principal-config administration; platform key update and principal
test-token routes remain undeclared. Empty API-key scopes and empty principal
configuration modes are rejected instead of defaulted. Every new or modified Go source file stays
below 400 lines. The Lite server build passes with the repository's
`sqlite_fts5` tag after using the missing `libduckdb_static.a` directly from the
already-downloaded official Go module archive; the extracted module-cache
directory itself is incomplete on this host. A wider `go test ./internal/...`
still reports unrelated upstream/environment failures in packages outside this
patch, so only the passing focused and affected-package suites are completion
evidence.

The SQLite v12-to-v13 migration test proves tenant and platform rows, stored
fields, primary-key sequence, indexes, uniqueness, foreign key, and scope
constraints survive while omitted-scope inserts fail. The PostgreSQL 16
migration was also executed against a temporary local cluster: the up migration
removed the default and preserved both scope rows, and the down migration
restored the old default. Full database, type, repository, middleware, and
router package tests pass; focused service, handler, and secret-envelope tests
pass. `go vet` passes for all affected packages, `git diff --check` is clean,
and the final arm64 Lite binary is
`/tmp/weknora-security-review-lite` with SHA-256
`a726b1bb58dee5d10c6eebd494c1bd63b2468e787aa30dfc0a757d35f8521ed8`.

The runtime credential extension is also complete. One race-enabled HTTP test
drives eight concurrent requests through handler, service, repository, SQLite,
encryption hooks, and response serialization; exactly one response is `201`,
all others are `200`, and every response contains the same key ID and token.
The test also proves one encrypted database row and an explicit empty
`knowledge_base_ids` array. Focused tests pass across types, repository,
service, handler, router, and database; full type, repository, router, and
database packages pass; `go vet` passes for all six affected packages. SQLite
migrations run through version 15. PostgreSQL migration `000093` was executed
against a temporary real database: up created the UUID unique constraint,
duplicate insertion failed, and down removed both constraint and column.

The final `ai-code-check` found and fixed two protocol defects. The create path
previously left `knowledge_base_ids` nil, which could serialize as `null` and
violate the Platform exact response schema; the service now constructs an
explicit empty array and the real concurrent HTTP test protects that contract.
Replay validation now also proves the decrypted token hashes to the persisted
`key_hash`, so corrupted or mismatched stored credentials fail closed instead
of returning a token that cannot authenticate. No material finding remains in
the runtime-key path. The full
handler package still has the previously documented redacted-parser expectation
failure, and the full application-service package still has the documented MCP
SSRF fixture failure because this host resolves `example.com` into
`198.18.0.0/15`; neither failure reaches the runtime-key call chain.
