// Package model defines the data shapes shared by the AI engine, the database and the UI.
// The structs double as the JSON Schema given to the LLM, so every field is required
// (use empty values instead of omitting) and nothing is a pointer.
package model

const (
	AspectTech     = "tech"
	AspectIndustry = "industry"
	AspectPeople   = "people"
	AspectHR       = "hr"
)

type Aspect struct {
	Key    string
	Label  string
	Weight float64
	Color  string
}

// Aspects in lane order.
var Aspects = []Aspect{
	{AspectTech, "Tech stack", 0.35, "var(--c-tech)"},
	{AspectIndustry, "Industry", 0.20, "var(--c-ind)"},
	{AspectPeople, "The people", 0.20, "var(--c-ppl)"},
	{AspectHR, "HR questions", 0.25, "var(--c-hr)"},
}

type Stack struct {
	Name     string  `json:"name"`
	Depth    string  `json:"depth" jsonschema:"enum=strong,enum=working,enum=basic,enum=none"`
	Years    float64 `json:"years"`
	LastUsed int     `json:"lastUsed" jsonschema:"description=Year last used hands-on or 0 if never"`
}

type Flag struct {
	Kind string `json:"kind" jsonschema:"enum=plus,enum=risk"`
	Text string `json:"text"`
}

// Role is one position in the person's career, used for the career timeline.
type Role struct {
	Title      string   `json:"title"`
	Company    string   `json:"company"`
	Start      string   `json:"start" jsonschema:"description=Start as YYYY-MM or YYYY, or empty if unknown"`
	End        string   `json:"end" jsonschema:"description=End as YYYY-MM or YYYY, or empty if this is the current role"`
	Industry   string   `json:"industry"`
	Skills     []string `json:"skills" jsonschema:"description=Skills used in this role, named as in stacks"`
	Highlights []string `json:"highlights" jsonschema:"description=Up to 4 short achievements, each one line"`
}

type Profile struct {
	DisplayName string   `json:"displayName" jsonschema:"description=First name only or empty"`
	Headline    string   `json:"headline" jsonschema:"description=One line: level and years and focus"`
	Location    string   `json:"location"`
	WorkRights  []string `json:"workRights"`
	Stacks      []Stack  `json:"stacks"`
	Industries  []string `json:"industries"`
	Flags       []Flag   `json:"flags" jsonschema:"description=Career-shape signals an interviewer will notice: gaps and pivots and leadership and speed"`
	Experience  []Role   `json:"experience" jsonschema:"description=Roles and projects with dates, most recent first. Empty if the source has none"`
}

type JobMeta struct {
	Title      string   `json:"title"`
	Company    string   `json:"company"`
	Band       string   `json:"band" jsonschema:"description=Grade or level if stated else empty"`
	Location   string   `json:"location"`
	WorkType   string   `json:"workType" jsonschema:"description=For example Hybrid and Permanent else empty"`
	Closes     string   `json:"closes" jsonschema:"description=Closing date if stated else empty"`
	Summary    string   `json:"summary"`
	MustHave   []string `json:"mustHave"`
	NiceToHave []string `json:"niceToHave"`
}

type Fit struct {
	Match     int      `json:"match" jsonschema:"description=0 to 100 baseline match of profile to job"`
	Strengths []string `json:"strengths"`
	Gaps      []string `json:"gaps"`
	Risks     []string `json:"risks"`
}

type Bridge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Section struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

type QuizItem struct {
	Q       string   `json:"q"`
	Options []string `json:"options"`
	Correct int      `json:"correct" jsonschema:"description=Index into options"`
	Why     string   `json:"why"`
}

type Test struct {
	Kind   string     `json:"kind" jsonschema:"enum=none,enum=quiz,enum=scenario"`
	Quiz   []QuizItem `json:"quiz"`
	Prompt string     `json:"prompt" jsonschema:"description=Scenario prompt for scenario tests else empty"`
	Rubric []string   `json:"rubric" jsonschema:"description=Scenario rubric points else empty"`
}

type Project struct {
	Repo       string   `json:"repo" jsonschema:"description=Suggested GitHub repo name or empty when this node has no project"`
	Stack      string   `json:"stack"`
	Milestones []string `json:"milestones"`
}

type HRQuestion struct {
	Q   string `json:"q"`
	Tip string `json:"tip"`
}

type Reading struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Node struct {
	ID          string       `json:"id"`
	Aspect      string       `json:"aspect" jsonschema:"enum=tech,enum=industry,enum=people,enum=hr"`
	Title       string       `json:"title"`
	Baseline    float64      `json:"baseline" jsonschema:"description=0 to 1 credit the resume already earns for this node"`
	Links       []string     `json:"links" jsonschema:"description=ids of related nodes in other lanes"`
	Milestone   bool         `json:"milestone"`
	Verify      bool         `json:"verify" jsonschema:"description=true if any fact here should be double-checked"`
	Brief       string       `json:"brief" jsonschema:"description=Second person and recruiter voice and why this matters for THIS job"`
	Bridge      []Bridge     `json:"bridge" jsonschema:"description=What the user already knows mapped to the new concept"`
	Sections    []Section    `json:"sections" jsonschema:"description=Extra lists such as Scenarios or Your gap-fill pitch or Ask them or Answer framework"`
	Tasks       []string     `json:"tasks"`
	Test        Test         `json:"test"`
	Project     Project      `json:"project"`
	HRQuestions []HRQuestion `json:"hrQuestions"`
	PeopleCards bool         `json:"peopleCards" jsonschema:"description=true if the user should fill person cards here"`
	Reading     []Reading    `json:"reading"`
}

type Journey struct {
	Meta  JobMeta `json:"meta"`
	Fit   Fit     `json:"fit"`
	Nodes []Node  `json:"nodes"`
	// While a journey is being written, Nodes holds placeholders (title and links only) for
	// the ids in Pending. Failed lists nodes that couldn't be written and can be retried.
	Pending []string `json:"pending,omitempty"`
	Failed  []string `json:"failed,omitempty"`
}

// Unwritten reports whether a node is still a placeholder.
func (j Journey) Unwritten(id string) bool {
	for _, x := range j.Pending {
		if x == id {
			return true
		}
	}
	for _, x := range j.Failed {
		if x == id {
			return true
		}
	}
	return false
}

// Written returns the journey without placeholder nodes (for scoring).
func (j Journey) Written() Journey {
	if len(j.Pending) == 0 && len(j.Failed) == 0 {
		return j
	}
	out := j
	out.Nodes = make([]Node, 0, len(j.Nodes))
	for _, n := range j.Nodes {
		if !j.Unwritten(n.ID) {
			out.Nodes = append(out.Nodes, n)
		}
	}
	return out
}

// Progress maps "<nodeID>:<taskIndex>" and "<nodeID>:test" to done.
type Progress map[string]bool

// People holds the user's person cards, keyed by node id.
type PersonCard struct {
	Name string `json:"name,omitempty"`
	Work string `json:"work,omitempty"`
	Fit  string `json:"fit,omitempty"`
}
type People map[string][]PersonCard
