package engine

import (
	"sort"
	"strings"

	"github.com/andriantam/ace-mate/internal/model"
)

// MergeProfiles combines the profiles extracted from several documents into one.
// sources must be in priority order (resumes before links, newest first): single-value
// fields such as the headline come from the first source that has them. Skills are
// matched across sources by name ("Go" and "Golang" are one skill) and keep the strongest
// evidence; roles are matched by company and title.
func MergeProfiles(sources []model.Profile) model.Profile {
	var out model.Profile
	for _, p := range sources {
		if out.DisplayName == "" {
			out.DisplayName = p.DisplayName
		}
		if out.Headline == "" {
			out.Headline = p.Headline
		}
		if out.Location == "" {
			out.Location = p.Location
		}
		if len(out.WorkRights) == 0 {
			out.WorkRights = p.WorkRights
		}
		out.Industries = unionStrings(out.Industries, p.Industries)
		for _, s := range p.Stacks {
			mergeStack(&out.Stacks, s)
		}
		for _, f := range p.Flags {
			if !hasFlag(out.Flags, f) && len(out.Flags) < 10 {
				out.Flags = append(out.Flags, f)
			}
		}
		for _, r := range p.Experience {
			mergeRole(&out.Experience, r)
		}
	}
	sort.SliceStable(out.Experience, func(i, j int) bool { return out.Experience[i].Start > out.Experience[j].Start })
	return normalizeProfile(out)
}

var depthOrder = map[string]int{"none": 0, "basic": 1, "working": 2, "strong": 3}

func mergeStack(list *[]model.Stack, s model.Stack) {
	for i := range *list {
		cur := &(*list)[i]
		if !sameSkill(cur.Name, s.Name) {
			continue
		}
		if depthOrder[s.Depth] > depthOrder[cur.Depth] {
			cur.Depth = s.Depth
		}
		cur.Years = max(cur.Years, s.Years)
		cur.LastUsed = max(cur.LastUsed, s.LastUsed)
		return
	}
	*list = append(*list, s)
}

func hasFlag(list []model.Flag, f model.Flag) bool {
	for _, x := range list {
		if norm(x.Text) == norm(f.Text) {
			return true
		}
	}
	return false
}

func mergeRole(list *[]model.Role, r model.Role) {
	for i := range *list {
		cur := &(*list)[i]
		if norm(cur.Company) != norm(r.Company) || !looselyEqual(cur.Title, r.Title) {
			continue
		}
		if len(r.Highlights) > len(cur.Highlights) {
			cur.Highlights = r.Highlights
		}
		cur.Skills = unionStrings(cur.Skills, r.Skills)
		if cur.Start == "" || (r.Start != "" && len(r.Start) > len(cur.Start)) {
			cur.Start = r.Start // prefer the more precise date
		}
		if cur.End == "" && r.End != "" && strings.TrimSpace(cur.Start) == "" {
			cur.End = r.End
		}
		if cur.Industry == "" {
			cur.Industry = r.Industry
		}
		return
	}
	*list = append(*list, r)
}
