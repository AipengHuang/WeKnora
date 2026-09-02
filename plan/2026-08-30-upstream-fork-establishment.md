# Upstream Fork Establishment

## Objective

Replace the snapshot-based repository lineage with a complete fork of
`Tencent/WeKnora` under the active GitHub account while preserving every local
file and recovery path before rebuilding the local checkout.

## Repository Boundary

- The fork remains a complete WeKnora repository with official history and
  directory structure.
- Platform product code does not import WeKnora source through a symlink,
  subtree, submodule, or sibling source path.
- Platform consumes a tested WeKnora release through HTTPS/SSE APIs and pinned
  immutable deployment artifacts.
- The official `frontend/` remains present and buildable for upstream parity,
  but it is not modified into the Adax product UI.
- The standalone Adax browser UI lives in `platform/apps/web` and calls WeKnora
  only through Platform Gateway and Service.
- The personal fork is public because the official repository is public. No
  proprietary Adax patch is pushed until its visibility is explicitly approved.

## Implementation Steps

- [x] Inspect the previous branch, commit, remotes, and working tree.
- [x] Confirm the active GitHub account.
- [x] Create the personal GitHub fork from `Tencent/WeKnora`.
- [x] Verify the fork parent, default branch, and official ancestry.
- [x] Record refs, status, remotes, commit, file manifests, and a complete Git
  bundle for the previous snapshot repository.
- [x] Preserve ignored local files without printing secret contents.
- [x] Rebuild the local checkout from the verified fork.
- [x] Configure `origin` as the personal fork and fetch-only `upstream` as
  `Tencent/WeKnora`.
- [x] Allow only platform keys with `system_tenants_read` or
  `system_tenants_manage` to read or configure tenant API principals, so the
  Platform provisioner can require signed external-user isolation.
- [x] Add the minimal tenant runtime-key administration and interactive
  `chat` capability patch defined in
  `plan/2026-08-30-external-principal-control-plane.md`.
- [x] Reapply only approved, minimal Adax backend patches with focused tests.
- [ ] Verify the official frontend without adding Adax product UI changes.
- [ ] Establish the release-tag sync and immutable-image release workflow.

## Affected Areas

- The personal GitHub account and its WeKnora fork.
- The recovery archive for the previous local snapshot and Platform copy.
- The clean `repos/weknora` checkout.
- WeKnora build, deployment, migrations, API, DocReader, MCP server, and
  official frontend verification.

## Verification Approach

- Query GitHub repository metadata and require `parent.full_name` to equal
  `Tencent/WeKnora`.
- Require the official default branch commit to equal the fork default branch
  before any local Adax change.
- Run `git fsck --connectivity-only` on the rebuilt checkout.
- Verify `origin` and `upstream` URLs and fetch behavior.
- Compare every final Adax backend patch with the selected official release.
- Build and test the backend, DocReader, MCP server, migrations, and official
  frontend before production cutover.
- Verify Platform browser, desktop, and native bundles do not import fork source
  files.
- Verify a tenant full-access key still cannot reach API-principal
  configuration, while a platform key needs the exact read or manage
  capability.
- Verify production images are selected by exact digest and never by `latest`
  or a floating branch.

## Upstream Sync Procedure

1. Fetch `upstream` branches and tags without changing the current release.
2. Select and verify one official release tag and commit.
3. Review migrations, configuration, OpenAPI, DocReader, MCP, storage, and
   deployment changes.
4. Create `sync/upstream-vX.Y.Z` from the current Adax release.
5. Merge the selected official tag; never merge unrelated histories.
6. Resolve conflicts in favor of upstream behavior and reapply only approved
   minimal patches.
7. Run the complete fork verification suite.
8. Build immutable images and record their digests.
9. Restore a production backup in staging and verify migrations, knowledge,
   documents, sessions, SSE, MCP, and files.
10. Tag the validated Adax release and deploy only the recorded digests.

There is no runtime protocol downgrade, previous-endpoint retry, or automatic
deployment from upstream `main`.

## Progress

The verified public fork is `AipengHuang/WeKnora`. GitHub reports
`Tencent/WeKnora` as its parent and source. The clean local checkout, fork
`origin/main`, and `upstream/main` all resolve to
`4d78f9dc120a552775d5dd0dec3acd5a2fc0661f` before this plan is added.

The previous repository, the full Platform copy, and a verified complete Git
bundle are preserved under
`backups/weknora-before-fork-2026-08-30.lct4uV`. The `upstream` remote has no
usable push URL, so sync is fetch-only by construction.

The first approved backend patch exposes tenant API-principal configuration
only to platform keys with the existing `system_tenants_read` or
`system_tenants_manage` capabilities. Tenant keys remain denied. The focused
test failed before route registration, then the complete router and middleware
suites passed. The focused handler projection test also proves list responses
omit reusable API-key secret material.

## Final Outcome

Fork establishment and the first explicitly approved minimal backend patch are
complete. Official frontend parity, release-tag sync, complete release
verification, and immutable deployment evidence remain release work and are
not claimed here.
