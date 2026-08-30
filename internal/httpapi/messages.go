package httpapi

import (
	"encoding/json"
	"net/http"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/outbox"
)

// sendInteractive: botões/lista/CTA. Só funciona em sessão engine=cloud;
// no whatsmeow devolve 501 not_supported.
func (d Deps) sendInteractive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Session string `json:"session"`
		ChatID  string `json:"chatId"`
		engine.Interactive
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || req.Type == "" || req.Body == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId, type e body sao obrigatorios")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendInteractive(r.Context(), req.ChatID, req.Interactive)
	sendResp(w, res, err)
}

// sendTemplate: mensagem de template aprovada (Cloud API).
func (d Deps) sendTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Session    string          `json:"session"`
		ChatID     string          `json:"chatId"`
		Name       string          `json:"name"`
		Language   string          `json:"language"`
		Components json.RawMessage `json:"components"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || req.Name == "" || req.Language == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId, name e language sao obrigatorios")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendTemplate(r.Context(), req.ChatID, req.Name, req.Language, req.Components)
	sendResp(w, res, err)
}

// queueOpts e embutido nas requests de envio: quando enqueue=true, o envio
// passa pela fila de saida com pacing por sessao (anti-ban) em vez de sair
// na hora. delay adiciona um atraso extra sobre o intervalo do pacing.
type queueOpts struct {
	Enqueue bool   `json:"enqueue,omitempty"`
	Delay   string `json:"delay,omitempty"` // ex.: "30s", "5m"
}

// msgExtra e embutido nos envios que aceitam citacao / mencoes / link preview.
type msgExtra struct {
	QuotedID          string   `json:"quotedId,omitempty"`
	QuotedParticipant string   `json:"quotedParticipant,omitempty"`
	QuotedText        string   `json:"quotedText,omitempty"`
	Mentions          []string `json:"mentions,omitempty"`
	LinkPreview       bool     `json:"linkPreview,omitempty"`
}

func (m msgExtra) opts() engine.MessageOpts {
	return engine.MessageOpts{
		QuotedID: m.QuotedID, QuotedParticipant: m.QuotedParticipant, QuotedText: m.QuotedText,
		Mentions: m.Mentions, LinkPreview: m.LinkPreview,
	}
}

type sendTextReq struct {
	queueOpts
	msgExtra
	Session string `json:"session"`
	ChatID  string `json:"chatId"`
	Text    string `json:"text"`
}

func (d Deps) sendText(w http.ResponseWriter, r *http.Request) {
	var req sendTextReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || req.Text == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e text sao obrigatorios")
		return
	}
	if req.Enqueue {
		d.enqueue(w, r, req.Session, outbox.KindText,
			outbox.Args{ChatID: req.ChatID, Text: req.Text, Opts: req.opts()}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendText(r.Context(), req.ChatID, req.Text, req.opts())
	sendResp(w, res, err)
}

// mediaReq cobre imagem/arquivo/video/audio: os campos extras sao ignorados
// pelos tipos que nao os usam.
type mediaReq struct {
	queueOpts
	msgExtra
	Session  string `json:"session"`
	ChatID   string `json:"chatId"`
	Caption  string `json:"caption"`
	Mimetype string `json:"mimetype"`
	Filename string `json:"filename"`
	Data     string `json:"data"` // base64 (aceita data URI)
	Seconds  uint32 `json:"seconds"`
	GIF      bool   `json:"gif"`
	Voice    bool   `json:"voice"`
}

// mediaFor valida a request comum de midia e devolve os dados normalizados.
func (d Deps) mediaFor(w http.ResponseWriter, r *http.Request) (mediaReq, engine.Media, bool) {
	var req mediaReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return req, engine.Media{}, false
	}
	if req.ChatID == "" || req.Data == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e data sao obrigatorios")
		return req, engine.Media{}, false
	}
	data, detected, err := decodeB64(req.Data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "data nao e base64 valido")
		return req, engine.Media{}, false
	}
	mime := req.Mimetype
	if mime == "" {
		mime = detected
	}
	return req, engine.Media{
		Data: data, Mimetype: mime, Filename: req.Filename, Caption: req.Caption,
		Seconds: req.Seconds, GIF: req.GIF, Voice: req.Voice, Opts: req.opts(),
	}, true
}

type stickerReq struct {
	queueOpts
	msgExtra
	Session string `json:"session"`
	ChatID  string `json:"chatId"`
	Data    string `json:"data"`
}

func (d Deps) sendSticker(w http.ResponseWriter, r *http.Request) {
	var req stickerReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || req.Data == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e data sao obrigatorios")
		return
	}
	data, _, err := decodeB64(req.Data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "data nao e base64 valido")
		return
	}
	if req.Enqueue {
		d.enqueue(w, r, req.Session, outbox.KindSticker,
			outbox.Args{ChatID: req.ChatID, Media: &engine.Media{Data: data}, Opts: req.opts()}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendSticker(r.Context(), req.ChatID, data, req.opts())
	sendResp(w, res, err)
}

type pollReq struct {
	queueOpts
	msgExtra
	Session    string   `json:"session"`
	ChatID     string   `json:"chatId"`
	Name       string   `json:"name"`
	Options    []string `json:"options"`
	Selectable int      `json:"selectable"` // quantas opcoes podem ser marcadas (default 1)
}

func (d Deps) sendPoll(w http.ResponseWriter, r *http.Request) {
	var req pollReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || req.Name == "" || len(req.Options) < 2 {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId, name e ao menos 2 options sao obrigatorios")
		return
	}
	if req.Enqueue {
		d.enqueue(w, r, req.Session, outbox.KindPoll, outbox.Args{
			ChatID: req.ChatID, PollName: req.Name, PollOpts: req.Options, PollPick: req.Selectable, Opts: req.opts(),
		}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendPoll(r.Context(), req.ChatID, req.Name, req.Options, req.Selectable, req.opts())
	sendResp(w, res, err)
}

// sendMedia e o corpo comum de sendImage/sendFile/sendVideo/sendAudio.
func (d Deps) sendMedia(w http.ResponseWriter, r *http.Request, kind outbox.Kind, call func(eng engine.Engine, chatID string, m engine.Media) (engine.SendResult, error)) {
	req, m, ok := d.mediaFor(w, r)
	if !ok {
		return
	}
	if req.Enqueue {
		mm := m
		d.enqueue(w, r, req.Session, kind, outbox.Args{ChatID: req.ChatID, Media: &mm}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := call(eng, req.ChatID, m)
	sendResp(w, res, err)
}

func (d Deps) sendImage(w http.ResponseWriter, r *http.Request) {
	d.sendMedia(w, r, outbox.KindImage, func(eng engine.Engine, chatID string, m engine.Media) (engine.SendResult, error) {
		return eng.SendImage(r.Context(), chatID, m.Data, m.Mimetype, m.Caption)
	})
}

func (d Deps) sendFile(w http.ResponseWriter, r *http.Request) {
	d.sendMedia(w, r, outbox.KindFile, func(eng engine.Engine, chatID string, m engine.Media) (engine.SendResult, error) {
		return eng.SendFile(r.Context(), chatID, m)
	})
}

func (d Deps) sendVideo(w http.ResponseWriter, r *http.Request) {
	d.sendMedia(w, r, outbox.KindVideo, func(eng engine.Engine, chatID string, m engine.Media) (engine.SendResult, error) {
		return eng.SendVideo(r.Context(), chatID, m)
	})
}

func (d Deps) sendAudio(w http.ResponseWriter, r *http.Request) {
	d.sendMedia(w, r, outbox.KindAudio, func(eng engine.Engine, chatID string, m engine.Media) (engine.SendResult, error) {
		return eng.SendAudio(r.Context(), chatID, m)
	})
}

type sendLocationReq struct {
	queueOpts
	Session   string  `json:"session"`
	ChatID    string  `json:"chatId"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
}

