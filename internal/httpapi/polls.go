package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/pollstore"
)

// GET /api/{session}/polls/{messageId}
//
// Placar corrente de uma enquete criada por esta sessao — pra reconciliacao
// periodica. Vem do cache (Redis, TTL ~30d): enquete fora da janela devolve
// options vazio. voters trazem o JID de quem votou em cada opcao.
func (d Deps) getPollResult(w http.ResponseWriter, r *http.Request) {
	if d.Cache == nil {
		writeErr(w, http.StatusServiceUnavailable, "no_cache", "placar de enquete indisponivel (sem Redis)")
		return
	}
	session := chi.URLParam(r, "session")
	pollID := chi.URLParam(r, "messageId")

	opts, votes, err := pollstore.New(d.Cache).Result(r.Context(), session, pollID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}

	type optOut struct {
		Name   string   `json:"name"`
		Voters []string `json:"voters"`
	}
	list := make([]optOut, len(opts))
	idx := make(map[string]int, len(opts))
	for i, o := range opts {
		list[i] = optOut{Name: o, Voters: []string{}}
		idx[o] = i
	}
	for voter, sel := range votes {
		for _, s := range sel {
			if i, ok := idx[s]; ok {
				list[i].Voters = append(list[i].Voters, voter)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pollId":  pollID,
		"options": list,
	})
}
