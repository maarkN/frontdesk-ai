package domain

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FAQ is one question/answer pair the agent may use to ground replies.
type FAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// OnboardingProfile is the "import from my website" result: business facts
// and FAQ extracted (by an LLM in production) for review before activation
// (RF4, onboarding self-service in under 10 minutes).
type OnboardingProfile struct {
	WebsiteURL   string    `json:"websiteUrl"`
	BusinessName string    `json:"businessName,omitempty"`
	Services     []string  `json:"services,omitempty"`
	FAQs         []FAQ     `json:"faqs,omitempty"`
	ExtractedAt  time.Time `json:"extractedAt"`
}

// SiteExtractor extracts an OnboardingProfile from a business website. The
// production implementation calls an LLM; the MVP ships a deterministic fake
// so onboarding and its tests never touch the network.
type SiteExtractor interface {
	ExtractProfile(ctx context.Context, websiteURL string) (OnboardingProfile, error)
}

// StaticExtractor is the MVP fake: it derives a plausible profile from the
// URL alone, deterministically and offline.
type StaticExtractor struct {
	// Now stamps ExtractedAt; time.Now when nil (zero value is usable).
	Now func() time.Time
}

var _ SiteExtractor = (*StaticExtractor)(nil)

// ExtractProfile returns a canned trade-business profile named after the
// website host.
func (e *StaticExtractor) ExtractProfile(_ context.Context, websiteURL string) (OnboardingProfile, error) {
	if websiteURL == "" {
		return OnboardingProfile{}, fmt.Errorf("extract profile: empty website url")
	}
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	return OnboardingProfile{
		WebsiteURL:   websiteURL,
		BusinessName: hostOf(websiteURL),
		Services:     []string{"plumbing repair", "water heater installation", "drain cleaning"},
		FAQs: []FAQ{
			{Question: "Do you serve my area?", Answer: "We serve the greater metro area; share your postal code and we will confirm."},
			{Question: "Est-ce que vous offrez un service d'urgence?", Answer: "Oui, un service d'urgence 24/7 est offert pour les fuites majeures."},
		},
		ExtractedAt: now().UTC(),
	}, nil
}

// hostOf extracts a display name from a URL without touching the network.
func hostOf(rawURL string) string {
	host := rawURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimPrefix(host, "www.")
}
