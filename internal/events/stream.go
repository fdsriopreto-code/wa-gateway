package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// streamKey e o Redis Stream que serve de log duravel de eventos. webhook e
// inbox consomem dele (consumer groups, com XACK e replay de pendencias),
// entao um restart / crash no meio nao perde entrega nem persistencia.
const streamKey = "wa:events"

type Stream struct {
	rdb     *redis.Client
	maxLen  int64
	log     *slog.Logger
	in      chan Event
	dropped atomic.Uint64
}

type wireEvent struct {
	ID        string `json:"id"`
	Session   string `json:"session"`
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Timestamp int64  `json:"ts"`
	Payload   any    `json:"payload"`
}

// NewStream cria o log e sobe o goroutine de escrita. maxLen limita o stream
// (aprox) para nao crescer sem fim.
func NewStream(rdb *redis.Client, log *slog.Logger) *Stream {
	s := &Stream{rdb: rdb, maxLen: 100_000, log: log, in: make(chan Event, 8192)}
	go s.writer()
	return s
}

// Append enfileira o evento para escrita (nao bloqueia o publicador). Se o
// buffer de escrita encher (Redis persistentemente lento), loga e perde — o
// mesmo evento ja foi entregue best-effort pelo barramento in-process.
func (s *Stream) Append(e Event) {
	if s == nil {
		return
	}
	select {
	case s.in <- e:
	default:
		if n := s.dropped.Add(1); n%200 == 1 && s.log != nil {
			s.log.Error("event stream: buffer de escrita cheio, evento NAO duravel", "dropped_total", n)
		}
	}
}

func (s *Stream) writer() {
	for e := range s.in {
		b, err := json.Marshal(wireEvent{e.ID, e.Session, e.Name, e.Engine, e.Timestamp.UnixMilli(), e.Payload})
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = s.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey, MaxLen: s.maxLen, Approx: true,
			Values: map[string]any{"d": b},
		}).Err()
		cancel()
		if err != nil && s.log != nil {
			s.log.Warn("event stream: XADD falhou", "err", err)
		}
	}
}

// Consume roda um consumer group ate ctx cancelar: entrega cada evento que
// casa `patterns` ao handler, com ate `concurrency` em paralelo; XACK so no
// sucesso (erro -> fica pendente -> reprocessa). Reivindica pendencias de
// consumidores mortos a cada ~30s.
func (s *Stream) Consume(ctx context.Context, group, consumer string, patterns []string, concurrency int, handler func(context.Context, Event) error) {
	if concurrency < 1 {
		concurrency = 1
	}
	// "$" = grupo novo começa nos eventos daqui pra frente. Em restart o grupo
	// já existe (BUSYGROUP) e retoma do cursor salvo no Redis + PEL, então não
	// há risco de reprocessar o stream inteiro.
	if err := s.rdb.XGroupCreateMkStream(ctx, streamKey, group, "$").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		s.log.Error("event stream: criar grupo", "group", group, "err", err)
	}

	sem := make(chan struct{}, concurrency)
	runBatch := func(msgs []redis.XMessage) {
		var wg sync.WaitGroup
		for _, m := range msgs {
			wg.Add(1)
			sem <- struct{}{}
			go func(m redis.XMessage) {
				defer wg.Done()
				defer func() { <-sem }()
				s.dispatch(ctx, group, m, patterns, handler)
			}(m)
		}
		wg.Wait()
	}

	lastClaim := time.Now()
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: group, Consumer: consumer, Streams: []string{streamKey, ">"},
			Count: 256, Block: 5 * time.Second,
		}).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if err != redis.Nil && s.log != nil {
				s.log.Warn("event stream: XREADGROUP", "group", group, "err", err)
				time.Sleep(time.Second)
			}
		} else {
			for _, st := range res {
				runBatch(st.Messages)
			}
		}
		if time.Since(lastClaim) > 30*time.Second {
			s.claimStale(ctx, group, consumer, patterns, handler, runBatch)
			lastClaim = time.Now()
		}
	}
}

func (s *Stream) dispatch(ctx context.Context, group string, m redis.XMessage, patterns []string, handler func(context.Context, Event) error) {
	raw, _ := m.Values["d"].(string)
	var w wireEvent
	if json.Unmarshal([]byte(raw), &w) != nil {
		s.rdb.XAck(context.Background(), streamKey, group, m.ID) // lixo: nao trava a fila
		return
	}
	e := Event{
		ID: w.ID, Session: w.Session, Name: w.Name, Engine: w.Engine,
		Timestamp: time.UnixMilli(w.Timestamp), Payload: w.Payload,
	}
	if len(patterns) > 0 && !MatchAny(patterns, e.Name) {
		s.rdb.XAck(context.Background(), streamKey, group, m.ID) // nao interessa a este grupo
		return
	}
	if err := handler(ctx, e); err != nil {
		if s.log != nil {
			s.log.Warn("event stream: handler falhou, vai reprocessar", "group", group, "event", e.Name, "err", err)
		}
		return // sem XACK -> pendente -> claimStale reprocessa
	}
	s.rdb.XAck(context.Background(), streamKey, group, m.ID)
}

func (s *Stream) claimStale(ctx context.Context, group, consumer string, patterns []string, handler func(context.Context, Event) error, run func([]redis.XMessage)) {
	start := "0"
	for {
		msgs, next, err := s.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: streamKey, Group: group, Consumer: consumer,
			MinIdle: 60 * time.Second, Start: start, Count: 128,
		}).Result()
		if err != nil || len(msgs) == 0 {
			return
		}
		run(msgs)
		if next == "0-0" || next == "" {
			return
		}
		start = next
	}
}
