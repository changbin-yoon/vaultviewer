// Preprocessing for Obsidian-flavored Markdown: standard remark/GFM doesn't
// understand wikilinks, embeds, or callouts, so we rewrite them into plain
// markdown constructs (real links, a sentinel-marked blockquote paragraph)
// before handing the string to react-markdown.

export interface ParsedDoc {
  frontmatter: string | null;
  body: string;
}

// Obsidian notes commonly start with a "---\n...\n---" YAML block. We don't
// parse it (no YAML dependency) — just split it off so it can be shown as
// raw metadata instead of breaking the markdown renderer (a bare "---"
// otherwise parses as a thematic break with stray text below it).
export function splitFrontmatter(raw: string): ParsedDoc {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(raw);
  if (!match) return { frontmatter: null, body: raw };
  return { frontmatter: match[1], body: raw.slice(match[0].length) };
}

const IMAGE_EXT = /\.(png|jpe?g|gif|svg|webp|bmp)$/i;

// Folders used purely to store uploaded images/attachments (see
// MarkdownDocument's image upload, which always targets "attachments/")
// aren't meant to be browsed directly — the tree and folder browser both
// hide them.
export function isManagedFolderName(name: string): boolean {
  return name.toLowerCase() === "attachments";
}

// Obsidian never shows the ".md" extension in its own UI — match that here.
// Display-only: strips a trailing ".md" from a name or full path (the
// underlying file path used for API calls always keeps the real extension).
export function stripMdExtension(nameOrPath: string): string {
  return nameOrPath.replace(/\.md$/i, "");
}

// Fenced code blocks (```...```) and inline code spans (`...`) are shown
// verbatim — text inside them documenting wikilink/embed/callout syntax
// (as this very file's own examples do) must never be rewritten as if it
// were a real link. Every raw-markdown scan below splits on this first and
// only touches the non-code segments (the even indices of String#split
// with a capturing group).
const CODE_SEGMENT = /(```[\s\S]*?```|`[^`\n]*`)/g;

export function mapOutsideCode(md: string, transform: (text: string) => string): string {
  return md
    .split(CODE_SEGMENT)
    .map((segment, i) => (i % 2 === 0 ? transform(segment) : segment))
    .join("");
}

function stripCode(md: string): string {
  return md
    .split(CODE_SEGMENT)
    .filter((_, i) => i % 2 === 0)
    .join("");
}

// [[target]] / [[target|Alias]] / [[target#heading|Alias]] -> a real
// markdown link the custom `a` renderer intercepts by its "wikilink:" and
// "embed:" pseudo-schemes, since remark otherwise leaves "[[...]]" as
// literal text.
export function preprocessObsidian(md: string): string {
  return mapOutsideCode(md, (text) => {
    let out = preprocessCallouts(text);

    // Embeds: ![[file]] or ![[file|alt]]
    out = out.replace(/!\[\[([^\]|#]+)(#[^\]|]+)?(\|([^\]]+))?\]\]/g, (_m, target, _anchor, _p, alias) => {
      const t = (target as string).trim();
      const label = (alias as string | undefined)?.trim() || t;
      return `![${label}](embed:${encodeURIComponent(t)})`;
    });

    // Wikilinks: [[file]] or [[file|alias]]
    out = out.replace(/\[\[([^\]|#]+)(#[^\]|]+)?(\|([^\]]+))?\]\]/g, (_m, target, _anchor, _p, alias) => {
      const t = (target as string).trim();
      const label = (alias as string | undefined)?.trim() || t;
      return `[${label}](wikilink:${encodeURIComponent(t)})`;
    });

    return out;
  });
}

// Rewrites "> [!type] Title" callout headers into a sentinel paragraph
// ("%%CALLOUT:type:Title%%") followed by a blank quote line, which forces
// remark to split the header into its own paragraph within the blockquote
// so the Blockquote component can pull it out and style the box.
function preprocessCallouts(md: string): string {
  return md.replace(/^> ?\[!(\w+)\]([^\n]*)$/gm, (_m, type, title) => {
    return `> %%CALLOUT:${type}:${(title as string).trim()}%%\n>`;
  });
}

export function isImageTarget(target: string): boolean {
  return IMAGE_EXT.test(target);
}

// Raw [[target]] / [[target#heading|alias]] references in a note's text,
// note-links only (attachment embeds are excluded, matching Obsidian's
// graph view). Used to build the graph view's edge list.
export function extractWikilinkTargets(raw: string): string[] {
  const targets: string[] = [];
  const re = /!?\[\[([^\]|#]+)(#[^\]|]+)?(\|[^\]]+)?\]\]/g;
  const text = stripCode(raw);
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    const target = m[1].trim();
    if (!isImageTarget(target)) targets.push(target);
  }
  return targets;
}

// Resolves a wikilink/embed target to a vault-relative path (matching the
// FileItem.path format used elsewhere). Obsidian omits the .md extension
// and allows bare filenames resolved from anywhere in the vault; we only
// support the common cases: an explicit vault-relative path, or a bare
// name relative to the current file's directory.
export function resolveLinkTarget(target: string, currentDir: string, forEmbed: boolean): string {
  const clean = decodeURIComponent(target).replace(/^\.?\//, "");
  const hasExt = /\.[a-z0-9]+$/i.test(clean);
  const withExt = hasExt ? clean : `${clean}.md`;
  if (clean.includes("/")) return withExt;
  if (forEmbed && !hasExt) return withExt; // rare: bare markdown embed name
  return currentDir ? `${currentDir}/${withExt}` : withExt;
}

// [[target]] / [[target|alias]] / ![[target#heading|alias]] — matches the
// same raw-target grammar as extractWikilinkTargets/preprocessObsidian.
const WIKILINK_TARGET = /(!?\[\[)([^\]|#]+)((?:#[^\]|]+)?(?:\|[^\]]+)?\]\])/g;

// Rewrites every [[...]]/![[...]] reference to oldName in md so it points
// at newName instead — used when renaming a note within the same
// directory, to keep other notes' links from breaking. Only the raw
// target's final path segment is compared/replaced (case-sensitive, exact
// match, extension stripped) — any leading directory, #heading anchor,
// |alias, and embed-vs-link form are left exactly as the user wrote them.
// Runs outside code blocks/spans only (see mapOutsideCode) so literal
// syntax examples in documentation notes are never rewritten.
export function renameWikilinkReferences(md: string, oldName: string, newName: string): string {
  return mapOutsideCode(md, (text) =>
    text.replace(WIKILINK_TARGET, (whole, prefix, rawTarget, suffix) => {
      const clean = (rawTarget as string).trim();
      const hasExt = /\.[a-z0-9]+$/i.test(clean);
      const dot = hasExt ? clean.lastIndexOf(".") : clean.length;
      const stem = clean.slice(0, dot);
      const ext = clean.slice(dot);
      const slash = stem.lastIndexOf("/");
      const dir = slash >= 0 ? stem.slice(0, slash + 1) : "";
      const bareName = slash >= 0 ? stem.slice(slash + 1) : stem;
      // A note is never an image — skip embeds pointing at attachments
      // (e.g. ![[some.png]]) even if their stem happens to match oldName.
      if (bareName !== oldName || isImageTarget(clean)) return whole;
      return `${prefix}${dir}${newName}${ext}${suffix}`;
    })
  );
}
