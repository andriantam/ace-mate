package engine

import (
	"testing"

	"github.com/andriantam/ace-mate/internal/model"
)

func TestMergeProfiles(t *testing.T) {
	resume := model.Profile{
		Headline: "Senior engineer", Location: "Singapore", WorkRights: []string{"SG"},
		Stacks:     []model.Stack{{Name: "Go", Depth: "working", Years: 3, LastUsed: 2024}, {Name: "Postgres", Depth: "strong", Years: 6, LastUsed: 2024}},
		Industries: []string{"Banking"},
		Flags:      []model.Flag{{Kind: "plus", Text: "Ships fast"}},
		Experience: []model.Role{{Title: "Engineer", Company: "Acme", Start: "2019-01", End: "2024-01", Skills: []string{"Go"}, Highlights: []string{"Built ledger"}}},
	}
	github := model.Profile{
		Headline:   "Open-source Go developer",
		Stacks:     []model.Stack{{Name: "Golang", Depth: "strong", Years: 5, LastUsed: 2026}, {Name: "Rust", Depth: "basic", Years: 1, LastUsed: 2026}},
		Industries: []string{"banking", "Developer tools"},
		Flags:      []model.Flag{{Kind: "plus", Text: "Ships fast"}},
		Experience: []model.Role{{Title: "Maintainer", Company: "go-ledger (open source)", Start: "2025", Skills: []string{"Go"}}},
	}
	p := MergeProfiles([]model.Profile{resume, github})
	if p.Headline != "Senior engineer" || p.Location != "Singapore" {
		t.Fatalf("the resume should win single fields: %+v", p)
	}
	if len(p.Stacks) != 3 {
		t.Fatalf("Go and Golang are one skill: %+v", p.Stacks)
	}
	if g := p.Stacks[0]; g.Name != "Go" || g.Depth != "strong" || g.Years != 5 || g.LastUsed != 2026 {
		t.Fatalf("the strongest evidence should win: %+v", g)
	}
	if len(p.Industries) != 2 || len(p.Flags) != 1 {
		t.Fatalf("industries=%v flags=%v", p.Industries, p.Flags)
	}
	if len(p.Experience) != 2 || p.Experience[0].Start != "2025" {
		t.Fatalf("roles should merge newest first: %+v", p.Experience)
	}
	if MergeProfiles(nil).Stacks == nil {
		t.Fatal("empty merge must still have non-nil slices")
	}
}
