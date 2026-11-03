---
name: yj-arch-core
description: >-
  Repository architecture constitution for this monorepo. Use on every coding,
  review, or refactor task. Enforces fixed platform roots, the env field
  contract, universal entry/flow/domain/view/infra roles (domain = owned
  concept, persistence optional), acyclic imports, and minimal edit scope.
  Always apply together with exactly one matching yj-* platform skill for the
  folder being edited.
---

# yj-arch-core

## Purpose

Reduce AI cost and blast radius. Do not reinterpret the whole repo, add
wrappers/layers, or read unrelated platforms.

1. Existing repo conventions win when clearer than this skill.
2. Identify the platform root folder first, then load **only** that platform skill.
3. Edit the smallest file set that owns the change.

## Platform roots

```text
backend-msa/          # yj-backend-msa
frontend/             # yj-frontend
```

```text
{platform}/
  {service_name}/     # one deployable / one app — code only
```

- Platform and service folders hold **code only**: source folders, stack files
  (e.g. `package.json`) and the service's `.env.examples`. Run/build/deploy
  tooling lives outside them (see Tooling below).
- `{platform}/proto/` is reserved for protobuf contracts; it is never a service.
- Do not invent new platform roots or put app code at repo root.
- Do not create a `Makefile`, `scripts/` folder or run/build script inside any
  platform or service folder.

## Environment contract

- Every service root has `.env.examples` — the committed field contract. Format
  every entry as one comment line followed by one empty `FIELD=` line, with one
  blank line between entries.
- Field sets come from the platform's env kind:
  - `listen`: `backend-msa` (PORT 8080), `frontend` (PORT 5173)
- `HOST`/`PORT` are required for `listen`. For `worker` they are optional — add
  them only when the process really listens. `app` services need NAME/VERSION only.
- Do not invent fields outside the contract; add a field to `.env.examples`
  first, then wherever its values are kept.
- Never commit a plain `.env`, and do not use Vite's `.env.local` convention
  unless a thin loader maps it to this structure.

## Tooling

### Runner (`tools/runner/`)

- Root `make <platform>` starts every service of that platform concurrently;
  `make <platform> NAME=<service>` starts one. Both call
  `tools/runner/run.sh <platform> [service]` (`run.bat` / `run.ps1` on Windows).
- `tools/runner/` and the root `Makefile` / `make.bat` are managed by yjcli and
  regenerated — do not edit them.
- Each service declares its repository-owned local command as `RUN_COMMAND` in
  `tools/secrets/<platform>/<service>/.env.local-dev`. The runner exports that
  file and executes the declaration; it never infers commands from
  `package.json`, `pyproject.toml`, `go.mod`, or other manifests.
- Local runs always use `.env.local-dev` — do not hardcode HOST/PORT in scripts.

### Secrets (`tools/secrets/`)

- Env values live only in `tools/secrets/<platform>/<service>/.env.<environment>`.
  Services keep `.env.examples` only — never create `.env*` value files inside
  a service folder.
- `.env.local-dev` is required. New services also start with `.env.development`
  and `.env.production`; remove unused ones or add others (`.env.staging`,
  `.env.test`, …) following the `.env.<environment>` naming pattern.
- These files are **committed in plain text** — the repository must stay
  private. Do not copy their values into code, docs, logs or commit messages.
- Value files list populated `FIELD=value` lines consecutively, without
  field-description comments or separator blank lines.
- When adding a field, add it to the service's `.env.examples` and to every
  `.env.<environment>` file of that service.

### Builder (`tools/builder/`)

- Builds: `make <platform>-build-development|production [NAME=<service>]` →
  `tools/builder/<platform>/build-<environment>.sh`. Deploys:
  `make <platform>-deploy-development|production [NAME=<service>]` →
  `tools/builder/<platform>/deploy-<environment>.sh`.
- `tools/builder/build-common.sh`, `deploy-common.sh` and `steps/*.sh` are
  managed by yjcli and regenerated. Put build/package logic in
  `tools/builder/<platform>/build-<environment>.sh` and upload/rollout in
  `deploy-<environment>.sh` — those are repository-owned and fail with an
  implementation prompt until implemented.
