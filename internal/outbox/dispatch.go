package outbox

import (
	"context"
	"fmt"

	"wa-gateway/internal/engine"
)

// Kind identifica a acao de envio carregada por um Job.
type Kind string

const (
	KindText     Kind = "text"
	KindImage    Kind = "image"
	KindFile     Kind = "file"
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindSticker  Kind = "sticker"
	KindLocation Kind = "location"
	KindContact  Kind = "contact"
	KindPoll     Kind = "poll"
	KindForward  Kind = "forward"
	KindReaction Kind = "reaction"
	KindDelete   Kind = "delete"
	KindEdit     Kind = "edit"
)

// Args e a uniao dos parametros possiveis de um envio. Cada Kind usa um
// subconjunto; o resto fica nulo/zero e nao vai pro JSON.
type Args struct {
	ChatID   string                `json:"chatId,omitempty"`
	Text     string                `json:"text,omitempty"`
	Media    *engine.Media         `json:"media,omitempty"`
	Location *engine.Location      `json:"location,omitempty"`
	Contacts []engine.Contact      `json:"contacts,omitempty"`
	Ref      *engine.MessageRef    `json:"ref,omitempty"`
	Emoji    string                `json:"emoji,omitempty"`
	Opts     engine.MessageOpts    `json:"opts,omitempty"`
	PollName string                `json:"pollName,omitempty"`
	PollOpts []string              `json:"pollOpts,omitempty"`
	PollPick int                   `json:"pollPick,omitempty"`
	Forward  *engine.ForwardSource `json:"forward,omitempty"`
}

// Dispatch executa o job contra a engine ja resolvida.
func Dispatch(ctx context.Context, eng engine.Engine, kind Kind, a Args) (engine.SendResult, error) {
	switch kind {
	case KindText:
		return eng.SendText(ctx, a.ChatID, a.Text, a.Opts)
	case KindImage:
		m := media(a)
		return eng.SendImage(ctx, a.ChatID, m.Data, m.Mimetype, m.Caption)
	case KindFile:
		return eng.SendFile(ctx, a.ChatID, media(a))
	case KindVideo:
		return eng.SendVideo(ctx, a.ChatID, media(a))
	case KindAudio:
		return eng.SendAudio(ctx, a.ChatID, media(a))
	case KindSticker:
		return eng.SendSticker(ctx, a.ChatID, media(a).Data, a.Opts)
	case KindPoll:
		return eng.SendPoll(ctx, a.ChatID, a.PollName, a.PollOpts, a.PollPick, a.Opts)
	case KindForward:
		if a.Forward == nil {
			return engine.SendResult{}, fmt.Errorf("forward ausente")
		}
		return eng.Forward(ctx, a.ChatID, *a.Forward)
	case KindLocation:
		if a.Location == nil {
			return engine.SendResult{}, fmt.Errorf("location ausente")
		}
		return eng.SendLocation(ctx, a.ChatID, *a.Location)
	case KindContact:
		return eng.SendContact(ctx, a.ChatID, a.Contacts)
	case KindReaction:
		if a.Ref == nil {
			return engine.SendResult{}, fmt.Errorf("ref ausente")
		}
		return eng.SendReaction(ctx, *a.Ref, a.Emoji)
	case KindDelete:
		if a.Ref == nil {
			return engine.SendResult{}, fmt.Errorf("ref ausente")
		}
		return eng.DeleteMessage(ctx, *a.Ref)
	case KindEdit:
		if a.Ref == nil {
			return engine.SendResult{}, fmt.Errorf("ref ausente")
		}
		return eng.EditMessage(ctx, *a.Ref, a.Text)
	default:
		return engine.SendResult{}, fmt.Errorf("kind desconhecido: %q", kind)
	}
}

func media(a Args) engine.Media {
	if a.Media == nil {
		return engine.Media{}
	}
	return *a.Media
}
