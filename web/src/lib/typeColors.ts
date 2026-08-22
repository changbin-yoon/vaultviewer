// Deterministic color assignment for the graph view's node types
// (frontmatter "type:" values are free-form user vocabulary, not a fixed
// enum, so colors are picked by hashing the string rather than a lookup
// table). Hues are chosen to read distinctly from each other while
// staying in the same muted-wireframe family as the app's lavender accent.
const PALETTE = [
  "#7c6bab", // lavender (accent)
  "#5980a6", // steel blue
  "#4f9d8f", // teal
  "#c98a3c", // amber
  "#c2607c", // rose
  "#7a9d5c", // sage
  "#6b7690", // slate
  "#b8654f", // terracotta
];

export function colorForType(type: string): string {
  let hash = 0;
  for (let i = 0; i < type.length; i++) {
    hash = (hash * 31 + type.charCodeAt(i)) | 0;
  }
  return PALETTE[Math.abs(hash) % PALETTE.length];
}
