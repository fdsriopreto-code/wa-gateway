// Package pollstore guarda, por sessao, as opcoes de uma enquete e um placar
// corrente dos votos. Usado pra decifrar e agregar votos de enquete
// (evento message.poll_vote e GET /api/{session}/polls/{messageId}).
//
// Tudo vive no Redis com TTL curto (~30 dias): e cache de reconciliacao, nao
// fonte da verdade.
package pollstore

import (
	"context"
	"encoding/json"
	"time"

	"wa-gateway/internal/cache"
)

// TTL das chaves de enquete.
const TTL = 30 * 24 * time.Hour

type Store struct{ rc *cache.Redis }

func New(rc *cache.Redis) *Store { return &Store{rc: rc} }

func optKey(session, pollID string) string  { return "wa:poll:opts:" + session + ":" + pollID }
func voteKey(session, pollID string) string { return "wa:poll:votes:" + session + ":" + pollID }

// SavePollOptions grava as opcoes EXATAS da enquete (ordem preservada, sem
// normalizar/trim) — elas precisam bater byte a byte no hash do voto depois.
func (s *Store) SavePollOptions(ctx context.Context, session, pollID string, options []string) error {
	if s == nil || s.rc == nil || pollID == "" {
		return nil
	}
	return s.rc.SetJSON(ctx, optKey(session, pollID), options, TTL)
}

// PollOptions devolve as opcoes guardadas (nil, nil se nao achou).
func (s *Store) PollOptions(ctx context.Context, session, pollID string) ([]string, error) {
	if s == nil || s.rc == nil || pollID == "" {
		return nil, nil
	}
	var out []string
	ok, err := s.rc.GetJSON(ctx, optKey(session, pollID), &out)
	if err != nil || !ok {
		return nil, err
	}
	return out, nil
}

// RecordVote atualiza o placar corrente: voter -> opcoes selecionadas AGORA.
// selected vazio = a pessoa desmarcou tudo (remove do placar).
func (s *Store) RecordVote(ctx context.Context, session, pollID, voter string, selected []string) error {
	if s == nil || s.rc == nil || pollID == "" || voter == "" {
		return nil
	}
	k := voteKey(session, pollID)
	rc := s.rc.Raw()
	if len(selected) == 0 {
		return rc.HDel(ctx, k, voter).Err()
	}
	b, _ := json.Marshal(selected)
	if err := rc.HSet(ctx, k, voter, b).Err(); err != nil {
		return err
	}
	return rc.Expire(ctx, k, TTL).Err()
}

// Result devolve as opcoes + o mapa voter -> opcoes selecionadas (placar atual).
func (s *Store) Result(ctx context.Context, session, pollID string) (options []string, votes map[string][]string, err error) {
	votes = map[string][]string{}
	options, err = s.PollOptions(ctx, session, pollID)
	if err != nil || s == nil || s.rc == nil {
		return options, votes, err
	}
	raw, err := s.rc.Raw().HGetAll(ctx, voteKey(session, pollID)).Result()
	if err != nil {
		return options, votes, err
	}
	for voter, js := range raw {
		var sel []string
		if json.Unmarshal([]byte(js), &sel) == nil {
			votes[voter] = sel
		}
	}
	return options, votes, nil
}