- `build-common.sh` runs every `steps/*.sh`. Each step self-detects by
  folder/file presence, never by platform name, so any platform can opt in:
  - `steps/proto.sh` → protoc codegen when `<platform>/proto/` exists
    (`proto/*.proto` → `proto/dist/{lang}`). protoc is the standard; buf is not used.
  - `steps/ssr.sh` → template/asset build when `<platform>/<service>/views/` exists.
  - `steps/docker.sh` → docker image **build only** (no push, no swarm/stack
    rollout) for each service with a `Dockerfile`, using
    `tools/secrets/<platform>/<service>/.env.<environment>`. Image version: the
    service's `package.json`/`pyproject.toml` `version`, else the `VERSION` env var.
- Deploy-side steps are `steps/deploy-*.sh` (none ship by default — e.g. image
  push + `docker stack deploy`, `kubectl apply`); `deploy-common.sh` runs them
  after the build.
- Deployable services get a `Dockerfile` + `.dockerignore` stub with no `FROM`
  yet — `docker build` fails until it is replaced. Delete it to opt a service out.
- `build-*` / `deploy-*` are `.sh` only — build/deploy tooling is not native to
  Windows, so `make.bat` runs those targets through WSL.

## Skill routing

| Path | Platform skill |
|------|------|
| `backend-msa/**` | `yj-backend-msa` |
| `frontend/**` | `yj-frontend` |

Never load all platform skills. Never apply a platform skill outside its folder.

## Universal roles

```text
entry   = executable entrypoint + route/command/screen registration + wiring
flow    = feature composition + dto / input-output shape
domain  = owned business concept (authority); persistence optional
view    = UI (only platforms that render UI)
infra   = shared technical modules (config, clients, logging, utils)
```

### Domain (owned concept — not “must have DB”)

Create `domains/{name}/` only when **this deployable owns** a concept (it is the authority for that idea).

Optional file slots inside a domain (use only what exists):

- `model` — data shape / entity (optional)
- `repository` — persistence/retrieval (optional; only if this process stores it)
- `rules` — decisions / pure policy (optional)
- `errors` / `types` — domain-only errors and value types (optional)

Valid shapes: data+rules · data only · rules only.  
If the deployable owns **no** concept → **omit `domains/` entirely**.

Missing domain does **not** mean collapse into entry. Still use flow + infra.

Placement when unsure:

1. Owned concept / shared policy across features? → domain  
2. One feature’s IO / mapping / orchestration? → the platform-defined flow
   boundary (commonly `services/{feature}`)
3. Clients, config, logging, technical helpers? → infra (`modules`)  
4. Bootstrap / register only? → entry (`apps` or platform entry files)  
5. Otherwise do not invent a new home — extend an existing flow

UI clients default to **no `domain`**: model = API types; feature rules live in flow.  
Exception only for heavy client-owned offline DB/engine concepts.

UI platform skills may map flow and view to separate technical roots. Those
roots must use the same feature boundary so one feature's flow and view remain
easy to locate without mixing their roles.

## Import direction

Allowed: `entry -> flow -> domain -> infra`, `entry -> infra`, `flow -> infra`, `view -> flow`, `domain -> infra`.

Forbidden: `domain -> flow/entry/other domain`, `flow -> entry/other flow` (unless repo allows), `infra -> *`, `view -> domain/infra/network`, generated UI → project-specific imports.

Entry must stay thin: no feature DTOs, business policy, or grab-bag utils in entry packages.

Cycles are structure bugs — do not paper over with lazy imports or wrappers.

## Editing workflow

Before: platform skill → feature/domain → minimal file set → prefer reuse.

During: edit in place; no thin wrappers/adapters; no one-line functions unless the name is a real domain concept; keep feature work and drive-by refactors separate.

Avoid names: `{entity}_v2|_wrapper|_adapter|_helper`, `{feature}_manager|_processor`.

Avoid layers unless user requests: `usecases/`, `workflows/`, `orchestrations/`, `application/`, `controllers/`, `handlers/`.

`flow` is the composition layer — do not invent a layer above it.

After: summarize files, boundaries, new files, test/migration risk briefly.

## Cross-platform contracts

- Prefer generated clients / `{platform}/proto/dist/{lang}` (e.g.
  `backend-msa/proto/dist/{lang}`) over hand-copied DTOs.
- Do not import another platform's source tree.
- Do not read unrelated platforms unless the user task explicitly spans them.
