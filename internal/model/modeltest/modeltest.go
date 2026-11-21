// Package modeltest builds small, well-formed profiles and journeys for tests. The content
// is placeholder text: it describes no real person or job.
package modeltest

import (
	"fmt"

	"github.com/andriantam/ace-mate/internal/model"
)

// Profile returns a minimal valid profile.
func Profile() model.Profile {
	return model.Profile{
		Headline:   "Placeholder headline",
		WorkRights: []string{},
		Stacks:     []model.Stack{{Name: "Skill A", Depth: "strong", Years: 3, LastUsed: 2026}, {Name: "Skill B", Depth: "basic", Years: 1, LastUsed: 2024}},
		Industries: []string{"Industry A"},
		Flags:      []model.Flag{},
		Experience: []model.Role{},
	}
}

// LaneSizes is how many nodes Journey puts in each lane.
var LaneSizes = map[string]int{"tech": 5, "industry": 4, "people": 4, "hr": 4}

// Journey returns a complete journey: every lane has nodes, the first node of each lane is
// a milestone, nodes link to the same position in the next lane, and every node has tasks.
func Journey() model.Journey {
	prefix := map[string]string{"tech": "t", "industry": "i", "people": "p", "hr": "h"}
	var nodes []model.Node
	for li, a := range model.Aspects {
		next := model.Aspects[(li+1)%len(model.Aspects)].Key
		for k := 1; k <= LaneSizes[a.Key]; k++ {
			id := fmt.Sprintf("%s%d", prefix[a.Key], k)
			n := model.Node{
				ID: id, Aspect: a.Key, Title: "Milestone " + id, Baseline: 0.1,
				Links: []string{fmt.Sprintf("%s%d", prefix[next], 1)}, Milestone: k == 1,
				Brief: "Placeholder brief for " + id, Bridge: []model.Bridge{}, Sections: []model.Section{},
				Tasks: []string{"Task one", "Task two"}, Test: model.Test{Kind: "none", Quiz: []model.QuizItem{}, Rubric: []string{}},
				Project: model.Project{Milestones: []string{}}, HRQuestions: []model.HRQuestion{}, Reading: []model.Reading{},
			}
			nodes = append(nodes, n)
		}
	}
	return model.Journey{
		Meta:  model.JobMeta{Title: "Placeholder role", Company: "Placeholder company", MustHave: []string{}, NiceToHave: []string{}},
		Fit:   model.Fit{Match: 50, Strengths: []string{}, Gaps: []string{}, Risks: []string{}},
		Nodes: nodes,
	}
}
