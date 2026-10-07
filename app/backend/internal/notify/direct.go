package notify

import (
	"context"
	"strings"
)

// Direct is a message incident response composes itself (an escalation step, a war room
// update): the e-mail subject and text, the Telegram HTML and the links Teams shows as buttons.
type Direct struct {
	Subject, Text, HTML string
	Links               []DirectLink
	// AckID: each address also gets its own acknowledgement link for this incident.
	AckID string
	// AckTitle is the word of the acknowledgement link.
	AckTitle string
}

type DirectLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Outcome is what happened to one address of a direct message.
type Outcome struct {
	Channel string
	// Address is the raw address, Shown the one records may show (webhooks redacted).
	Address, Shown string
	Err            error
}

// Ready reports whether a channel is turned on and has what it needs to send.
func (s *Service) Ready(kind string) bool {
	ch := channelOf(kind)
	if ch == nil {
		return false
	}
	return ch.Ready(s.settings().set.Notify)
}

// SendDirect sends a message to addresses with the saved channel settings, each address once;
// addresses of channels that are off or not valid are reported with ErrDisabled or
// ErrUnknownChannel.
func (s *Service) SendDirect(ctx context.Context, m Direct, to []Target) []Outcome {
	c := s.config()
	base := strings.TrimRight(c.set.PublicURL, "/")
	links := s.Links()
	seen := map[string]bool{}
	var out []Outcome
	for _, t := range to {
		key := t.Channel + "|" + strings.ToLower(t.Address)
		if t.Address == "" || seen[key] {
			continue
		}
		seen[key] = true
		ch := channelOf(t.Channel)
		o := Outcome{Channel: t.Channel, Address: t.Address, Shown: ShowAddress(t.Channel, t.Address)}
		switch {
		case ch == nil:
			o.Err = ErrUnknownChannel
		case !ch.Enabled(c.set.Notify):
			o.Err = ErrDisabled
		case !ch.Valid(t.Address):
			o.Err = ErrUnknownChannel
		}
		if o.Err != nil {
			out = append(out, o)
			continue
		}
		msg := composed{subject: m.Subject, text: m.Text, html: m.HTML}
		var ack string
		if m.AckID != "" && base != "" && links != nil {
			ack = base + "/ack/" + links.Sign(m.AckID, ch.Recipient(t.Address), s.now().Add(LinkTTL))
			title := m.AckTitle
			if title == "" {
				title = lang(c.locale)["ack"]
			}
			msg.links = append(msg.links, link{title, ack})
			msg.text += "\n" + title + ": " + ack + "\n"
			msg.html += "\n<a href=\"" + escapeAttr(ack) + "\">" + escapeAttr(title) + "</a>"
		}
		for _, l := range m.Links {
			if l.URL != "" {
				msg.links = append(msg.links, link{l.Title, l.URL})
			}
		}
		o.Err = unwrap(s.send(ctx, c, msg, target{channel: t.Channel, address: t.Address, recipient: ch.Recipient(t.Address)}))
		out = append(out, o)
	}
	return out
}

func escapeAttr(v string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(v)
}
