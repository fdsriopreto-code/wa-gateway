package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/engine"
)

// GET /api/groups?session=
func (d Deps) listGroups(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, r.URL.Query().Get("session"))
	if !ok {
		return
	}
	groups, err := eng.ListGroups(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// GET /api/groups/{jid}?session=
func (d Deps) getGroup(w http.ResponseWriter, r *http.Request) {
	eng, ok := d.engineFor(w, r.URL.Query().Get("session"))
	if !ok {
		return
	}
	g, err := eng.GroupInfo(r.Context(), chi.URLParam(r, "jid"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

type createGroupReq struct {
	Session      string   `json:"session"`
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// POST /api/groups
func (d Deps) createGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "name e obrigatorio")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	g, err := eng.CreateGroup(r.Context(), req.Name, req.Participants)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

type sessionOnlyReq struct {
	Session string `json:"session"`
}

// POST /api/groups/{jid}/leave
func (d Deps) leaveGroup(w http.ResponseWriter, r *http.Request) {
	var req sessionOnlyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	if err := eng.LeaveGroup(r.Context(), chi.URLParam(r, "jid")); err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type participantsReq struct {
	Session      string   `json:"session"`
	Action       string   `json:"action"` // add | remove | promote | demote
	Participants []string `json:"participants"`
}

// POST /api/groups/{jid}/participants
func (d Deps) groupParticipants(w http.ResponseWriter, r *http.Request) {
	var req participantsReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Action == "" || len(req.Participants) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "action e participants sao obrigatorios")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	res, err := eng.UpdateParticipants(r.Context(), chi.URLParam(r, "jid"), engine.ParticipantAction(req.Action), req.Participants)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type groupNameReq struct {
	Session string `json:"session"`
	Name    string `json:"name"`
}

// PUT /api/groups/{jid}/name
func (d Deps) setGroupName(w http.ResponseWriter, r *http.Request) {
	var req groupNameReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "name e obrigatorio")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	if err := eng.SetGroupName(r.Context(), chi.URLParam(r, "jid"), req.Name); err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type groupTopicReq struct {
	Session string `json:"session"`
	Topic   string `json:"topic"`
}

// PUT /api/groups/{jid}/topic
func (d Deps) setGroupTopic(w http.ResponseWriter, r *http.Request) {
	var req groupTopicReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	if err := eng.SetGroupTopic(r.Context(), chi.URLParam(r, "jid"), req.Topic); err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/groups/{jid}/invite-link?session=&reset=true
func (d Deps) groupInviteLink(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	eng, ok := d.engineFor(w, q.Get("session"))
	if !ok {
		return
	}
	link, err := eng.GroupInviteLink(r.Context(), chi.URLParam(r, "jid"), q.Get("reset") == "true")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"link": link})
}

type groupPhotoReq struct {
	Session string `json:"session"`
	Data    string `json:"data"` // base64 (jpeg)
}

// PUT /api/groups/{jid}/photo
func (d Deps) setGroupPhoto(w http.ResponseWriter, r *http.Request) {
	var req groupPhotoReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Data == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "data e obrigatorio")
		return
	}
	data, _, err := decodeB64(req.Data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "data nao e base64 valido")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	id, err := eng.SetGroupPhoto(r.Context(), chi.URLParam(r, "jid"), data)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pictureId": id})
}

type groupToggleReq struct {
	Session string `json:"session"`
	Enabled bool   `json:"enabled"`
}

// PUT /api/groups/{jid}/announce   {enabled}  (true = só admins enviam)
func (d Deps) setGroupAnnounce(w http.ResponseWriter, r *http.Request) {
	var req groupToggleReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	if err := eng.SetGroupAnnounce(r.Context(), chi.URLParam(r, "jid"), req.Enabled); err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PUT /api/groups/{jid}/locked   {enabled}  (true = só admins editam infos)
func (d Deps) setGroupLocked(w http.ResponseWriter, r *http.Request) {
	var req groupToggleReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	if err := eng.SetGroupLocked(r.Context(), chi.URLParam(r, "jid"), req.Enabled); err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type joinGroupReq struct {
	Session string `json:"session"`
	Code    string `json:"code"`
}

// POST /api/groups/join
func (d Deps) joinGroup(w http.ResponseWriter, r *http.Request) {
	var req joinGroupReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "code e obrigatorio")
		return
	}
	eng, ok := d.engineFor(w, req.Session)
	if !ok {
		return
	}
	jid, err := eng.JoinGroupWithLink(r.Context(), req.Code)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "group_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"jid": jid})
}
