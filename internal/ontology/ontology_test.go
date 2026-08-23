package ontology

import (
	"testing"

	"github.com/vaultviewer/vaultviewer/internal/audit"
	"github.com/vaultviewer/vaultviewer/internal/storage/local"
)

func TestSplitFrontmatter(t *testing.T) {
	fm, body := splitFrontmatter("---\ntitle: X\ntype: note\n---\n\nbody text")
	if fm != "title: X\ntype: note" {
		t.Errorf("unexpected frontmatter: %q", fm)
	}
	if body != "\nbody text" {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestSplitFrontmatterNoneFound(t *testing.T) {
	fm, body := splitFrontmatter("# just a note\nno frontmatter here")
	if fm != "" {
		t.Errorf("expected no frontmatter, got %q", fm)
	}
	if body != "# just a note\nno frontmatter here" {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestExtractType(t *testing.T) {
	got := extractType("title: X\ntype: component\ntags:\n  - infra")
	if got == nil || *got != "component" {
		t.Fatalf("expected type=component, got %v", got)
	}
}

func TestExtractTypeQuoted(t *testing.T) {
	got := extractType(`type: "component"`)
	if got == nil || *got != "component" {
		t.Fatalf("expected quotes stripped, got %v", got)
	}
}

func TestExtractTypeAbsent(t *testing.T) {
	if got := extractType("title: X"); got != nil {
		t.Fatalf("expected nil, got %v", *got)
	}
}

func TestExtractFrontmatterRelationsAllThreeShapes(t *testing.T) {
	fm := `title: Trino
type: component
tags: [infra]
depends_on:
  - "[[01-예제/HMS-메타스토어]]"
  - "[[01-예제/Storage-Ceph]]"
queried_by: "[[Airflow]]"`

	rels := extractFrontmatterRelations(fm)
	if len(rels) != 2 {
		t.Fatalf("expected 2 relations, got %d: %+v", len(rels), rels)
	}
	if rels[0].relation != "depends_on" || len(rels[0].targets) != 2 {
		t.Fatalf("unexpected depends_on relation: %+v", rels[0])
	}
	if rels[0].targets[0] != "01-예제/HMS-메타스토어" || rels[0].targets[1] != "01-예제/Storage-Ceph" {
		t.Fatalf("unexpected depends_on targets: %+v", rels[0].targets)
	}
	if rels[1].relation != "queried_by" || len(rels[1].targets) != 1 || rels[1].targets[0] != "Airflow" {
		t.Fatalf("unexpected queried_by relation: %+v", rels[1])
	}
}

func TestExtractFrontmatterRelationsSkipsReservedKeys(t *testing.T) {
	fm := "title: X\ntype: note\ntags:\n  - a\n  - b"
	if rels := extractFrontmatterRelations(fm); len(rels) != 0 {
		t.Fatalf("expected no relations from reserved keys, got %+v", rels)
	}
}

func TestExtractWikilinkTargetsSkipsCodeAndImages(t *testing.T) {
	body := "See [[Real Note]] and also `[[Not This]]` and:\n```\n[[Also Not This]]\n```\n![[picture.png]]"
	targets := extractWikilinkTargets(body)
	if len(targets) != 1 || targets[0] != "Real Note" {
		t.Fatalf("expected only [[Real Note]], got %+v", targets)
	}
}

func TestResolveLinkTarget(t *testing.T) {
	cases := []struct{ target, dir, want string }{
		{"두번째-노트", "01-예제", "01-예제/두번째-노트.md"},
		{"01-예제/두번째-노트", "", "01-예제/두번째-노트.md"},
		{"이미지.png", "attachments", "attachments/이미지.png"},
		{"두번째-노트", "", "두번째-노트.md"},
	}
	for _, c := range cases {
		got := resolveLinkTarget(c.target, c.dir)
		if got != c.want {
			t.Errorf("resolveLinkTarget(%q, %q) = %q, want %q", c.target, c.dir, got, c.want)
		}
	}
}

func TestBuildProducesNoDuplicateEdgeForRelationTargets(t *testing.T) {
	// The exact bug fixed in web/src/lib/vaultIndex.ts (0.1.33): a
	// [[wikilink]] inside a frontmatter relation field must not also be
	// picked up as a second, untyped edge from scanning the whole file.
	root := t.TempDir()
	audioRecorder := audit.NewMemoryRecorder()
	eng, err := local.New(root, audioRecorder)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	if err := eng.Save("01-예제/HMS.md", []byte("---\ntitle: HMS\ntype: component\ndepends_on:\n  - \"[[01-예제/Storage]]\"\n---\n\n바디에는 링크 없음."), "alice", ""); err != nil {
		t.Fatalf("Save HMS: %v", err)
	}
	if err := eng.Save("01-예제/Storage.md", []byte("---\ntitle: Storage\ntype: component\n---\n\n스토리지."), "alice", ""); err != nil {
		t.Fatalf("Save Storage: %v", err)
	}

	graph, err := Build(eng)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("expected exactly 1 edge (no duplicate), got %d: %+v", len(graph.Edges), graph.Edges)
	}
	e := graph.Edges[0]
	if e.Source != "01-예제/HMS.md" || e.Target != "01-예제/Storage.md" || e.Relation == nil || *e.Relation != "depends_on" {
		t.Fatalf("unexpected edge: %+v", e)
	}
}

func TestBuildMarksUnresolvedTargetAsPhantomNode(t *testing.T) {
	root := t.TempDir()
	eng, err := local.New(root, audit.NewMemoryRecorder())
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	if err := eng.Save("note.md", []byte("본문에 [[아직-없는-노트]] 링크."), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	graph, err := Build(eng)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var phantom *Node
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == "아직-없는-노트.md" {
			phantom = &graph.Nodes[i]
		}
	}
	if phantom == nil {
		t.Fatalf("expected a phantom node for the unresolved target, got nodes: %+v", graph.Nodes)
	}
	if phantom.Resolved {
		t.Errorf("expected phantom node to have Resolved=false")
	}
}

func TestBuildEmptyVault(t *testing.T) {
	root := t.TempDir()
	eng, err := local.New(root, audit.NewMemoryRecorder())
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	graph, err := Build(eng)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Fatalf("expected an empty graph, got %+v", graph)
	}
}
