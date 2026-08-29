package whatsmeow

import (
	"reflect"
	"strings"

	waEvents "go.mau.fi/whatsmeow/types/events"

	"wa-gateway/internal/engine"
)

// translate mapeia um evento cru do whatsmeow para um nome canonico
// "dominio.acao". Eventos sem mapeamento explicito ainda passam, como
// "engine.<tipo>" — cobertura total, sem perder nada.
func translate(raw any) (name string, payload any) {
	switch raw.(type) {

	// ---- conexao / sessao ----
	case *waEvents.QR:
		return "session.qr.raw", raw
	case *waEvents.PairSuccess:
		return "session.pair", raw
	case *waEvents.PairError:
		return "session.pair.error", raw
	case *waEvents.QRScannedWithoutMultidevice:
		return "session.qr.no_multidevice", raw
	case *waEvents.Connected:
		return "session.status", raw
	case *waEvents.Disconnected:
		return "session.status", raw
	case *waEvents.LoggedOut:
		return "session.logged_out", raw
	case *waEvents.StreamReplaced:
		return "session.stream_replaced", raw
	case *waEvents.StreamError:
		return "session.stream_error", raw
	case *waEvents.ConnectFailure:
		return "session.connect_failure", raw
	case *waEvents.ClientOutdated:
		return "session.client_outdated", raw
	case *waEvents.TemporaryBan:
		return "session.temporary_ban", raw
	case *waEvents.KeepAliveTimeout:
		return "session.keepalive_timeout", raw
	case *waEvents.KeepAliveRestored:
		return "session.keepalive_restored", raw

	// ---- mensagens ----
	// NOTE: *waEvents.Message e *waEvents.Receipt sao tratados antes de chegar
	// aqui (handleEvent), com payload normalizado.
	case *waEvents.UndecryptableMessage:
		return "message.undecryptable", raw
	case *waEvents.MediaRetry:
		return "message.media_retry", raw

	// ---- presenca ----
	case *waEvents.Presence:
		return "presence.update", raw
	case *waEvents.ChatPresence:
		return "chat.presence", raw

	// ---- grupos ----
	case *waEvents.GroupInfo:
		return "group.update", raw
	case *waEvents.JoinedGroup:
		return "group.join", raw

	// ---- contatos / perfil ----
	case *waEvents.Contact:
		return "contact.update", raw
	case *waEvents.PushName:
		return "contact.push_name", raw
	case *waEvents.BusinessName:
		return "contact.business_name", raw
	case *waEvents.Picture:
		return "contact.picture", raw
	case *waEvents.IdentityChange:
		return "contact.identity_change", raw

	// ---- chats / ajustes ----
	case *waEvents.Mute:
		return "chat.mute", raw
	case *waEvents.Pin:
		return "chat.pin", raw
	case *waEvents.Star:
		return "message.star", raw
	case *waEvents.DeleteForMe:
		return "message.delete_for_me", raw
	case *waEvents.DeleteChat:
		return "chat.delete", raw
	case *waEvents.MarkChatAsRead:
		return "chat.read", raw
	case *waEvents.ClearChat:
		return "chat.clear", raw
	case *waEvents.Archive:
		return "chat.archive", raw

	// ---- labels ----
	case *waEvents.LabelEdit:
		return "label.edit", raw
	case *waEvents.LabelAssociationChat:
		return "label.chat", raw
	case *waEvents.LabelAssociationMessage:
		return "label.message", raw

	// ---- privacidade / bloqueio ----
	case *waEvents.PrivacySettings:
		return "privacy.settings", raw
	case *waEvents.Blocklist:
		return "blocklist.update", raw

	// ---- newsletters / canais ----
	case *waEvents.NewsletterJoin:
		return "newsletter.join", raw
	case *waEvents.NewsletterLeave:
		return "newsletter.leave", raw
	case *waEvents.NewsletterMuteChange:
		return "newsletter.mute", raw
	case *waEvents.NewsletterLiveUpdate:
		return "newsletter.live_update", raw

	// ---- chamadas ----
	case *waEvents.CallOffer:
		return "call.received", raw
	case *waEvents.CallOfferNotice:
		return "call.received", raw
	case *waEvents.CallAccept:
		return "call.accepted", raw
	case *waEvents.CallTerminate:
		return "call.terminated", raw
	case *waEvents.CallRelayLatency:
		return "call.relay_latency", raw

	// ---- sincronizacao ----
	case *waEvents.HistorySync:
		return "history.sync", raw
	case *waEvents.OfflineSyncPreview:
		return "sync.offline_preview", raw
	case *waEvents.OfflineSyncCompleted:
		return "sync.offline_completed", raw
	case *waEvents.AppStateSyncComplete:
		return "sync.appstate_complete", raw
	case *waEvents.AppState:
		return "sync.appstate", raw
	}

	// fallback: repassa qualquer coisa que o whatsmeow adicionar no futuro.
	return "engine." + strings.ToLower(goTypeName(raw)), raw
}

func goTypeName(v any) string {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		return "unknown"
	}
	return t.Name()
}

// connState deriva o Status interno a partir de eventos de conexao.
func connState(raw any) (engine.Status, bool) {
	switch raw.(type) {
	case *waEvents.Connected, *waEvents.PairSuccess:
		return engine.StatusWorking, true
	case *waEvents.LoggedOut:
		return engine.StatusLoggedOut, true
	case *waEvents.Disconnected, *waEvents.StreamReplaced, *waEvents.ConnectFailure,
		*waEvents.KeepAliveTimeout, *waEvents.TemporaryBan, *waEvents.ClientOutdated:
		return engine.StatusStarting, true
	}
	return "", false
}
