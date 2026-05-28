# Agent guide — VideoNode docs

This is the **VitePress** docs site for VideoNode, published at <https://mazurov.dev/videonode/> via the `gh-pages` branch. Content lives under `docs/`, config at `docs/.vitepress/config.ts`, build output at `docs/.vitepress/dist/`.

If you're an LLM agent writing or editing pages here, follow the rules below verbatim. Skipping them produces docs that get reverted.

## Quick commands

```bash
pnpm dev      # localhost:5174/videonode/  — hot reload
pnpm build    # produces docs/.vitepress/dist/
pnpm preview  # serves the built site
```

UI dev server runs on `:5173`; VitePress is pinned to `:5174` to avoid collision.

## Diátaxis mode per page — never mix

Every page lives in one of four modes. Stay in mode for the entire page.

| Mode | Folder | Voice | Cardinal sin |
|---|---|---|---|
| **Tutorial** | `getting-started/` | "We'll…", "you'll see…" — beginner-friendly | Listing options; assuming knowledge |
| **How-to** | `operating/` | Imperative, terse | Teaching theory; multiple approaches |
| **Reference** | `reference/` | Structured, table-first, no narrative | Telling a story; use cases |
| **Explanation** | `development/` (mostly) | Discursive, "because…", context | Step-by-step procedures |

`reference/pipeline-model.md` is the one cross-folder exception — explanation living in `reference/` because that's where readers expect it. Otherwise: folder ⇒ mode.

