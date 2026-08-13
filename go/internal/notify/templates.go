package notify

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/maarkn/frontdesk/internal/event"
)

// templateFS embeds the EN and FR-CA message templates. One file per
// (message, locale) pair, named "<base>.<locale>.tmpl"; text/template names
// each parsed template after its base filename.
//
//go:embed templates/*.tmpl
var templateFS embed.FS

// loadTemplates parses every embedded template into one set.
func loadTemplates() (*template.Template, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse notification templates: %w", err)
	}
	return tmpl, nil
}

// templateName maps a message base name and locale to the template file
// name. Anything that is not fr-CA falls back to English.
func templateName(base string, loc event.Locale) string {
	if loc == event.LocaleFRCA {
		return base + ".fr-ca.tmpl"
	}
	return base + ".en.tmpl"
}

// ownerSMSData feeds owner_sms.*.tmpl.
type ownerSMSData struct {
	Business   string
	CallerName string
	Service    string
	Urgency    string
	Phone      string
	Booked     bool
	When       string
	Message    string
	URL        string
}

// clientSMSData feeds client_sms.*.tmpl.
type clientSMSData struct {
	Business string
	Booked   bool
	When     string
}

// weeklyData feeds weekly_email.*.tmpl. The template's first output line is
// the e-mail subject; the rest is the body.
type weeklyData struct {
	Business       string
	CallsAnswered  int
	JobsBooked     int
	Messages       int
	EstimatedValue float64
}

// render executes one named template and trims surrounding whitespace.
func render(tmpl *template.Template, name string, data any) (string, error) {
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// urgencyLabel localizes the urgency variable for owner-facing text.
// Unknown values pass through verbatim.
func urgencyLabel(loc event.Locale, urgency string) string {
	fr := loc == event.LocaleFRCA
	switch urgency {
	case "emergency":
		if fr {
			return "urgence"
		}
		return "emergency"
	case "high":
		if fr {
			return "élevée"
		}
		return "high"
	case "", "normal":
		if fr {
			return "normale"
		}
		return "normal"
	default:
		return urgency
	}
}

// formatWhen renders an appointment time in the tenant's timezone, using
// locale-appropriate conventions.
func formatWhen(t time.Time, loc event.Locale, tz *time.Location) string {
	t = t.In(tz)
	if loc == event.LocaleFRCA {
		return t.Format("02/01/2006 à 15h04")
	}
	return t.Format("Jan 2, 2006 at 3:04 PM")
}

// loadLocation resolves an IANA timezone name, falling back to UTC.
func loadLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}
