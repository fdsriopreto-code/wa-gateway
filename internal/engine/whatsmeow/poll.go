package whatsmeow

import (
	"bytes"
	"context"

	"go.mau.fi/whatsmeow"
	waEvents "go.mau.fi/whatsmeow/types/events"

	"wa-gateway/internal/events"
)

// emitPollVote decifra um voto de enquete e publica "message.poll_vote".
// Roda fora do caminho critico (a decriptacao toca a tabela de secrets).
//
// selectedOptions vem em TEXTO (a opcao votada), resolvido casando o hash do
// voto contra o SHA-256 de cada opcao guardada no pollstore no momento do
// sendPoll. Se a enquete nao esta no store (antiga / Redis reiniciou) ou a
// decriptacao falha, emite mesmo assim com selectedOptions vazio e
// "undecoded": true — pra nao engolir o voto em silencio.
func (e *Engine) emitPollVote(ev *waEvents.Message) {
	pu := ev.Message.GetPollUpdateMessage()
	if pu == nil {
		return
	}
	pollID := pu.GetPollCreationMessageKey().GetID()
	voter := preferPN(ev.Info.Sender, ev.Info.SenderAlt)

	out := map[string]any{
		"pollId":          pollID,
		"chatId":          jidStr(ev.Info.Chat),
		"voter":           jidStr(voter),
		"voterName":       ev.Info.PushName,
		"selectedOptions": []string{},
		"removed":         true,
		"timestamp":       ev.Info.Timestamp.Unix(),
	}
	if e.wantRaw() {
		out["raw"] = ev
	}

	undecoded := func(reason string) {
		out["undecoded"] = true
		if reason != "" {
			e.deps.Logger.Warn("pollvote nao resolvido", "poll", pollID, "motivo", reason)
		}
		e.emitID("pv:"+ev.Info.ID, events.MessagePollVote, out)
	}

	ctx := context.Background()
	var stored []string
	if e.deps.PollStore != nil {
		stored, _ = e.deps.PollStore.PollOptions(ctx, e.deps.Session, pollID)
	}
	if len(stored) == 0 {
		undecoded("enquete fora do store")
		return
	}
	client, err := e.currentClient()
	if err != nil {
		undecoded("sessao offline")
		return
	}
	vote, err := client.DecryptPollVote(ctx, ev)
	if err != nil {
		undecoded("decrypt: " + err.Error())
		return
	}

	optHashes := whatsmeow.HashPollOptions(stored)
	selected := make([]string, 0, len(stored))
	for i, oh := range optHashes {
		for _, sh := range vote.GetSelectedOptions() {
			if bytes.Equal(oh, sh) {
				selected = append(selected, stored[i])
				break
			}
		}
	}
	out["selectedOptions"] = selected
	out["removed"] = len(selected) == 0
	delete(out, "undecoded")

	if e.deps.PollStore != nil {
		_ = e.deps.PollStore.RecordVote(ctx, e.deps.Session, pollID, jidStr(voter), selected)
	}
	e.emitID("pv:"+ev.Info.ID, events.MessagePollVote, out)
}
