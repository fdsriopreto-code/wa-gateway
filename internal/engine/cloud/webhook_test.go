package cloud

import (
	"log/slog"
	"sync"
	"testing"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/events"
)

func testEngine(t *testing.T) (*Engine, *[]events.Event, *sync.Mutex) {
	t.Helper()
	var got []events.Event
	var mu sync.Mutex
	e := &Engine{
		deps: engine.Deps{
			Session: "s1",
			Logger:  slog.Default(),
			Emit: func(ev events.Event) {
				mu.Lock()
				got = append(got, ev)
				mu.Unlock()
			},
		},
		cfg: engine.CloudConfig{PhoneNumberID: "PNID", AccessToken: "tok", GraphVersion: "v21.0"},
	}
	return e, &got, &mu
}

func TestIngestText(t *testing.T) {
	e, got, mu := testEngine(t)
	e.Ingest([]byte(`{
	  "entry":[{"changes":[{"field":"messages","value":{
	    "metadata":{"phone_number_id":"PNID"},
	    "contacts":[{"wa_id":"5517999","profile":{"name":"Fulano"}}],
	    "messages":[{"from":"5517999","id":"wamid.1","timestamp":"1700000000","type":"text","text":{"body":"oi"}}]
	  }}]}]
	}`))
	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 2 { // message.any + message
		t.Fatalf("emitiu %d eventos: %+v", len(*got), *got)
	}
	names := map[string]bool{}
	for _, ev := range *got {
		names[ev.Name] = true
		p := ev.Payload.(map[string]any)
		if p["body"] != "oi" || p["chatId"] != "5517999@s.whatsapp.net" || p["pushName"] != "Fulano" {
			t.Fatalf("payload errado: %+v", p)
		}
	}
	if !names["message"] || !names["message.any"] {
		t.Fatalf("faltou message/message.any: %v", names)
	}
}

func TestIngestButtonReplyAndStatus(t *testing.T) {
	e, got, mu := testEngine(t)
	e.Ingest([]byte(`{
	  "entry":[{"changes":[{"value":{
	    "messages":[{"from":"55","id":"m1","timestamp":"1","type":"interactive",
	      "interactive":{"type":"button_reply","button_reply":{"id":"opt_a","title":"Opção A"}}}],
	    "statuses":[{"id":"m0","status":"read","timestamp":"2","recipient_id":"55"}]
	  }}]}]
	}`))
	mu.Lock()
	defer mu.Unlock()
	var sawReply, sawAck bool
	for _, ev := range *got {
		p := ev.Payload.(map[string]any)
		if ev.Name == "message.any" {
			if p["type"] == "button" && p["body"] == "Opção A" {
				r := p["reply"].(map[string]any)
				if r["id"] == "opt_a" {
					sawReply = true
				}
			}
		}
		if ev.Name == "message.ack" {
			if p["type"] == "read" {
				sawAck = true
			}
		}
	}
	if !sawReply || !sawAck {
		t.Fatalf("reply=%v ack=%v — eventos: %+v", sawReply, sawAck, *got)
	}
}
