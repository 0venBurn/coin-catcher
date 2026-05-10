#!/usr/bin/env bun
/**
 * fetch-figma.ts — Fetch and prune Figma API data for LLM consumption.
 *
 * Usage:
 *   FIGMA_API_KEY must be exported in the environment.
 *
 *   bun run scripts/fetch-figma.ts --file-key FILE_KEY
 *   bun run scripts/fetch-figma.ts --file-key FILE_KEY --node-id "123:456"
 *   bun run scripts/fetch-figma.ts --file-key FILE_KEY --depth 2
 *   bun run scripts/fetch-figma.ts --file-key FILE_KEY --output /tmp/figma_context.json
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface FigmaColor {
  r: number;
  g: number;
  b: number;
  a: number;
}

interface FigmaFill {
  type: string;
  visible?: boolean;
  color?: FigmaColor;
  opacity?: number;
  imageRef?: string;
  gifRef?: string;
  gradientStops?: unknown[];
  gradientHandlePositions?: unknown[];
  [key: string]: unknown;
}

interface FigmaEffect {
  type: string;
  visible?: boolean;
  [key: string]: unknown;
}

interface FigmaTextStyle {
  fontFamily?: string;
  fontPostScriptName?: string;
  fontWeight?: number;
  fontSize?: number;
  lineHeightPx?: number;
  lineHeightPercent?: number;
  letterSpacing?: number;
  textAlignHorizontal?: string;
  textAlignVertical?: string;
  textDecoration?: string;
  textCase?: string;
  italic?: boolean;
  [key: string]: unknown;
}

interface FigmaNode {
  id: string;
  name: string;
  type: string;
  visible?: boolean;
  children?: FigmaNode[];
  fills?: FigmaFill[];
  effects?: FigmaEffect[];
  style?: FigmaTextStyle;
  characters?: string;
  [key: string]: unknown;
}

interface PrunedNode {
  id: string;
  name: string;
  type: string;
  _childCount?: number;
  children?: PrunedNode[];
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const KEEP_FIELDS = new Set([
  "id",
  "name",
  "type",
  "visible",
  // Layout
  "absoluteBoundingBox",
  "size",
  "relativeTransform",
  "layoutMode",
  "primaryAxisSizingMode",
  "counterAxisSizingMode",
  "primaryAxisAlignItems",
  "counterAxisAlignItems",
  "paddingLeft",
  "paddingRight",
  "paddingTop",
  "paddingBottom",
  "itemSpacing",
  "layoutGrow",
  "layoutAlign",
  "layoutPositioning",
  "constraints",
  "clipsContent",
  // Visual
  "fills",
  "strokes",
  "strokeWeight",
  "strokeAlign",
  "effects",
  "opacity",
  "blendMode",
  "cornerRadius",
  "rectangleCornerRadii",
  // Typography
  "characters",
  "style",
  // Components
  "componentId",
  "componentProperties",
  "componentPropertyDefinitions",
  "variantProperties",
  // Children — handled separately in pruneNode
  "children",
]);

const STYLE_KEEP_FIELDS = new Set([
  "fontFamily",
  "fontPostScriptName",
  "fontWeight",
  "fontSize",
  "lineHeightPx",
  "lineHeightPercent",
  "letterSpacing",
  "textAlignHorizontal",
  "textAlignVertical",
  "textDecoration",
  "textCase",
  "italic",
]);

const FILL_DROP_FIELDS = new Set(["imageRef", "gifRef"]);
const TOKEN = process.env.FIGMA_API_KEY;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function requireToken(): string {
  if (!TOKEN) {
    console.error("FIGMA_API_KEY environment variable is required");
    process.exit(1);
  }

  return TOKEN;
}

async function figmaGet(path: string, token: string): Promise<unknown> {
  const url = `https://api.figma.com/v1${path}`;
  const res = await fetch(url, {
    headers: { "X-Figma-Token": token },
  });

  if (!res.ok) {
    const body = await res.text();
    console.error(`HTTP ${res.status} from Figma API: ${body}`);
    process.exit(1);
  }

  return res.json();
}

function pruneFill(fill: FigmaFill): Omit<FigmaFill, "imageRef" | "gifRef"> {
  const result: Record<string, unknown> = {};

  for (const [k, v] of Object.entries(fill)) {
    if (!FILL_DROP_FIELDS.has(k)) {
      result[k] = v;
    }
  }

  // Normalise color from 0–1 floats to 0–255 integers for readability
  if (result.color) {
    const c = result.color as FigmaColor;
    result.color = {
      r: Math.round(c.r * 255),
      g: Math.round(c.g * 255),
      b: Math.round(c.b * 255),
      a: Math.round(c.a * 1000) / 1000,
    };
  }

  return result as Omit<FigmaFill, "imageRef" | "gifRef">;
}

function pruneNode(
  node: FigmaNode,
  depth = 0,
  maxDepth = Infinity,
): PrunedNode | null {
  // Strip invisible nodes entirely
  if (node.visible === false) return null;

  const pruned: Record<string, unknown> = {};

  for (const key of KEEP_FIELDS) {
    if (!(key in node)) continue;
    const val = node[key];

    if (key === "fills" && Array.isArray(val)) {
      pruned[key] = (val as FigmaFill[])
        .filter((f) => f.visible !== false)
        .map(pruneFill);
    } else if (
      key === "style" &&
      node.type === "TEXT" &&
      val &&
      typeof val === "object"
    ) {
      pruned[key] = Object.fromEntries(
        Object.entries(val as FigmaTextStyle).filter(([k]) =>
          STYLE_KEEP_FIELDS.has(k),
        ),
      );
    } else if (key === "effects" && Array.isArray(val)) {
      pruned[key] = (val as FigmaEffect[]).filter((e) => e.visible !== false);
    } else if (key === "children" && Array.isArray(val)) {
      if (depth >= maxDepth) {
        pruned["_childCount"] = val.length;
      } else {
        const children = (val as FigmaNode[])
          .map((child) => pruneNode(child, depth + 1, maxDepth))
          .filter((c): c is PrunedNode => c !== null);
        if (children.length > 0) {
          pruned["children"] = children;
        }
      }
    } else {
      pruned[key] = val;
    }
  }

  return pruned as PrunedNode;
}

function extractStyles(data: Record<string, unknown>): Record<string, unknown> {
  const styles = data.styles as
    | Record<string, Record<string, unknown>>
    | undefined;
  if (!styles) return {};

  return Object.fromEntries(
    Object.entries(styles).map(([id, style]) => [
      id,
      {
        name: style.name,
        type: style.styleType,
        description: style.description ?? "",
      },
    ]),
  );
}

function extractComponents(
  data: Record<string, unknown>,
): Record<string, unknown> {
  const components = data.components as
    | Record<string, Record<string, unknown>>
    | undefined;
  if (!components) return {};

  return Object.fromEntries(
    Object.entries(components).map(([id, comp]) => [
      id,
      {
        name: comp.name,
        description: comp.description ?? "",
        componentSetId: comp.componentSetId,
      },
    ]),
  );
}

function parseArgs() {
  const args = process.argv.slice(2);
  const get = (flag: string) => {
    const i = args.indexOf(flag);
    return i !== -1 ? args[i + 1] : undefined;
  };

  const fileKey = get("--file-key");

  if (!fileKey) {
    console.error(
      "Usage: bun run fetch-figma.ts --file-key KEY [--node-id ID] [--depth N] [--output PATH]",
    );
    process.exit(1);
  }

  return {
    fileKey,
    token: requireToken(),
    nodeId: get("--node-id"),
    depth: get("--depth") ? parseInt(get("--depth")!, 10) : Infinity,
    output: get("--output") ?? "/tmp/figma_context.json",
  };
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const { fileKey, token, nodeId, depth, output } = parseArgs();

  console.error(`Fetching Figma file ${fileKey}...`);

  let result: Record<string, unknown>;

  if (nodeId) {
    const encoded = encodeURIComponent(nodeId);
    const data = (await figmaGet(
      `/files/${fileKey}/nodes?ids=${encoded}`,
      token,
    )) as Record<string, unknown>;
    const nodesData = data.nodes as Record<string, Record<string, unknown>>;

    // Node key in response may use different colon encoding — find it
    const nodeKey =
      Object.keys(nodesData).find(
        (k) => k === nodeId || k === nodeId.replace(":", "%3A"),
      ) ?? Object.keys(nodesData)[0];

    if (!nodeKey) {
      console.error(`Node ${nodeId} not found in response`);
      process.exit(1);
    }

    const rawNode = nodesData[nodeKey].document as FigmaNode;
    const pruned = pruneNode(rawNode, 0, depth);

    result = {
      fileKey,
      fetchType: "node",
      nodeId,
      node: pruned,
      styles: (nodesData[nodeKey].styles as Record<string, unknown>) ?? {},
    };
  } else {
    const depthParam = isFinite(depth) ? `?depth=${depth}` : "";
    const data = (await figmaGet(
      `/files/${fileKey}${depthParam}`,
      token,
    )) as Record<string, unknown>;

    const document = data.document as FigmaNode;
    const pruned = pruneNode(document, 0, depth);

    result = {
      fileKey,
      fileName: data.name ?? "",
      fetchType: isFinite(depth) ? "shallow" : "full",
      ...(isFinite(depth) && { depth }),
      document: pruned,
      styles: extractStyles(data),
      components: extractComponents(data),
    };
  }

  const json = JSON.stringify(result, null, 2);
  await Bun.write(output, json);

  const sizeKb = Buffer.byteLength(json) / 1024;
  console.error(`Written to ${output} (${sizeKb.toFixed(1)} KB)`);

  if (sizeKb > 200) {
    console.error(
      "WARNING: Output is large. Re-fetch with --node-id to target a specific frame.",
    );
  }
}

main();
