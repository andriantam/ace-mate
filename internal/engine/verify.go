package engine

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/andriantam/ace-mate/internal/model"
)

// Verification reports how the resume was read and how well the readings agree.
// It is shown to the user right after upload and is not stored.
type Verification struct {
	Methods    []string `json:"methods"`    // "text", "visual" or "pdf"
	Agreement  int      `json:"agreement"`  // 0-100 share of skills both readings found; -1 when only one reading ran
	Confirmed  []string `json:"confirmed"`  // skills both readings found
	OnlyText   []string `json:"onlyText"`   // kept: only the text reading found them, and they appear in the resume text
	OnlyVisual []string `json:"onlyVisual"` // kept: only the visual reading found them
	Dropped    []string `json:"dropped"`    // removed: one reading found them, they are not in the resume text, and the other did not see them
	Mismatches []string `json:"mismatches"` // fields where the readings disagree
	Notes      []string `json:"notes"`
}

// Reconcile merges a text reading and a visual (OCR-style) reading of the same resume.
//
// The text reading is the base because it has the exact characters. Two skills are the
// same when one name's words are all contained in the other's, so "Docker" matches
// "docker-compose" and "CDP" matches "Customer Data Platform (CDP)". A skill is kept when
// both readings found it, or when its name appears in the resume text. A skill that only
// one reading found and that the text does not contain is dropped, which removes
// hallucinated skills and OCR misreads.
func Reconcile(text, visual *model.Profile, rawText string) (model.Profile, Verification) {
	v := Verification{Methods: []string{"text", "visual"}, Confirmed: []string{}, OnlyText: []string{}, OnlyVisual: []string{}, Dropped: []string{}, Mismatches: []string{}, Notes: []string{}}
	out := *text
	out.Stacks = nil

	vUsed := make([]bool, len(visual.Stacks))
	matchedText, matchedVisual := 0, 0
	for _, ts := range text.Stacks {
		matched := false
		for j, vs := range visual.Stacks {
			if !sameSkill(ts.Name, vs.Name) {
				continue
			}
			if !matched {
				matched = true
				compareStack(&v, ts, vs)
			}
			if !vUsed[j] {
				vUsed[j] = true
				matchedVisual++
			}
		}
		switch {
		case matched:
			matchedText++
			out.Stacks = append(out.Stacks, ts)
			v.Confirmed = append(v.Confirmed, ts.Name)
		case mentions(rawText, ts.Name):
			out.Stacks = append(out.Stacks, ts)
			v.OnlyText = append(v.OnlyText, ts.Name)
		default:
			v.Dropped = append(v.Dropped, ts.Name+" (not in the resume text, and the visual reading did not see it)")
		}
	}
	for j, vs := range visual.Stacks {
		if vUsed[j] {
			continue
		}
		if mentions(rawText, vs.Name) {
			out.Stacks = append(out.Stacks, vs)
			v.OnlyVisual = append(v.OnlyVisual, vs.Name)
		} else {
			v.Dropped = append(v.Dropped, vs.Name+" (only the visual reading found it, and it is not in the resume text)")
		}
	}

	v.Agreement = 100
	if total := len(text.Stacks) + len(visual.Stacks); total > 0 {
		v.Agreement = int(math.Round(100 * float64(matchedText+matchedVisual) / float64(total)))
	}

	out.Location = text.Location
	if !looselyEqual(text.Location, visual.Location) {
		v.Mismatches = append(v.Mismatches, fmt.Sprintf("Location: %q in the text reading, %q in the visual reading (kept the text value)", text.Location, visual.Location))
	} else if len(visual.Location) > len(text.Location) {
		out.Location = visual.Location // same place, more detail
	}
	out.WorkRights = mergeLoose(text.WorkRights, visual.WorkRights, &v, "Work rights")
	out.Industries = unionStrings(text.Industries, visual.Industries)
	if len(out.Experience) == 0 {
		out.Experience = visual.Experience // a scan-like layout can hide roles from the text layer
	}

	if v.Agreement < 60 {
		v.Notes = append(v.Notes, "The two readings differ a lot. Check your profile carefully before using it.")
	}
	if len(out.Stacks) == 0 {
		v.Notes = append(v.Notes, "No skill survived the cross-check.")
	}
	return out, v
}

// compareStack reports disagreements for one skill. Years and depth are the model's
// estimates (resumes rarely state them), so only large differences are worth a user's attention.
func compareStack(v *Verification, ts, vs model.Stack) {
	if skillKey(ts.Name) != skillKey(vs.Name) {
		return // different granularity (Docker vs docker-compose): the numbers are not comparable
	}
	if math.Abs(ts.Years-vs.Years) >= 4 {
		v.Mismatches = append(v.Mismatches, fmt.Sprintf("%s: about %.0f years in the text reading, %.0f in the visual reading (kept the text value)", ts.Name, ts.Years, vs.Years))
	}
	if abs(depthRank(ts.Depth)-depthRank(vs.Depth)) >= 2 {
		v.Mismatches = append(v.Mismatches, fmt.Sprintf("%s: depth %q in the text reading, %q in the visual reading (kept the text value)", ts.Name, ts.Depth, vs.Depth))
	}
}