func (d Deps) sendLocation(w http.ResponseWriter, r *http.Request) {
	var req sendLocationReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e obrigatorio")
		return
	}
	loc := engine.Location{Latitude: req.Latitude, Longitude: req.Longitude, Name: req.Name, Address: req.Address}
	if req.Enqueue {
		d.enqueue(w, r, req.Session, outbox.KindLocation, outbox.Args{ChatID: req.ChatID, Location: &loc}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendLocation(r.Context(), req.ChatID, loc)
	sendResp(w, res, err)
}

type sendContactReq struct {
	queueOpts
	Session  string           `json:"session"`
	ChatID   string           `json:"chatId"`
	Contacts []engine.Contact `json:"contacts"`
}

func (d Deps) sendContact(w http.ResponseWriter, r *http.Request) {
	var req sendContactReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || len(req.Contacts) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e contacts sao obrigatorios")
		return
	}
	if req.Enqueue {
		d.enqueue(w, r, req.Session, outbox.KindContact, outbox.Args{ChatID: req.ChatID, Contacts: req.Contacts}, req.Delay)
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.SendContact(r.Context(), req.ChatID, req.Contacts)
	sendResp(w, res, err)
}

// msgRefReq e a base das operacoes sobre uma mensagem ja existente.
type msgRefReq struct {
	queueOpts
	Session   string `json:"session"`
	ChatID    string `json:"chatId"`
	MessageID string `json:"messageId"`
	FromMe    bool   `json:"fromMe"`
	SenderID  string `json:"senderId"`
	Emoji     string `json:"emoji"` // so para reaction ("" remove)
	Text      string `json:"text"`  // so para edit
}

func (r msgRefReq) ref() engine.MessageRef {
	return engine.MessageRef{ChatID: r.ChatID, ID: r.MessageID, FromMe: r.FromMe, SenderID: r.SenderID}
}

func (d Deps) decodeRef(w http.ResponseWriter, r *http.Request) (engine.Engine, msgRefReq, bool) {
	var req msgRefReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return nil, req, false
	}
	if req.ChatID == "" || req.MessageID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e messageId sao obrigatorios")
		return nil, req, false
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return nil, req, false
	}
	return eng, req, true
}

