# Svelte / SvelteKit Component Patterns

Reference for implementing Figma designs as idiomatic SvelteKit components.

---

## Component anatomy

```svelte
<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { ComponentProps } from './ComponentName.types';

  // Props (map from Figma component properties)
  export let variant: 'primary' | 'secondary' | 'ghost' = 'primary';
  export let size: 'sm' | 'md' | 'lg' = 'md';
  export let disabled = false;
  export let loading = false;

  // Internal state
  let hovered = false;

  // Events
  const dispatch = createEventDispatcher<{ click: MouseEvent }>();
</script>

<button
  class="btn btn--{variant} btn--{size}"
  class:disabled
  class:loading
  {disabled}
  on:click={(e) => dispatch('click', e)}
  on:mouseenter={() => (hovered = true)}
  on:mouseleave={() => (hovered = false)}
>
  <slot />
</button>

<style>
  .btn { /* base styles */ }
  .btn--primary { /* variant */ }
  .btn--sm { /* size */ }
  .disabled { opacity: 0.5; pointer-events: none; }
</style>
```

---

## Mapping Figma → Svelte props

| Figma concept                      | Svelte equivalent                                        |
| ---------------------------------- | -------------------------------------------------------- |
| Component property (Boolean)       | `export let propName: boolean = false`                   |
| Component property (Text)          | `export let text: string = 'Default'`                    |
| Component property (Instance swap) | `export let icon: typeof SvelteComponent \| null = null` |
| Variant property                   | Union type prop: `export let variant: 'A' \| 'B' = 'A'`  |
| Exposed nested prop                | Destructure or pass through slot                         |

---

## Auto-layout → CSS

| Figma                                  | CSS                                     |
| -------------------------------------- | --------------------------------------- |
| `layoutMode: HORIZONTAL`               | `display: flex; flex-direction: row`    |
| `layoutMode: VERTICAL`                 | `display: flex; flex-direction: column` |
| `primaryAxisAlignItems: MIN`           | `justify-content: flex-start`           |
| `primaryAxisAlignItems: CENTER`        | `justify-content: center`               |
| `primaryAxisAlignItems: MAX`           | `justify-content: flex-end`             |
| `primaryAxisAlignItems: SPACE_BETWEEN` | `justify-content: space-between`        |
| `counterAxisAlignItems: MIN`           | `align-items: flex-start`               |
| `counterAxisAlignItems: CENTER`        | `align-items: center`                   |
| `counterAxisAlignItems: MAX`           | `align-items: flex-end`                 |
| `counterAxisAlignItems: BASELINE`      | `align-items: baseline`                 |
| `itemSpacing: N`                       | `gap: Npx`                              |
| `layoutGrow: 1`                        | `flex: 1`                               |
| `layoutAlign: STRETCH`                 | `align-self: stretch`                   |
| `primaryAxisSizingMode: FIXED`         | explicit `width` or `height`            |
| `primaryAxisSizingMode: AUTO`          | no explicit sizing (shrink-wrap)        |
| `counterAxisSizingMode: FIXED`         | explicit cross-axis size                |
| `counterAxisSizingMode: AUTO`          | `align-self: flex-start` (or auto)      |

---

## Constraints → CSS position

When a node has `constraints` and is NOT in an auto-layout frame:

| Figma constraint       | CSS approach                                         |
| ---------------------- | ---------------------------------------------------- |
| horizontal: LEFT       | `left: Xpx` (absolute)                               |
| horizontal: RIGHT      | `right: Xpx` (absolute)                              |
| horizontal: LEFT_RIGHT | `left: Xpx; right: Xpx` or `width: calc(100% - Xpx)` |
| horizontal: CENTER     | `left: 50%; transform: translateX(-50%)`             |
| horizontal: SCALE      | `width: X%` (relative to parent)                     |
| vertical: TOP          | `top: Ypx`                                           |
| vertical: BOTTOM       | `bottom: Ypx`                                        |
| vertical: TOP_BOTTOM   | `top: Ypx; bottom: Ypx`                              |
| vertical: CENTER       | `top: 50%; transform: translateY(-50%)`              |
| vertical: SCALE        | `height: Y%`                                         |

Use `position: absolute` on the child and `position: relative` on the parent for constrained layouts.

---

## Color fills

