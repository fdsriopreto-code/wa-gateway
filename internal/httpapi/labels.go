package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GET /api/{session}/labels — etiquetas do WhatsApp Business (cache local,
// re-sincronizado no reconnect).
func (d Deps) listLabels(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	res, err := eng.Labels(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /api/{session}/labels — cria/edita/apaga uma etiqueta.
// {labelId, name, color, deleted}
func (d Deps) editLabel(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		LabelID string `json:"labelId"`
		Name    string `json:"name"`
		Color   int32  `json:"color"`
		Deleted bool   `json:"deleted"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := eng.EditLabel(r.Context(), req.LabelID, req.Name, req.Color, req.Deleted); err != nil {
		writeErr(w, http.StatusBadGateway, "label_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "labelId": req.LabelID})
}

// POST /api/{session}/labels/chat — associa/desassocia etiqueta de um chat.
// {chatId, labelId, on}
func (d Deps) labelChat(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		ChatID  string `json:"chatId"`
		LabelID string `json:"labelId"`
		On      bool   `json:"on"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := eng.SetChatLabel(r.Context(), req.ChatID, req.LabelID, req.On); err != nil {
		writeErr(w, http.StatusBadGateway, "label_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/{session}/labels/message — associa/desassocia etiqueta de uma
// mensagem. {chatId, messageId, labelId, on}
func (d Deps) labelMessage(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, chi.URLParam(r, "session"))
	if !ok {
		return
	}
	var req struct {
		ChatID    string `json:"chatId"`
		MessageID string `json:"messageId"`
		LabelID   string `json:"labelId"`
		On        bool   `json:"on"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := eng.SetMessageLabel(r.Context(), req.ChatID, req.MessageID, req.LabelID, req.On); err != nil {
		writeErr(w, http.StatusBadGateway, "label_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