func depthRank(d string) int {
	switch d {
	case "strong":
		return 3
	case "working":
		return 2
	case "basic":
		return 1
	}
	return 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Single describes a resume read only one way. rawText is empty when only the visual reading ran.
func Single(p model.Profile, method, rawText string, notes ...string) (model.Profile, Verification) {
	v := Verification{Methods: []string{method}, Agreement: -1, Confirmed: []string{}, OnlyText: []string{}, OnlyVisual: []string{}, Dropped: []string{}, Mismatches: []string{}, Notes: append([]string{}, notes...)}
	if rawText != "" {
		var missing []string
		for _, s := range p.Stacks {
			if !mentions(rawText, s.Name) {
				missing = append(missing, s.Name)
			}
		}
		if len(missing) > 0 {
			v.Notes = append(v.Notes, "Not found word-for-word in the resume text (may be inferred): "+strings.Join(missing, ", "))
		}
	}
	return p, v
}

// ── skill names ──

var aliases = map[string]string{
	"golang": "go", "k8s": "kubernetes", "postgres": "postgresql", "psql": "postgresql",
	"js": "javascript", "ts": "typescript", "amazonwebservices": "aws",
	"googlecloud": "gcp", "googlecloudplatform": "gcp", "mongo": "mongodb",
	"dotnet": "net", "csharp": "c#",
}

// skillKey normalises a skill name so "Node.js", "NodeJS" and "node" compare equal.
func skillKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '#' {
			b.WriteRune(r)
		}
	}
	k := b.String()
	if len(k) > 4 && strings.HasSuffix(k, "js") {
		k = strings.TrimSuffix(k, "js")
	}
	if a, ok := aliases[k]; ok {
		return a
	}
	return k
}

var genericWords = map[string]bool{
	"integration": true, "integrations": true, "development": true, "framework": true,
	"and": true, "the": true, "of": true, "with": true, "for": true,
}

// words splits a name into normalised words ("docker-compose" gives docker, compose).
func words(name string) []string {
	var cur strings.Builder
	var out []string
	flush := func() {
		if cur.Len() > 0 {
			w := skillKey(cur.String())
			if w != "" && !genericWords[w] {
				out = append(out, w)
			}
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '#' || r == '.' {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func subset(a, b []string) bool {
	if len(a) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, w := range b {
		set[w] = true
	}
	for _, w := range a {
		if !set[w] {
			return false
		}
	}
	return true
}

// sameSkill: every word of one name appears in the other ("Claude" and "Claude/MCP integrations",
// "CDP" and "Customer Data Platform (CDP)"). "Java" and "JavaScript" stay different.
func sameSkill(a, b string) bool {
	wa, wb := words(a), words(b)
	return subset(wa, wb) || subset(wb, wa)
}

func looselyEqual(a, b string) bool {
	if norm(a) == norm(b) {
		return true
	}
	return sameSkill(a, b)
}

// mergeLoose merges two lists of free-text items, treating an item and a more detailed
// version of it ("Malaysia" and "Malaysia (Permanent Resident)") as the same, and keeping the detailed one.
func mergeLoose(text, visual []string, v *Verification, label string) []string {
	out := []string{}
	used := make([]bool, len(visual))
	for _, t := range text {
		best := t
		found := false
		for j, x := range visual {
			if looselyEqual(t, x) {
				found, used[j] = true, true
				if len(x) > len(best) {
					best = x
				}
			}
		}
		if !found {
			v.Mismatches = append(v.Mismatches, fmt.Sprintf("%s: %q was found by the text reading only", label, t))
		}
		out = append(out, best)
	}
	for j, x := range visual {
		if !used[j] {
			v.Mismatches = append(v.Mismatches, fmt.Sprintf("%s: %q was found by the visual reading only (not added)", label, x))
		}
	}
	return out
}

// mentions reports whether the raw text contains the skill (or a known alias) as a whole word.
// Compound names such as "Claude/MCP integrations" count when every part is present.
func mentions(raw, name string) bool {
	if mentionsOne(raw, name) {
		return true
	}
	// Parenthetical short form: "Customer Data Platform (CDP)".
	if i, j := strings.Index(name, "("), strings.LastIndex(name, ")"); i >= 0 && j > i {
		if mentionsOne(raw, strings.TrimSpace(name[:i])) || mentionsOne(raw, name[i+1:j]) {
			return true
		}
	}
	// Every word present: "docker-compose" when the text says "Docker Compose", "Claude/MCP integrations".
	ws := words(name)
	if len(ws) < 2 {
		return false
	}
	for _, w := range ws {
		if !mentionsOne(raw, w) {
			return false
		}
	}
	return true
}

func mentionsOne(raw, name string) bool {
	low := strings.ToLower(raw)
	want := skillKey(name)
	cands := map[string]bool{strings.ToLower(strings.TrimSpace(name)): true, want: true}
	for alias, canon := range aliases {
		if canon == want {
			cands[alias] = true
		}
	}
	if len(want) > 0 && want != strings.ToLower(strings.TrimSpace(name)) {
		cands[want+".js"] = true
		cands[want+"js"] = true
	}
	for c := range cands {
		if c == "" {
			continue
		}
		re := regexp.MustCompile(`(?:^|[^a-z0-9+#])` + regexp.QuoteMeta(c) + `(?:$|[^a-z0-9+#])`)
		if re.MatchString(low) {
			return true
		}
		// PDF text extraction often glues neighbouring words together ("ExampleCoNext.js"), which
		// defeats a whole-word match. Names of five or more characters are distinctive enough to
		// match inside a run of text. Short names ("Go", "Java") keep the whole-word rule.
		if len(c) >= 5 && strings.Contains(low, c) {
			return true
		}
	}
	return false
}

func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range append(append([]string{}, a...), b...) {
		if k := norm(s); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}