```typescript
// Figma SOLID fill (after pruning, values are 0-255)
// { type: "SOLID", color: { r: 255, g: 87, b: 51, a: 1 }, opacity: 1 }
// → CSS:
const fill = `rgba(${color.r}, ${color.g}, ${color.b}, ${color.a * opacity})`;

// Figma GRADIENT_LINEAR
// { type: "GRADIENT_LINEAR", gradientStops: [...], gradientHandlePositions: [...] }
// → Approximate as CSS linear-gradient; compute angle from handle positions
```

---

## Typography

```svelte
<style>
  .text-heading {
    font-family: 'Inter', sans-serif;   /* fontFamily */
    font-weight: 700;                    /* fontWeight */
    font-size: 24px;                     /* fontSize */
    line-height: 32px;                   /* lineHeightPx */
    letter-spacing: -0.5px;             /* letterSpacing (Figma uses 0-100 scale; divide by 100 * fontSize for em, or use px directly) */
    text-align: left;                    /* textAlignHorizontal → lowercase */
  }
</style>
```

**Letter spacing note**: Figma's `letterSpacing` is in percentage of font size (e.g. `5` = 5% of `fontSize`). Convert: `letterSpacing_css = (figma_letterSpacing / 100) * fontSize + "px"`. Or if `letterSpacingUnit` is `PIXELS`, use directly.

---

## Effects → CSS

```css
/* DROP_SHADOW effect */
/* { type: "DROP_SHADOW", color: {r,g,b,a}, offset: {x,y}, radius, spread } */
box-shadow: Xpx Ypx RADIUSpx SPREADpx rgba(r, g, b, a);

/* INNER_SHADOW */
box-shadow: inset Xpx Ypx RADIUSpx rgba(r, g, b, a);

/* LAYER_BLUR */
filter: blur(RADIUSpx);

/* BACKGROUND_BLUR */
backdrop-filter: blur(RADIUSpx);
```

---

## Slots vs props for content

```svelte
<!-- Use slots when Figma shows text/icon as swappable content areas -->
<div class="card">
  <slot name="header" />
  <slot />  <!-- default slot for body -->
  <slot name="footer" />
</div>

<!-- Use props when Figma shows fixed text or a specific string property -->
<script>
  export let label: string;
  export let count: number = 0;
</script>
<span>{label} ({count})</span>
```

---

## Transitions (optional enhancement)

```svelte
<script>
  import { fade, fly, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
</script>

<!-- Fade in on mount -->
{#if visible}
  <div transition:fade={{ duration: 200 }}>...</div>
{/if}

<!-- Slide up -->
<div in:fly={{ y: 12, duration: 250, easing: cubicOut }}>...</div>
```

---

## CSS custom properties for design tokens

If the project uses design tokens (detected by repeated color/spacing values across components), output a `tokens.css` and reference via vars:

```css
/* src/lib/styles/tokens.css */
:root {
  --color-primary: rgba(99, 102, 241, 1);
  --color-surface: rgba(255, 255, 255, 1);
  --color-text: rgba(17, 24, 39, 1);
  --radius-md: 8px;
  --space-sm: 8px;
  --space-md: 16px;
}
```

Then in components: `background: var(--color-primary);`

---

## SvelteKit-specific: `$lib` imports

Always use `$lib` alias for internal imports:

```typescript
import Button from "$lib/components/Button.svelte";
import type { ButtonProps } from "$lib/components/Button.types";
```

---

## Responsive strategy

Figma frames are fixed-pixel designs. Default approach:

- Use `max-width` + `width: 100%` for fluid containers
- Preserve fixed sizes only for things that shouldn't resize (icons, avatars, specific UI elements)
- Ask the user about breakpoints if the design shows multiple frame sizes

---

## Common gotchas

1. **Figma `opacity` vs fill alpha**: A node can have both `opacity: 0.8` AND `fills[0].color.a: 0.5`. The effective alpha is `0.8 * 0.5 = 0.4`. Apply `opacity` to the element; fill alpha is already part of the color.

2. **Groups vs Frames**: Figma `GROUP` nodes have no independent layout — their bounds are the union of children. Render as `<div style="position:relative">` with absolutely-positioned children, or flatten if the group is purely visual.

3. **INSTANCE nodes**: These are component usages. Their `componentId` maps to a component in `data.components`. Use the pruned instance properties to determine which Svelte component to render and with what props.

4. **Hidden nodes**: The fetch script strips `visible: false` nodes. If a component has variant-dependent visibility, re-check the Figma source.

5. **Image fills**: Output a placeholder `<img src="" alt="[figma image fill]" />` with a comment noting the node name. The user must export the asset from Figma.

6. **Overflowing content**: If `clipsContent: true` on a frame, add `overflow: hidden` to the CSS.
