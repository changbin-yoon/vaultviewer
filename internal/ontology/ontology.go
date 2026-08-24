// Package ontology builds the same note graph (nodes + typed/untyped
// edges) web/src/lib/vaultIndex.ts computes client-side for the graph
// view — as a backend-servable structure, so a consumer that isn't the
// AccessLens frontend (an AI agent, a script, an MCP server wrapping this
// API) can get the whole ontology in one call instead of fetching every
// note and re-implementing the frontmatter/wikilink parsing itself.
//
// This is a from-scratch Go port of that TS logic, not a shared
// implementation — see the design note in the git history for why (no
// Node runtime in the deploy image, and a cache of the frontend's own
// computation can't be trusted to be fresh when nobody has the app open).
// Keep the two in behavioral sync by hand: any change to the parsing
// rules in web/src/lib/{markdown,vaultIndex}.ts should have a matching
// change here, and vice versa.
package ontology

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/accesslens/accesslens/internal/storage"
)

// Node mirrors web/src/lib/vaultIndex.ts's GraphNode.
type Node struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Resolved bool    `json:"resolved"`
	Type     *string `json:"type,omitempty"`
}

// Edge mirrors web/src/lib/vaultIndex.ts's GraphEdge. Relation is nil for
// a plain [[wikilink]] found in a note's body, and set to the frontmatter
// field name (e.g. "depends_on") for a typed relation declared there.
type Edge struct {
	Source   string  `json:"source"`
	Target   string  `json:"target"`
	Relation *string `json:"relation,omitempty"`
}

// Graph is the full response /api/graph serves. Tags and backlinks are
// deliberately not included: backlinks are trivially derived by filtering
// Edges on Target, and tags aren't part of the ontology this endpoint
// exists for.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func isMarkdown(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}

// Build scans every markdown note reachable in engine and assembles the
// note graph.
func Build(engine storage.VaultStorageEngine) (*Graph, error) {
	type note struct {
		path, dir, frontmatter, body string
	}
	var notes []note
	known := map[string]bool{}

	err := storage.WalkFiles(engine, isMarkdown, func(p string, content []byte) error {
		fm, body := splitFrontmatter(string(content))
		notes = append(notes, note{path: p, dir: parentDir(p), frontmatter: fm, body: body})
		known[p] = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	nodes := map[string]Node{}
	var edges []Edge

	for _, n := range notes {
		nodes[n.path] = Node{ID: n.path, Name: path.Base(n.path), Resolved: true, Type: extractType(n.frontmatter)}
	}

	ensurePhantom := func(target, resolved string) {
		if known[resolved] {
			return
		}
		if _, exists := nodes[resolved]; exists {
			return
		}
		nodes[resolved] = Node{ID: resolved, Name: path.Base(target), Resolved: false}
	}

	for _, n := range notes {
		// Body only — scanning the frontmatter too would also match
		// [[wikilinks]] written inside a relation field below, double
		// counting each as a second, untyped edge to the same target.
		seen := map[string]bool{}
		for _, target := range extractWikilinkTargets(n.body) {
			resolved := resolveLinkTarget(target, n.dir)
			ensurePhantom(target, resolved)
			if seen[resolved] {
				continue
			}
			seen[resolved] = true
			edges = append(edges, Edge{Source: n.path, Target: resolved})
		}

		for _, rel := range extractFrontmatterRelations(n.frontmatter) {
			relation := rel.relation
			for _, target := range rel.targets {
				resolved := resolveLinkTarget(target, n.dir)
				ensurePhantom(target, resolved)
				edges = append(edges, Edge{Source: n.path, Target: resolved, Relation: &relation})
			}
		}
	}

	nodeList := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].ID < nodeList[j].ID })
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		return edges[i].Target < edges[j].Target
	})

	return &Graph{Nodes: nodeList, Edges: edges}, nil
}

func parentDir(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

// splitFrontmatter mirrors web/src/lib/markdown.ts's splitFrontmatter.
var frontmatterRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)

func splitFrontmatter(raw string) (frontmatter, body string) {
	m := frontmatterRe.FindStringSubmatchIndex(raw)
	if m == nil {
		return "", raw
	}
	return raw[m[2]:m[3]], raw[m[1]:]
}

// extractType mirrors extractFrontmatterType in vaultIndex.ts.
var typeRe = regexp.MustCompile(`(?m)^type:\s*(.+)$`)