func (d Deps) reaction(w http.ResponseWriter, r *http.Request) {
	eng, req, ok := d.decodeRef(w, r)
	if !ok {
		return
	}
	res, err := eng.SendReaction(r.Context(), req.ref(), req.Emoji)
	sendResp(w, res, err)
}

func (d Deps) deleteMessage(w http.ResponseWriter, r *http.Request) {
	eng, req, ok := d.decodeRef(w, r)
	if !ok {
		return
	}
	res, err := eng.DeleteMessage(r.Context(), req.ref())
	sendResp(w, res, err)
}

func (d Deps) editMessage(w http.ResponseWriter, r *http.Request) {
	eng, req, ok := d.decodeRef(w, r)
	if !ok {
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "text e obrigatorio")
		return
	}
	res, err := eng.EditMessage(r.Context(), req.ref(), req.Text)
	sendResp(w, res, err)
}

func (d Deps) sendSeen(w http.ResponseWriter, r *http.Request) {
	eng, req, ok := d.decodeRef(w, r)
	if !ok {
		return
	}
	if err := eng.MarkRead(r.Context(), req.ref()); err != nil {
		writeErr(w, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type presenceReq struct {
	Session string `json:"session"`
	ChatID  string `json:"chatId"`
	State   string `json:"state"` // typing | recording | paused
}

func (d Deps) chatPresence(w http.ResponseWriter, r *http.Request) {
	var req presenceReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ChatID == "" || req.State == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "chatId e state sao obrigatorios")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	if err := eng.SendChatPresence(r.Context(), req.ChatID, engine.ChatState(req.State)); err != nil {
		writeErr(w, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
