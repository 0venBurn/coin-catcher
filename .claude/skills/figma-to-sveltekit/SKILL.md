---
name: figma-to-sveltekit
description: Implement Figma designs as SvelteKit components without the Figma MCP. Use this skill whenever the user shares a Figma URL, paste of Figma JSON/inspect data, exported design tokens, or any Figma file key and wants it turned into working SvelteKit/Svelte component code. Also trigger when the user says "implement this design", "convert Figma to Svelte", "build this from Figma", or shares a figma.com link with implementation intent. This skill fetches structured design data directly from the Figma REST API and produces copied design styled svelte components. DO NOT IMPLEMENT FUNCTIONALITY.
---

# Figma → SvelteKit Skill

Fetches design context from the Figma REST API and implements it as production-ready SvelteKit components — no MCP required. Produces the same quality of structured design data the MCP would provide, via a direct API fetch + pruning pipeline.

---

## Step 0: Gather inputs

Before fetching, collect:

1. **Figma file key** — the alphanumeric ID in any Figma URL:
   `https://www.figma.com/file/FILE_KEY/...` or
   `https://www.figma.com/design/FILE_KEY/...`

2. **Node ID(s)** (optional but preferred) — the `?node-id=X:Y` param from a Figma share link. If the user doesn't have it, you'll fetch the full file and help them identify the right frame.

3. **Figma PAT** — use exported `FIGMA_API_KEY` from the environment. If missing, ask the user to export one.
   Guide: Figma → Account Settings → Security → Personal Access Tokens → Generate.

4. **Project context** — ask once if not already known:
   - CSS approach (Tailwind, CSS modules, plain scoped `<style>`, design token vars)?
   - TypeScript or JS?
   - Any existing component library / design system constraints?

---

## Step 1: Fetch design data

Use the helper script to fetch and prune the Figma API response to just what's needed. See `scripts/fetch-figma.ts`. Requires Bun — guaranteed available in any SvelteKit project.

```bash
bun run scripts/fetch-figma.ts \
  --file-key FILE_KEY \
  [--node-id "X:Y"] \
  --output /tmp/figma_context.json
```

The script:

- Hits `GET https://api.figma.com/v1/files/:key` (or `/nodes` if a node ID is provided)
- Prunes the node tree to only what's relevant (see pruning rules in the script)
- Extracts: layout/sizing, colors, typography, spacing, component properties, auto-layout constraints, image fills

If the file is large and no node ID was given, use the `/v1/files/:key?depth=2` param to get a shallow tree first, identify the target frame with the user, then re-fetch with the node ID.

---

## Step 2: Parse the context

After fetching, read `/tmp/figma_context.json` and extract:

### Layout

- `absoluteBoundingBox` / `size` → width/height
- `layoutMode` (HORIZONTAL | VERTICAL | NONE) → flex direction
- `primaryAxisAlignItems`, `counterAxisAlignItems` → justify/align
- `paddingLeft/Right/Top/Bottom` → padding
- `itemSpacing` → gap
- `layoutGrow`, `layoutAlign` → flex-grow, align-self
- `constraints` → CSS position strategy

### Typography

- `fontFamily`, `fontWeight`, `fontSize`, `lineHeightPx`, `letterSpacing`, `textAlignHorizontal`
- Map to CSS: `font-family`, `font-weight`, `font-size`, `line-height`, `letter-spacing`, `text-align`

### Colors & Fills

- `fills[].type === "SOLID"` → `rgba(r*255, g*255, b*255, a)`
- `fills[].type === "GRADIENT_LINEAR"` → CSS `linear-gradient()`
- `fills[].type === "IMAGE"` → note as placeholder, ask user for asset
- `strokes[]` → `border`
- `effects[].type === "DROP_SHADOW"` → `box-shadow`
- `cornerRadius` / `rectangleCornerRadii` → `border-radius`

### Components & Variants

- If the node is a `COMPONENT` or `COMPONENT_SET`, extract `componentPropertyDefinitions` → Svelte props
- Variants map to prop-driven conditional classes or CSS custom properties

### Assets / Images

- Image fills need to be exported separately. Note which nodes have image fills and guide the user to export them from Figma (right-click → Export) or use the Figma API images endpoint.

---

## Step 3: Implement as SvelteKit component

See `references/svelte-patterns.md` for detailed Svelte idioms. Core rules:

### File structure

```
src/lib/components/
├── ComponentName.svelte
├── ComponentName.types.ts   (if complex props)
└── index.ts                 (barrel export)
```

### Component template

```svelte
<script lang="ts">
  // Props mapped from Figma component properties
  export let variant: 'primary' | 'secondary' = 'primary';
  export let label: string = '';
  export let disabled: boolean = false;
</script>

<!-- Markup reflecting Figma node hierarchy -->
<div class="component-root" class:disabled>
  <slot />
</div>

<style>
  /* Scoped styles — values from Figma context */
  .component-root {
    /* layout */
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    padding: 12px 16px;

    /* visual */
    background: var(--color-surface, #fff);
    border-radius: 8px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.12);
  }
</style>
```

### Key rules

- **Always use scoped `<style>`** — no global leakage
- **Map Figma colors to CSS custom properties** where a design token system is present; otherwise inline the extracted rgba values
- **Auto-layout → Flexbox/Grid**: `HORIZONTAL` = `flex-direction: row`, `VERTICAL` = `flex-direction: column`
- **Figma constraints → CSS position**: `SCALE` = `width: 100%`, `LEFT_RIGHT` = specific left/right values
- **Component properties → Svelte props**: boolean props = Figma booleans, string props = text overrides, variant props = union types
- **Pixel values**: use `px` directly from Figma — don't convert to rem unless the project uses rem-based tokens
- **`class:` directive** for variant/state styles rather than inline style strings
- DO NOT IMPLEMENT ANYTHING NON-UI RELATED. For example if it is a signup page DO NOT implement the form logic or props passing. JUST implement the structure and styling
- NO event handlers, NO exported callback props, NO store imports — structure and styles only

### Responsive considerations

Figma designs are fixed-size frames. Ask the user: _"Should this component be fixed-width, fluid, or responsive? Figma's frame was Xpx wide."_ Then apply appropriate width strategy.

---

## Step 4: Emit output

Produce:

1. The `.svelte` file(s) — one per Figma component/frame
2. A brief **implementation notes** section explaining:
   - Any assets that need to be exported from Figma manually
   - Any fonts that need to be loaded (`@font-face` or Google Fonts)
   - Deviations from the design (e.g. approximated gradients, placeholder images)
   - Suggested design tokens if a pattern was detected

---

## Error handling

| Situation                   | Action                                                             |
| --------------------------- | ------------------------------------------------------------------ |
| 401 from Figma API          | PAT is wrong or expired — ask user to regenerate                   |
| 403                         | File is private and PAT doesn't have access                        |
| Node not found              | Node ID may be stale — re-fetch at depth=2 and re-identify         |
| File too large (>10MB JSON) | Fetch with `?depth=2` first, then targeted node fetch              |
| Image fills                 | Note them, don't block — output placeholder `<img>` with a comment |
| Custom fonts                | List them in implementation notes — user must load them            |

---

## Reference files

- `scripts/fetch-figma.ts` — Fetches + prunes Figma API response (run with `bun run`)
- `references/svelte-patterns.md` — SvelteKit-specific idioms, stores, transitions, etc.
- `references/figma-api-nodes.md` — Node type reference (FRAME, COMPONENT, TEXT, VECTOR, etc.)
