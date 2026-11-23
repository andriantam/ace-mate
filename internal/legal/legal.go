// Package legal holds the version of the Terms of Service and Privacy Policy.
//
// The documents are pages in the UI (web/src/pages/legal), filled with the operator's
// details from the LEGAL_* settings. They describe AceMate as it ships. If you run a
// public copy, review them with a lawyer, change them to match how you run it, and bump
// Version: see docs/legal-checklist.md.
package legal

// Version is the current Terms and Privacy Policy version. Change it (and Effective)
// whenever either document changes materially: every user is then asked to accept again.
const Version = "3"

// Effective is the date the current version took effect (YYYY-MM-DD).
const Effective = "2026-11-23"
