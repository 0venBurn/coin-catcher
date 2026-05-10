# Figma API Node Types Reference

Quick reference for interpreting node types returned by the Figma REST API.

---

## Top-level document structure

```
DOCUMENT
└── PAGE (one or more)
    └── FRAME | SECTION | COMPONENT_SET
        └── ... (nested nodes)
```

---

## Node types

### Structural / Layout nodes

| Type            | Description                      | Render as                          |
| --------------- | -------------------------------- | ---------------------------------- |
| `DOCUMENT`      | Root document node               | — (never rendered)                 |
| `PAGE`          | A Figma page                     | — (never rendered)                 |
| `FRAME`         | Container with layout properties | `<div>`                            |
| `GROUP`         | Visual grouping, no layout       | `<div style="position:relative">`  |
| `SECTION`       | Organisational grouping          | — (usually ignored)                |
| `COMPONENT`     | Reusable component definition    | Svelte component file              |
| `COMPONENT_SET` | Collection of variants           | Svelte component with variant prop |
| `INSTANCE`      | Usage of a component             | `<ComponentName {...props}>`       |

### Content nodes

| Type                | Description                        | Render as                                  |
| ------------------- | ---------------------------------- | ------------------------------------------ |
| `TEXT`              | Text content with typography style | `<p>`, `<span>`, `<h1>`–`<h6>`, etc.       |
| `RECTANGLE`         | Filled/stroked rectangle           | `<div>` with CSS, or `<img>` if image fill |
| `ELLIPSE`           | Circle or oval                     | `<div>` with `border-radius: 50%`          |
| `VECTOR`            | Vector path                        | `<svg>` (export from Figma as SVG)         |
| `BOOLEAN_OPERATION` | Union/subtract/intersect of paths  | `<svg>` (export from Figma as SVG)         |
| `STAR`              | Star shape                         | `<svg>`                                    |
| `LINE`              | Straight line                      | `<hr>` or `<div>` with `height: Npx`       |
| `REGULAR_POLYGON`   | Triangle/polygon                   | `<svg>`                                    |

### Special

| Type        | Description         | Notes                        |
| ----------- | ------------------- | ---------------------------- |
| `SLICE`     | Export slice marker | Ignore — it's an export hint |
| `STICKY`    | FigJam sticky note  | Ignore                       |
| `CONNECTOR` | FigJam connector    | Ignore                       |

---

## Key node fields by type

### FRAME / COMPONENT / INSTANCE

```json
{
  "type": "FRAME",
  "layoutMode": "HORIZONTAL | VERTICAL | NONE",
  "primaryAxisAlignItems": "MIN | CENTER | MAX | SPACE_BETWEEN",
  "counterAxisAlignItems": "MIN | CENTER | MAX | BASELINE",
  "paddingLeft": 16,
  "paddingRight": 16,
  "paddingTop": 12,
  "paddingBottom": 12,
  "itemSpacing": 8,
  "clipsContent": true,
  "fills": [...],
  "strokes": [...],
  "effects": [...],
  "cornerRadius": 8,
  "children": [...]
}
```

### TEXT

```json
{
  "type": "TEXT",
  "characters": "Button label",
  "style": {
    "fontFamily": "Inter",
    "fontWeight": 600,
    "fontSize": 14,
    "lineHeightPx": 20,
    "letterSpacing": 0,
    "textAlignHorizontal": "LEFT | CENTER | RIGHT | JUSTIFIED",
    "textAlignVertical": "TOP | CENTER | BOTTOM",
    "textDecoration": "NONE | UNDERLINE | STRIKETHROUGH",
    "textCase": "ORIGINAL | UPPER | LOWER | TITLE"
  },
  "fills": [{ "type": "SOLID", "color": { "r": 17, "g": 24, "b": 39, "a": 1 } }]
}
```

### COMPONENT_SET (variants)

```json
{
  "type": "COMPONENT_SET",
  "name": "Button",
  "componentPropertyDefinitions": {
    "Variant": {
      "type": "VARIANT",
      "defaultValue": "Primary",
      "variantOptions": ["Primary", "Secondary", "Ghost"]
    },
    "Size": {
      "type": "VARIANT",
      "defaultValue": "Medium",
      "variantOptions": ["Small", "Medium", "Large"]
    },
    "Disabled": {
      "type": "BOOLEAN",
      "defaultValue": "false"
    },
    "Label": {
      "type": "TEXT",
      "defaultValue": "Button"
    }
  }
}
```

### INSTANCE

```json
{
  "type": "INSTANCE",
  "componentId": "abc123",
  "componentProperties": {
    "Variant": { "type": "VARIANT", "value": "Secondary" },
    "Label": { "type": "TEXT", "value": "Cancel" },
    "Disabled": { "type": "BOOLEAN", "value": "false" }
  }
}
```

---

## Fill types

```json
// Solid
{ "type": "SOLID", "color": { "r": 255, "g": 87, "b": 51, "a": 1 }, "opacity": 1 }

// Linear gradient
{
  "type": "GRADIENT_LINEAR",
  "gradientHandlePositions": [
    { "x": 0.5, "y": 0 },
    { "x": 0.5, "y": 1 },
    { "x": 1, "y": 0 }
  ],
  "gradientStops": [
    { "color": { "r": 99, "g": 102, "b": 241, "a": 1 }, "position": 0 },
    { "color": { "r": 139, "g": 92, "b": 246, "a": 1 }, "position": 1 }
  ]
}

// Image fill
{ "type": "IMAGE", "scaleMode": "FILL | FIT | CROP | TILE" }
// → imageRef stripped by fetch script — render as placeholder img
```

---

## Effect types

```json
// Drop shadow
{
  "type": "DROP_SHADOW",
  "color": { "r": 0, "g": 0, "b": 0, "a": 0.12 },
  "offset": { "x": 0, "y": 2 },
  "radius": 8,
  "spread": 0,
  "visible": true
}

// Inner shadow
{ "type": "INNER_SHADOW", ... }

// Layer blur
{ "type": "LAYER_BLUR", "radius": 4 }

// Background blur
{ "type": "BACKGROUND_BLUR", "radius": 20 }
```

---

## Sizing modes

| Value             | Meaning                                                 |
| ----------------- | ------------------------------------------------------- |
| `FIXED`           | Explicit pixel size — use it directly                   |
| `AUTO`            | Hugs content — don't set explicit size                  |
| `FILL` (on child) | Stretch to fill parent — use `flex: 1` or `width: 100%` |

A node's `layoutGrow: 1` means it fills available space in the parent's primary axis.
