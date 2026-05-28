# VideoNode docs site

VitePress source for the docs published at <https://mazurov.dev/videonode/>.

## Local development

```bash
pnpm install
pnpm dev    # http://localhost:5174/videonode/   (UI uses 5173)
```

`pnpm build` produces a static site under `docs/.vitepress/dist/`. The same command runs in CI.

This project is wired into the repo-root `process-compose.yaml` as the `docs` process — `process-compose up docs` starts only the docs site.

## Authoring

Pages live under `docs/`, organized into four sections that match the Diátaxis modes:

- `getting-started/` — tutorials (`introduction`, `installation`, `quickstart`)
- `reference/` — pure reference lookup (`config-toml`, `pipeline-model`, `rest-api`)
- `operating/` — how-to guides (`streaming-outputs`, `encoders`, `observability`)
- `development/` — contributor explanations (`architecture`, `building`, `streams-toml`)

The sidebar is defined explicitly in `docs/.vitepress/config.ts` — when adding a page, update the sidebar there too. Search is the built-in VitePress local provider (no external service).

Mermaid diagrams: standard ` ```mermaid ` fenced code blocks render via `vitepress-plugin-mermaid`.

## Deployment

Two workflows publish to the `gh-pages` branch via `JamesIves/github-pages-deploy-action@v4`:

- **`.github/workflows/docs.yml`** — runs on push to `main` or `native` when `website/**` changes. Deploys `docs/.vitepress/dist` to the branch root.
- **`.github/workflows/release.yml`** (`publish-apt` job) — runs on `v*` tags. Deploys the APT repo (`dists/`, `pool/`, `public.key`) to the same branch.

Both jobs share the `pages-deploy` concurrency group so they serialize. Each uses `clean-exclude` to protect the other's files. **Never add `force: true` or `force_orphan: true`** — both silently bypass file-preservation and orphan-rebuild the branch.

PRs touching `website/**` are typecheck-built by `.github/workflows/ci-docs.yml` (no deploy).