func extractType(frontmatter string) *string {
	m := typeRe.FindStringSubmatch(frontmatter)
	if m == nil {
		return nil
	}
	value := strings.Trim(strings.TrimSpace(m[1]), `"'`)
	if value == "" {
		return nil
	}
	return &value
}

// extractFrontmatterRelations mirrors vaultIndex.ts's function of the
// same name: any frontmatter field besides title/type/tags whose value
// contains one or more [[wikilinks]] is a typed relation, the field name
// itself the relation's label.
var reservedFrontmatterKeys = map[string]bool{"title": true, "type": true, "tags": true}
var keyLineRe = regexp.MustCompile(`^([A-Za-z][\w-]*):\s*(.*)$`)
var blockItemRe = regexp.MustCompile(`^\s*-\s*(.+)$`)
var wikilinkInValueRe = regexp.MustCompile(`\[\[([^\]|#]+)`)

type relation struct {
	relation string
	targets  []string
}

func extractFrontmatterRelations(frontmatter string) []relation {
	if frontmatter == "" {
		return nil
	}
	lines := strings.Split(frontmatter, "\n")
	var out []relation
	for i := 0; i < len(lines); i++ {
		m := keyLineRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
		if m == nil {
			continue
		}
		key := m[1]
		if reservedFrontmatterKeys[strings.ToLower(key)] {
			continue
		}

		inline := strings.TrimSpace(m[2])
		var values []string
		switch {
		case strings.HasPrefix(inline, "["):
			trimmed := strings.TrimSuffix(strings.TrimPrefix(inline, "["), "]")
			for _, v := range strings.Split(trimmed, ",") {
				values = append(values, strings.TrimSpace(v))
			}
		case inline != "":
			values = []string{inline}
		default:
			for j := i + 1; j < len(lines); j++ {
				bm := blockItemRe.FindStringSubmatch(strings.TrimRight(lines[j], "\r"))
				if bm == nil {
					break
				}
				values = append(values, strings.TrimSpace(bm[1]))
			}
		}

		var targets []string
		for _, v := range values {
			if tm := wikilinkInValueRe.FindStringSubmatch(v); tm != nil {
				if t := strings.TrimSpace(tm[1]); t != "" {
					targets = append(targets, t)
				}
			}
		}
		if len(targets) > 0 {
			out = append(out, relation{relation: key, targets: targets})
		}
	}
	return out
}

// codeSegmentRe / extractWikilinkTargets mirror markdown.ts's
// stripCode+extractWikilinkTargets: skip fenced/inline code so literal
// syntax examples in documentation notes are never mistaken for real
// links, and exclude attachment (image) targets, matching the graph view.
var codeSegmentRe = regexp.MustCompile("(?s)(```.*?```|`[^`\n]*`)")
var wikilinkTargetRe = regexp.MustCompile(`!?\[\[([^\]|#]+)(#[^\]|]+)?(\|[^\]]+)?\]\]`)
var imageExtRe = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|svg|webp|bmp)$`)

func stripCode(md string) string {
	var b strings.Builder
	last := 0
	for _, loc := range codeSegmentRe.FindAllStringIndex(md, -1) {
		b.WriteString(md[last:loc[0]])
		last = loc[1]
	}
	b.WriteString(md[last:])
	return b.String()
}

func extractWikilinkTargets(raw string) []string {
	text := stripCode(raw)
	var targets []string
	for _, m := range wikilinkTargetRe.FindAllStringSubmatch(text, -1) {
		target := strings.TrimSpace(m[1])
		if !imageExtRe.MatchString(target) {
			targets = append(targets, target)
		}
	}
	return targets
}

// resolveLinkTarget mirrors markdown.ts's resolveLinkTarget with
// forEmbed=false (the only mode used for note-to-note relations/links).
var hasExtRe = regexp.MustCompile(`(?i)\.[a-z0-9]+$`)

func resolveLinkTarget(target, currentDir string) string {
	clean := strings.TrimPrefix(strings.TrimPrefix(target, "./"), "/")
	withExt := clean
	if !hasExtRe.MatchString(clean) {
		withExt = clean + ".md"
	}
	if strings.Contains(clean, "/") {
		return withExt
	}
	if currentDir == "" {
		return withExt
	}
	return currentDir + "/" + withExt
}
