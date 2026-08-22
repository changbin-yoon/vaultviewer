import * as api from "./api";
import { extractWikilinkTargets, resolveLinkTarget, splitFrontmatter } from "./markdown";

export interface GraphNode {
  id: string; // vault path
  name: string;
  resolved: boolean; // false = a link target with no matching note (phantom node, as in Obsidian)
  type: string | null; // frontmatter "type:" field, if any — drives graph node color
}

export interface GraphEdge {
  source: string;
  target: string;
}

export interface VaultIndex {
  nodes: GraphNode[];
  edges: GraphEdge[];
  backlinks: Map<string, string[]>; // note path -> paths of notes linking to it
  tags: Map<string, string[]>; // tag (without '#') -> note paths carrying it
}

function parentDir(path: string) {
  const parts = path.split("/");
  parts.pop();
  return parts.join("/");
}

// Frontmatter "tags:" as either a flow list ("tags: [a, b]"), a scalar
// ("tags: a"), or a block list ("tags:\n  - a\n  - b").
function extractFrontmatterTags(frontmatter: string): string[] {
  const lines = frontmatter.split(/\r?\n/);
  const idx = lines.findIndex((l) => /^tags:\s*(.*)$/.test(l));
  if (idx === -1) return [];
  const inline = lines[idx].replace(/^tags:\s*/, "").trim();
  if (inline.startsWith("[")) {
    return inline
      .replace(/^\[|\]$/g, "")
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
  }
  if (inline) return [inline];
  const tags: string[] = [];
  for (let i = idx + 1; i < lines.length; i++) {
    const m = /^\s*-\s*(.+)$/.exec(lines[i]);
    if (!m) break;
    tags.push(m[1].trim());
  }
  return tags;
}

// Frontmatter "type:" as a scalar value (quotes stripped). This is a plain
// user-defined string, not a fixed enum — the graph view assigns each
// distinct value a color deterministically.
function extractFrontmatterType(frontmatter: string): string | null {
  const m = /^type:\s*(.+)$/m.exec(frontmatter);
  if (!m) return null;
  const value = m[1].trim().replace(/^["']|["']$/g, "");
  return value || null;
}

// Inline #tag references in the note body (Obsidian tag syntax): a '#'
// immediately followed by word/Korean characters, not preceded by another
// word character (so it isn't matched mid-word or as a markdown heading,
// which requires a space after the '#').
function extractInlineTags(body: string): string[] {
  const tags: string[] = [];
  const re = /(^|[^\w#])#([\p{L}\p{N}_/-]+)/gu;
  let m: RegExpExecArray | null;
  while ((m = re.exec(body))) tags.push(m[2]);
  return tags;
}

let cached: Promise<VaultIndex> | null = null;

// Walks the whole vault once (breadth-first, via the same List API the file
// browser uses), fetching every note's content to build a note graph, tag
// index, and backlink map — mirroring Obsidian's graph/tags/backlinks
// panels. The result is memoized for the session; call with force:true to
// re-scan after edits.
export function getVaultIndex(force = false): Promise<VaultIndex> {
  if (!force && cached) return cached;
  cached = buildVaultIndex();
  return cached;
}

async function buildVaultIndex(): Promise<VaultIndex> {
  const mdPaths: string[] = [];
  let queue: string[] = [""];
  while (queue.length > 0) {
    const batches = await Promise.all(queue.map((p) => api.listTree(p).catch(() => [])));
    const nextQueue: string[] = [];
    for (const items of batches) {
      for (const item of items ?? []) {
        if (item.isDir) nextQueue.push(item.path);
        else if (item.name.toLowerCase().endsWith(".md")) mdPaths.push(item.path);
      }
    }
    queue = nextQueue;
  }

  const known = new Set(mdPaths);
  const nodes = new Map<string, GraphNode>();
  const edges: GraphEdge[] = [];
  const backlinks = new Map<string, string[]>();
  const tags = new Map<string, string[]>();

  const contents = await Promise.all(
    mdPaths.map((p) =>
      api
        .readFile(p)
        .then((f) => api.decodeContent(f.content))
        .catch(() => "")
    )
  );

  mdPaths.forEach((path, i) => {
    const dir = parentDir(path);
    const { frontmatter, body } = splitFrontmatter(contents[i]);
    nodes.set(path, {
      id: path,
      name: path.split("/").pop() ?? path,
      resolved: true,
      type: frontmatter ? extractFrontmatterType(frontmatter) : null,
    });

    for (const tag of [...(frontmatter ? extractFrontmatterTags(frontmatter) : []), ...extractInlineTags(body)]) {
      const clean = tag.replace(/^#/, "");
      if (!clean) continue;
      if (!tags.has(clean)) tags.set(clean, []);
      const list = tags.get(clean)!;
      if (!list.includes(path)) list.push(path);
    }

    const linkedFromThisNote = new Set<string>();
    for (const target of extractWikilinkTargets(contents[i])) {
      const resolved = resolveLinkTarget(target, dir, false);
      if (!known.has(resolved) && !nodes.has(resolved)) {
        nodes.set(resolved, { id: resolved, name: target.split("/").pop() ?? target, resolved: false, type: null });
      }
      // A note can link to the same target more than once (e.g. an
      // explanatory example plus a real link) — count that as a single
      // edge/backlink, not one per occurrence.
      if (linkedFromThisNote.has(resolved)) continue;
      linkedFromThisNote.add(resolved);
      edges.push({ source: path, target: resolved });
      if (!backlinks.has(resolved)) backlinks.set(resolved, []);
      backlinks.get(resolved)!.push(path);
    }
  });

  return { nodes: Array.from(nodes.values()), edges, backlinks, tags };
}
