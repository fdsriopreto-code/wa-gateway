package httpapi

import (
	"net/http"
	"strings"
)

// GET /api/contacts/check?session=&phone=55...,55...  (phone repetido ou lista)
func (d Deps) contactsCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	eng, ok := d.engineFor(w, q.Get("session"))
	if !ok {
		return
	}
	var phones []string
	for _, p := range q["phone"] {
		for _, part := range strings.Split(p, ",") {
			if s := strings.TrimSpace(part); s != "" {
				phones = append(phones, s)
			}
		}
	}
	if len(phones) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "phone e obrigatorio")
		return
	}
	res, err := eng.CheckOnWhatsApp(r.Context(), phones)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /api/contacts/info?session=&jid=...,...
func (d Deps) contactsInfo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	eng, ok := d.engineFor(w, q.Get("session"))
	if !ok {
		return
	}
	var jids []string
	for _, j := range q["jid"] {
		for _, part := range strings.Split(j, ",") {
			if s := strings.TrimSpace(part); s != "" {
				jids = append(jids, s)
			}
		}
	}
	if len(jids) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "jid e obrigatorio")
		return
	}
	res, err := eng.GetUserInfo(r.Context(), jids)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /api/contacts/profile-picture?session=&jid=&preview=true
func (d Deps) contactsPicture(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	eng, ok := d.engineFor(w, q.Get("session"))
	if !ok {
		return
	}
	jid := q.Get("jid")
	if jid == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "jid e obrigatorio")
		return
	}
	url, err := eng.GetProfilePicture(r.Context(), jid, q.Get("preview") == "true")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}
