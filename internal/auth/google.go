package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// birthdayScope is a Google "sensitive" scope. Requesting it in production needs Google's
// OAuth app verification, and users see it as a separate permission they can refuse.
const birthdayScope = "https://www.googleapis.com/auth/user.birthday.read"

const peopleURL = "https://people.googleapis.com/v1/people/me?personFields=birthdays"

type googleDate struct {
	Year, Month, Day int
}

// googleBirthday reads the user's birthday from the People API with the access token,
// which is used for this one call and then discarded. It returns "YYYY-MM-DD", or
// "--MM-DD" when the user shares no year, or "" when there is none or it was refused.
func googleBirthday(ctx context.Context, ts oauth2.TokenSource, client *http.Client) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, peopleURL, nil)
	if err != nil {
		return "", err
	}
	tok, err := ts.Token()
	if err != nil {
		return "", err
	}
	tok.SetAuthHeader(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return "", nil // the user didn't grant the scope
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("people api: status %d", resp.StatusCode)
	}
	var body struct {
		Birthdays []struct {
			Metadata struct {
				Primary bool `json:"primary"`
			} `json:"metadata"`
			Date *googleDate `json:"date"`
		} `json:"birthdays"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	var best *googleDate
	for _, b := range body.Birthdays {
		if b.Date == nil || b.Date.Month < 1 || b.Date.Month > 12 || b.Date.Day < 1 || b.Date.Day > 31 {
			continue
		}
		if best == nil || b.Metadata.Primary || (best.Year == 0 && b.Date.Year != 0) {
			best = b.Date
		}
	}
	return formatBirthday(best), nil
}

func formatBirthday(d *googleDate) string {
	if d == nil {
		return ""
	}
	if d.Year >= 1900 && d.Year <= time.Now().Year() {
		return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
	}
	return fmt.Sprintf("--%02d-%02d", d.Month, d.Day)
}