**Apply the compass at paragraph level, not just page level.** A reference page with a step-by-step procedure embedded mid-page is mixing modes inside the page — extract the procedure to a how-to and link. ([Diátaxis compass](https://diataxis.fr/compass/))

## Core rules (every page)

1. **Every page is page one.** Assume the reader landed here from a Google search. Open with one sentence stating what this page is and isn't. Link to prerequisites; don't recap them.
2. **Inverted pyramid.** Most important fact in the first 50 words. Caveats and edge cases below.
3. **Show, then tell.** Reference pages open with the schema/code/command. Prose follows the artifact, never precedes it.
4. **State the recommendation explicitly, then show alternatives.** Stripe pattern: open with "Use X for most cases" before presenting options. Less-good docs bury the recommendation in a note after listing three equally-weighted alternatives. Make the decision the reader can't. ([Stripe quickstart pattern](https://docs.stripe.com/payments/quickstart))
5. **Be specific, not generic.** Never write "various options", "many features", "flexible configuration". Name the option, count the features, list the configurations. If you can't, leave `:::warning TBD` citing the file you couldn't resolve and move on.
6. **Cut filler.** Delete "in order to", "it should be noted that", "simply", "just", "easily", "powerful", "robust", "seamless", "leverage", "comprehensive". Goal: ≤300 words of *prose* per page. Code/tables/lists don't count.
7. **Concrete over abstract.** File paths, function names, exact command strings, real port numbers from the codebase. Never invent values to sound authoritative; mark TBD instead.
8. **No marketing voice, no emoji** unless quoting source. Skip "welcome!", "let's dive in", "in this guide we'll explore". Start.
9. **Link laterally.** Relative paths to sibling pages (`../reference/config-toml`, no `.md` extension thanks to `cleanUrls: true`). Don't repeat content; link to it.
10. **Code blocks need language tags.** ` ```toml`, ` ```go`, ` ```bash`. Required for syntax highlighting.

## Mode-specific rules

### Tutorials (`getting-started/`)

- **Never explain *why* mid-step — link out to an explanation page.** Inline rationale breaks the doing-flow that tutorials promise. ([Diátaxis tutorials](https://diataxis.fr/tutorials/))
- **Never offer alternative commands or paths.** Pick one and commit. Forcing the reader to choose during a tutorial breaks the contract. Alternatives belong in how-to or reference pages.
- **One product change can cascade through the entire narrative.** Run the tutorial end-to-end after any non-trivial code change to the touched component. A broken mid-step destroys all reader confidence, and the damage is invisible until someone tries it.

### How-to guides (`operating/`)

- **Title as a gerund phrase.** "Configuring TLS on RTSP" beats "TLS Configuration" — signals actionable content to scanners and search. ([Diátaxis how-to](https://diataxis.fr/how-to-guides/))
- **State the goal *before* the action in every procedure step.** "To deploy, run `kubectl apply -f deploy.yaml`" — not "Run `kubectl apply -f deploy.yaml` to deploy." Readers scanning steps need to know if a step is relevant before committing to read it. ([Google procedures](https://developers.google.com/style/procedures))
- **Practical completeness beats exhaustive completeness — document one path, the shortest viable one.** Covering every variation turns a how-to into a reference page, diluting both. ([Diátaxis how-to](https://diataxis.fr/how-to-guides/))

### Reference (`reference/`)

- **Open with the artifact (schema, table, signature). Prose is annotation, not preamble.**
- **Link to the raw machine-readable source if one exists.** REST API page links to `/openapi.json`. Don't make readers regenerate it from prose. ([OpenAPI best practices](https://learn.openapis.org/best-practices.html))
- **Show output only when non-obvious or verifiable.** Showing output for every snippet trains readers to ignore it; omit it for predictable cases. Use it when timing or async behavior matters.

### Explanation (`development/`)

- **Explanation is not a luxury appended after how-tos.** Without explanation, practitioners develop "loose and fragmented and fragile knowledge." Justify design decisions and trade-offs — that's the whole point. ([Diátaxis explanation](https://diataxis.fr/explanation/))
- **Lead with the *because*, not the *what*.** The code already shows the what; the doc adds the why.

## Style — Google developer style guide

- **Second person.** "You configure…" not "We configure…" or "One configures…".
- **Active voice as default; passive only for special cases.** Use passive when the actor is irrelevant ("The database was purged in January") or you're de-emphasizing blame ("Over 50 conflicts were found"). All other cases: doer = subject. Passive-as-default hides who does what — the primary failure mode in API docs. ([Google voice](https://developers.google.com/style/voice))
- **Sentence case for all headings.** `# Pipeline model`, not `# Pipeline Model`. Applies to `##` and `###` too.
- **Descriptive link text.** Never `click here`, `this document`, `this article`, or a bare URL. Link text must make sense out of context — screen readers and search crawlers read it in isolation. ([Google link text](https://developers.google.com/style/link-text))
- **Numbered lists for sequences only.** Step-by-step procedures get `1.`/`2.`/`3.`. Everything else uses bullets.
- **Introduce every list with a complete sentence.** Not a fragment that the list grammatically completes. Fragment intros break screen reader flow and make the list non-scannable. ([Google lists](https://developers.google.com/style/lists))
- **Serial comma:** "sources, composers, and streams".
- **`code font` for identifiers.** `config.toml`, `--port`, `VideoSink::start()`. Bare text for UI labels and concepts.
- **Don't inflect code as English words.** Never `` `ADDRESS`'s value ``; write `the ADDRESS constant's value`. Possessives on inline code force screen readers to treat `'s` as part of the identifier. ([Google code in text](https://developers.google.com/style/code-in-text))
- **No pre-announcements.** Don't document features that don't exist yet. Leave them out — not a `:::note coming soon`.

## Code samples

- **Mark omitted code with a language-appropriate comment, never `...` or `…`.** ` # ... ` in Bash, `// ...` in Go, `# ...` in TOML. Ellipsis-truncated samples can't be copy-pasted; either keep them runnable or explicitly comment what's elided. ([Google code samples](https://developers.google.com/style/code-samples))
- **Real values, not `foo`/`bar`.** Use plausible IDs (`source_id = "cam-lobby"`), realistic paths (`/dev/video0`), real port numbers from the codebase.
- **Length: one screen max.** If a sample needs scrolling, split or excerpt and link to the full file in the repo.

## Mermaid diagrams

`vitepress-plugin-mermaid` is wired in. Fenced ` ```mermaid` blocks render in any `.md` file — no JSX, no imports.

### When to use a Mermaid diagram

Use one when **a diagram makes the explanation faster than prose**:
- **Pipeline / data flow** — `flowchart LR`.
- **Sequence / RPC** — `sequenceDiagram`.
- **State machine** — `stateDiagram-v2`.
- **Decision tree** — `flowchart TD`.

**Don't** use a diagram for: decoration; type hierarchies (use a table); lists of features (use bullets); anything with >15 nodes (readers can't scan, SVG bloats the page).

### Mermaid rules

1. **Quote labels with special characters.** `A["videonode-source #1"]` — unquoted `#`, `(`, `:` break the parser silently.
2. **Add accessible descriptions** via `accTitle` and `accDescr` directives. Mermaid renders as inline SVG with no synthesized alt text:
   ```mermaid
   flowchart LR
     accTitle: VideoNode pipeline overview
     accDescr {
       V4L2 device feeds videonode-source, which broadcasts NV12 frames
       to the composer and sinks via SCM_RIGHTS.
     }
     V4L2[V4L2 device] --> src[videonode-source]
     src --> comp[videonode-composer]
   ```
3. **Diagram drift defense — cite the source as an HTML comment.** Above every diagram, add:
   ```markdown
   <!-- depicts: internal/streams/pipelinectl/manager.go:Manager.spawn (rev as of <commit-sha>) -->
   ```
   No automated Mermaid-to-code test tooling exists. The comment tells reviewers which file to re-read when checking for drift. This is the only reliable defense.
4. **One diagram per concept.** Two diagrams explaining the same thing from different angles means you're explaining the wrong thing.
5. **Keep node labels short** (≤30 chars). Detail goes in a numbered list below the diagram, referencing node IDs.
6. **Don't style manually.** No `classDef`, no inline color overrides. The plugin handles light/dark theme auto-switching — manual colors break dark mode.
7. **No `subgraph` unless the grouping is the point** (e.g., "everything inside this box is one process").
8. **Heavy diagrams: pre-render to SVG.** >15 nodes or slow in dev → run through the [Mermaid Live Editor](https://mermaid.live), export SVG, drop into `docs/public/diagrams/`, reference as `![alt](/videonode/diagrams/foo.svg)`. Runtime stays slim; the diagram becomes uneditable in markdown. Accept the tradeoff for big ones.

## File conventions

### Frontmatter

- **Home (`docs/index.md`):** `layout: home` block with `hero` and `features`. No `# Heading`.
- **Doc pages:** first `# H1` becomes the page title. Frontmatter only when needed (e.g., `outline: deep`).

### Sidebar

The sidebar is **explicit** in `docs/.vitepress/config.ts`. **When you add a new page, update the sidebar in the same edit.** A page that exists but isn't in the sidebar is invisible to navigation.

### Links

- Path-style without extension: `[the API](../reference/rest-api)`, not `(...rest-api.md)`.
- The build fails on dead links — CI catches them at PR time.

### Code groups (multi-tab snippets)

For "do it via TOML / REST / CLI" cases:

````markdown
::: code-group
```bash [REST]
curl -X POST localhost:8090/api/sources -d '{"id":"cam1"}'
```
```toml [TOML]
[[sources]]
id = "cam1"
```
:::
````

### Custom containers (admonitions)

```markdown
::: tip
::: info
::: warning
::: danger
::: details
```

One type per page max. Misuse dilutes attention.

## What NOT to do

- **No FAQs.** Every FAQ item is either a how-to (task), an explanation (concept), or a reference entry. Route it there. FAQ pages "accumulate disparate content on unrelated topics" and rot fast. ([Write the Docs](https://www.writethedocs.org/guide/writing/beginners-guide-to-docs/))
- **Don't introduce a new top-level section folder** (`troubleshooting/`, `recipes/`, etc.) without updating both the sidebar in `config.ts` AND `clean-exclude` in `.github/workflows/release.yml`. Otherwise the next release-tag apt-deploy will delete the folder.
- **Don't write `:::note coming soon` or `TODO: write this later`.** Leave the page out entirely.
- **Don't link to GitHub source with line numbers in rendered prose** — they rot. Link to file paths; let the reader navigate.
- **Don't migrate content from `CLAUDE.md` or `AGENTS.md` verbatim.** Those speak to coding agents; this site speaks to humans. Translate.
- **Don't add tracking, analytics, or comment systems.**
