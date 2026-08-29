package whatsmeow

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"

	"wa-gateway/internal/engine"
)

// ctxInfo monta o ContextInfo comum (citacao + mencoes). nil se nao ha nada.
func ctxInfo(o engine.MessageOpts) *waProto.ContextInfo {
	if o.QuotedID == "" && len(o.Mentions) == 0 {
		return nil
	}
	ci := &waProto.ContextInfo{}
	if o.QuotedID != "" {
		ci.StanzaID = proto.String(o.QuotedID)
		if o.QuotedParticipant != "" {
			ci.Participant = proto.String(normJID(o.QuotedParticipant))
		}
		body := o.QuotedText
		if body == "" {
			body = " "
		}
		ci.QuotedMessage = &waProto.Message{Conversation: proto.String(body)}
	}
	if len(o.Mentions) > 0 {
		ci.MentionedJID = make([]string, 0, len(o.Mentions))
		for _, m := range o.Mentions {
			ci.MentionedJID = append(ci.MentionedJID, normJID(m))
		}
	}
	return ci
}

// normJID aceita "5511999999999" ou um jid completo e devolve sempre um jid.
func normJID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "@") {
		return s
	}
	s = strings.TrimPrefix(s, "+")
	return s + "@" + types.DefaultUserServer
}

var urlRe = regexp.MustCompile(`https?://[^\s]+`)

// applyLinkPreview tenta buscar OG tags do primeiro link do texto (best-effort).
func (e *Engine) applyLinkPreview(ctx context.Context, etm *waProto.ExtendedTextMessage, text string) {
	u := urlRe.FindString(text)
	if u == "" {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, u, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; wa-gateway link preview)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	html, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	title := meta(html, "og:title")
	if title == "" {
		if m := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`).FindSubmatch(html); m != nil {
			title = strings.TrimSpace(string(m[1]))
		}
	}
	desc := meta(html, "og:description")
	if desc == "" {
		desc = meta(html, "description")
	}
	etm.MatchedText = proto.String(u)
	if title != "" {
		etm.Title = proto.String(decodeEntities(title))
	}
	if desc != "" {
		etm.Description = proto.String(decodeEntities(desc))
	}
	if img := meta(html, "og:image"); img != "" {
		if thumb := fetchThumb(cctx, img); len(thumb) > 0 {
			etm.JPEGThumbnail = thumb
			etm.PreviewType = waProto.ExtendedTextMessage_IMAGE.Enum()
		}
	}
}

func meta(html []byte, prop string) string {
	// <meta property="og:title" content="..."> em qualquer ordem de atributos
	re := regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)=["']` + regexp.QuoteMeta(prop) + `["'][^>]*>`)
	tag := re.Find(html)
	if tag == nil {
		return ""
	}
	m := regexp.MustCompile(`(?is)content=["']([^"']*)["']`).FindSubmatch(tag)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

func fetchThumb(ctx context.Context, url string) []byte {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 300*1024))
	if len(b) < 64 {
		return nil
	}
	return b
}

func decodeEntities(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&apos;", "'", "&nbsp;", " ")
	return r.Replace(s)
}

// --- sticker / poll ---

func (e *Engine) SendSticker(ctx context.Context, chatID string, data []byte, opts engine.MessageOpts) (engine.SendResult, error) {
	up, err := e.uploadMedia(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return engine.SendResult{}, err
	}
	return e.send(ctx, chatID, &waProto.Message{StickerMessage: &waProto.StickerMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		Mimetype:      proto.String("image/webp"),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		ContextInfo:   ctxInfo(opts),
	}})
}

func (e *Engine) SendPoll(ctx context.Context, chatID, name string, options []string, selectable int, opts engine.MessageOpts) (engine.SendResult, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.SendResult{}, err
	}
	if len(options) < 2 {
		return engine.SendResult{}, errBadReq("enquete precisa de pelo menos 2 opcoes")
	}
	if selectable <= 0 {
		selectable = 1
	}
	msg := client.BuildPollCreation(name, options, selectable)
	if ci := ctxInfo(opts); ci != nil && msg.PollCreationMessage != nil {
		msg.PollCreationMessage.ContextInfo = ci
	}
	return e.send(ctx, chatID, msg)
}

type reqErr struct{ s string }

func (e reqErr) Error() string { return e.s }
func errBadReq(s string) error { return reqErr{s} }
