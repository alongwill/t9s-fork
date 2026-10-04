# Follow-up plan: show the schematic YAML in the Extensions view

Same branch and rules as `docs/design/omni-readonly-plan.md` (do this after it, on the same branch).

## Facts already checked

- Talos ≥ 1.14 publishes `ImageFactorySchematics.runtime.talos.dev`, ID `image-factory-schematic`,
  spec `schematicId`, `flavor`, `apiUrl`
  (`~/sources/github.com/siderolabs/talos/pkg/machinery/resources/runtime/image_factory_schematic.go`).
- Older nodes: the extensions list contains an entry named `schematic` whose version is the
  schematic ID; assume `https://factory.talos.dev` as the factory then.
- Image Factory: `GET <apiUrl>/schematics/<id>` returns the schematic as `application/yaml`
  (`~/sources/github.com/siderolabs/image-factory/internal/frontend/http/api/schematic.go:76-94`;
  route `routes.go:113`, `AccessAuthenticated`: open on the public factory, may return 401/403 on a
  private/enterprise factory).

## Work

1. `internal/talos`: `GetSchematicInfo(node)` (resource first, extension-list fallback) and
   `FetchSchematicYAML(ctx, apiURL, id)` with `net/http`, 10 s timeout, `Accept: application/yaml`,
   validate `id` is 64 hex chars before building the URL, cache by `apiURL+id` for the session.
   This is a read (HTTP GET), so it is allowed in read-only mode.
2. Extensions view: the `schematic` row (or a header line `schematic <short id> · <factory host>`)
   gets `enter` → a YAML pane (reuse the browser's coloured YAML rendering) titled
   `Schematic <id[:12]>`, showing the YAML, the full ID, the flavor and the factory URL.
   Errors: 401/403 → `🔒 the factory needs authentication; schematic ID <id>`; network error →
   the error plus the ID and URL so the user can open it themselves.
3. In the browser's describe pane for `ImageFactorySchematic`, add a line `y on the instance shows the
   factory's YAML` and make `y` there open the same pane.
4. Tests: URL building and ID validation, `httptest` server for 200 YAML / 401 / timeout,
   fallback to the extension entry, render at 80×24.
5. README + help. Commit, then report as before.
