---
name: yj-frontend
description: >-
  Single frontend app architecture. Required stack: Vite + React + TypeScript +
  react-router. Feature-level flow/view separation; no domain folder by
  default. No plain .env. Use only when editing `frontend/**`.
---

# yj-frontend

Requires `yj-arch-core`. Scope: `frontend/**`.

## Required stack

**Vite + React + TypeScript + react-router** (mandatory).  
Do not scaffold CRA, Next.js, Webpack-only, or non-Vite bundlers unless the user explicitly overrides.

## Shape

```text
frontend/
  {app_name}/
    src/
      main.tsx, App.tsx          # entry
      stores/{feature}.ts        # flow state/orchestration
      api/{feature}/             # flow API access/mapping
      pages/{feature}/           # container view
      components/{feature}/      # presentational view
      lib/, hooks/               # infra
```

One `{app_name}` = one web app. No micro-frontends unless the user requests it.

## Roles

```text
entry = src/main.tsx, src/App.tsx (routes)
flow  = stores/{feature}.ts + api/{feature}
view  = pages/{feature} + components/{feature} (+ generated ui primitives)
infra = lib/*, hooks/*
```

Flow and view use the same `{feature}` boundary under separate role roots. Do not place a feature store or API module beside `pages/` or `components/` as if it were a view.

**No `src/domains/` by default.** Model = API types; feature rules live in store/container (flow).  
Exception: heavy **client-owned** offline DB/engine concepts only (then optional domain slots per core).

Missing domain ≠ dump DTOs/utils into `main.tsx` / `App.tsx`. Keep feature work under stores/api.

## Rules

- Follow `yj-arch-core` env names; listen apps need `HOST`/`PORT`. No plain `.env`.
- `npm start` / local mode must mean local-dev — do not silently point bare start at development/production deploy envs.
- Store calls the same feature's `api`; pages/components do not call network directly.
- Keep one store file per feature directly under `stores/`; do not add a redundant `{feature}/` directory for a single store file.
- Container route wires store + router; presentational page takes props only.
- Routes declared at entry — do not scatter route tables; entry stays registration-thin.
- Generated UI primitives: do not hand-edit or duplicate.
- Consume backend contracts via a generated client or the shared root contract (see "Shared proto contract") — never copy `.proto` into frontend.

## Optional: server templating (SSR)

Templating is an **option**, not this app's default. Most apps stay a client-rendered SPA — enable only when this app must render HTML server-side (e.g. a lightweight SSR/SSG entry alongside the Vite build).

When enabled, add sibling to `src/`:

```text
views/{page}.html | views/layouts/* | views/partials/*
```

- Template/asset bundling is wired into the repository's build tooling, which detects `{app_name}/views/` (see `yj-arch-core` › Tooling). Implement it there, not as an ad-hoc one-off command.
- Keep `src/` (client bundle) and `views/` (server-rendered templates) separate; do not fold template markup into `pages/`/`components/`.
- Do **not** create `views/` speculatively — it does not apply to a normal client-only SPA.

## Import direction

```text
entry -> view
view (container) -> flow (store)
store -> api -> infra
view (presentational) -> props only
```

## Editing scope

- Stay inside `frontend/{app_name}/` (and linked contract dist if needed).
- Do not load Electron/Flutter skills for React web work.

## Shared proto contract

- The gRPC/protobuf contract lives **only** at the repository root: schemas in `proto/*.proto`, generated libraries in `proto/dist/<lang>/` (skill `yj-proto`). It belongs to no platform.
- Before adding or calling a gRPC API, look for the root `proto/` and work there. Never create a `proto/` folder or a `.proto` file inside a platform or service folder, and never copy `.proto` files or generated code into one.
- If the repository has no root `proto/` (or not the language you need under `proto/dist/`), stop and ask the user to add it with yjcli (`yjcli proto add <language>`) instead of creating it by hand.
- Consume the generated library through a path dependency on `proto/dist/<lang>/`: Go `require` + `replace`, Dart `path:` dependency, Python editable path dependency. Do not edit generated code — change the `.proto` and regenerate (skill `yj-proto` › Generated libraries).
