// Package sources turns a website link into text the AI can read: GitHub profiles and
// repositories through GitHub's public API, and any other page as plain text. All
// requests go through safefetch, so users can't make the server reach internal hosts.
package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/andriantam/ace-mate/internal/safefetch"
)

// Page is what was read from a link.
type Page struct {
	Title string // a short label for the document list
	Text  string
	Note  string // what kind of source this is, for the AI prompt
}

var (
	ghName   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	ghReserv = map[string]bool{"orgs": true, "features": true, "topics": true, "collections": true, "explore": true,
		"marketplace": true, "settings": true, "login": true, "about": true, "pricing": true, "sponsors": true, "search": true}
)

// Read fetches the link and returns readable text.
func Read(ctx context.Context, raw string) (Page, error) {
	u, err := safefetch.Validate(raw)
	if err != nil {
		return Page{}, err
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == "github.com" {
		parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		switch {
		case len(parts) == 1 && ghName.MatchString(parts[0]) && !ghReserv[strings.ToLower(parts[0])]:
			return githubUser(ctx, parts[0])
		case len(parts) >= 2 && ghName.MatchString(parts[0]) && !ghReserv[strings.ToLower(parts[0])]:
			return githubRepo(ctx, parts[0], strings.TrimSuffix(parts[1], ".git"))
		}
	}
	return webPage(ctx, u)
}

func webPage(ctx context.Context, u *url.URL) (Page, error) {
	body, ct, err := safefetch.Get(ctx, u.String(), "text/html,text/plain;q=0.9")
	if err != nil {
		return Page{}, err
	}
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "text/plain") {
		return Page{}, &safefetch.ErrUser{Msg: "That link isn't a web page."}
	}
	text := string(body)
	title := u.Hostname() + strings.TrimSuffix(u.EscapedPath(), "/")
	if strings.Contains(ct, "html") {
		if m := titleRe.FindStringSubmatch(text); m != nil {
			if t := strings.TrimSpace(safefetch.HTMLToText(m[1])); t != "" && len(t) < 120 {
				title = t
			}
		}
		text = safefetch.HTMLToText(text)
	}
	if len(text) < 200 {
		return Page{}, &safefetch.ErrUser{Msg: "That page had almost no readable text (it may need JavaScript to load). Try another link, or paste the text."}
	}
	if len(text) > 40_000 {
		text = text[:40_000]
	}
	return Page{Title: title, Text: text, Note: "This is a public web page about the person (for example a personal site, portfolio or blog)."}, nil
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// ── GitHub ──

type ghUser struct {
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	Bio         string    `json:"bio"`
	Company     string    `json:"company"`
	Blog        string    `json:"blog"`
	PublicRepos int       `json:"public_repos"`
	Followers   int       `json:"followers"`
	CreatedAt   time.Time `json:"created_at"`
}

type ghRepo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Language    string    `json:"language"`
	Topics      []string  `json:"topics"`
	Stars       int       `json:"stargazers_count"`
	Fork        bool      `json:"fork"`
	Archived    bool      `json:"archived"`
	CreatedAt   time.Time `json:"created_at"`
	PushedAt    time.Time `json:"pushed_at"`
}

func getJSON(ctx context.Context, raw string, v any) error {
	body, _, err := safefetch.Get(ctx, raw, "application/vnd.github+json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return &safefetch.ErrUser{Msg: "GitHub returned something unexpected."}
	}
	return nil
}

func githubUser(ctx context.Context, login string) (Page, error) {
	var u ghUser
	if err := getJSON(ctx, "https://api.github.com/users/"+url.PathEscape(login), &u); err != nil {
		return Page{}, ghErr(err)
	}
	var repos []ghRepo
	if err := getJSON(ctx, "https://api.github.com/users/"+url.PathEscape(login)+"/repos?sort=pushed&per_page=40&type=owner", &repos); err != nil {
		return Page{}, ghErr(err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "GitHub profile %s", u.Login)
	if u.Name != "" {
		fmt.Fprintf(&sb, " (%s)", u.Name)
	}
	fmt.Fprintf(&sb, ". Member since %d. %d public repositories, %d followers.\n", u.CreatedAt.Year(), u.PublicRepos, u.Followers)
	if u.Bio != "" {
		fmt.Fprintf(&sb, "Bio: %s\n", u.Bio)
	}
	if u.Company != "" {
		fmt.Fprintf(&sb, "Company: %s\n", u.Company)
	}
	langs := map[string]int{}
	sb.WriteString("\nRepositories (own, most recently pushed first):\n")
	n := 0
	for _, r := range repos {
		if r.Fork {
			continue
		}
		if r.Language != "" {
			langs[r.Language]++
		}
		if n < 30 {
			fmt.Fprintf(&sb, "- %s: %s [language: %s; topics: %s; stars: %d; created %d; last push %s%s]\n",
				r.Name, orNone(r.Description), orNone(r.Language), orNone(strings.Join(r.Topics, ", ")), r.Stars,
				r.CreatedAt.Year(), r.PushedAt.Format("2006-01"), map[bool]string{true: "; archived", false: ""}[r.Archived])
			n++
		}
	}
	if n == 0 {
		sb.WriteString("(no public repositories of their own)\n")
	}
	if len(langs) > 0 {
		type kv struct {
			k string
			v int
		}
		var ls []kv
		for k, v := range langs {
			ls = append(ls, kv{k, v})
		}
		sort.Slice(ls, func(i, j int) bool { return ls[i].v > ls[j].v })
		var parts []string
		for _, x := range ls {
			parts = append(parts, fmt.Sprintf("%s (%d repos)", x.k, x.v))
		}
		fmt.Fprintf(&sb, "\nLanguages across own repositories: %s\n", strings.Join(parts, ", "))
	}
	return Page{
		Title: "github.com/" + u.Login,
		Text:  sb.String(),
		Note:  "This is the person's public GitHub profile, read from GitHub's API. Judge skills from the repositories' languages, topics, descriptions and recency. Treat each substantial repository as a project in experience, dated by its created and last-push dates.",
	}, nil
}

func githubRepo(ctx context.Context, owner, repo string) (Page, error) {
	var r ghRepo
	if err := getJSON(ctx, "https://api.github.com/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo), &r); err != nil {
		return Page{}, ghErr(err)
	}
	langs := map[string]int{}
	_ = getJSON(ctx, "https://api.github.com/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo)+"/languages", &langs)
	var sb strings.Builder
	fmt.Fprintf(&sb, "GitHub repository %s/%s: %s\n", owner, r.Name, orNone(r.Description))
	fmt.Fprintf(&sb, "Main language: %s. Topics: %s. Stars: %d. Created %s, last push %s.\n",
		orNone(r.Language), orNone(strings.Join(r.Topics, ", ")), r.Stars, r.CreatedAt.Format("2006-01"), r.PushedAt.Format("2006-01"))
	if len(langs) > 0 {
		var parts []string
		for k := range langs {
			parts = append(parts, k)
		}
		sort.Strings(parts)
		fmt.Fprintf(&sb, "Languages used: %s\n", strings.Join(parts, ", "))
	}
	return Page{
		Title: "github.com/" + owner + "/" + r.Name,
		Text:  sb.String(),
		Note:  "This is one of the person's public GitHub repositories. Add it to experience as a project and judge skills from its languages and topics.",
	}, nil
}

func ghErr(err error) error {
	if eu, ok := err.(*safefetch.ErrUser); ok && strings.Contains(eu.Msg, "403") {
		return &safefetch.ErrUser{Msg: "GitHub's rate limit was reached. Try again in an hour."}
	}
	return err
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return s
}
