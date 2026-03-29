package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/biter777/countries"
	useragent "github.com/mileusna/useragent"
	"github.com/johnvanham/bw-monitor/internal/redis"
	"github.com/johnvanham/bw-monitor/internal/ui"
)

// BuildDetailContent builds the full content string for a report detail view.
// This content is set on the detail viewport which handles scrolling natively.
func BuildDetailContent(r *redis.BlockReport, width int, dnsNames []string, dnsLoading bool) string {
	var lines []string

	add := func(s string) {
		lines = append(lines, s)
	}

	field := func(label, value string) {
		add(ui.LabelStyle.Render(label) + ui.ValueStyle.Render(value))
	}

	add("")
	field("Request ID:", r.ID)
	field("Date/Time:", r.Time().Format(time.RFC3339))
	field("IP Address:", r.IP)

	if dnsLoading {
		field("rDNS:", "(looking up...)")
	} else if len(dnsNames) > 0 {
		field("rDNS:", strings.Join(dnsNames, ", "))
	}

	if c := countries.ByName(r.Country); c != countries.Unknown {
		field("Country:", fmt.Sprintf("%s (%s)", c.String(), r.Country))
	} else {
		field("Country:", r.Country)
	}
	field("Method:", r.Method)
	field("URL:", r.URL)
	field("Status:", fmt.Sprintf("%d", r.Status))
	field("Reason:", r.Reason)
	field("Server:", r.ServerName)
	field("Security Mode:", r.SecurityMode)
	field("User Agent:", r.UserAgent)

	// Parsed user agent info
	ua := useragent.Parse(r.UserAgent)
	var uaParts []string
	if ua.Name != "" {
		v := ua.Name
		if ua.Version != "" {
			v += " " + ua.Version
		}
		uaParts = append(uaParts, v)
	}
	if ua.OS != "" {
		v := ua.OS
		if ua.OSVersion != "" {
			v += " " + ua.OSVersion
		}
		uaParts = append(uaParts, v)
	}
	if ua.Device != "" {
		uaParts = append(uaParts, ua.Device)
	}
	if ua.Mobile {
		uaParts = append(uaParts, "Mobile")
	} else if ua.Tablet {
		uaParts = append(uaParts, "Tablet")
	} else if ua.Desktop {
		uaParts = append(uaParts, "Desktop")
	}
	if ua.Bot {
		uaParts = append(uaParts, "Bot")
	}
	if len(uaParts) > 0 {
		field("Parsed UA:", strings.Join(uaParts, " / "))
	}

	if len(r.BadBehaviorDetails) > 0 {
		add("")
		add(ui.TitleStyle.Render("  Bad Behavior History"))
		add("")

		for i, d := range r.BadBehaviorDetails {
			add(ui.LabelStyle.Render(fmt.Sprintf("  Event %d:", i+1)))

			sec := int64(d.Date)
			nsec := int64((d.Date - float64(sec)) * 1e9)
			t := time.Unix(sec, nsec)

			indent := func(label, value string) {
				add("    " + ui.LabelStyle.Render(label) + ui.ValueStyle.Render(value))
			}

			indent("Date:", t.Format(time.RFC3339))
			indent("URL:", d.URL)
			indent("Method:", d.Method)
			indent("Status:", d.Status)
			indent("Ban Time:", fmt.Sprintf("%ds", d.BanTime))
			indent("Ban Scope:", d.BanScope)
			indent("Threshold:", fmt.Sprintf("%d", d.Threshold))
			indent("Count Time:", fmt.Sprintf("%ds", d.CountTime))
			add("")
		}
	}

	return strings.Join(lines, "\n")
}
