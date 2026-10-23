package whatsmeow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	waEvents "go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func fakeHistory(convs, msgsPer int) *waEvents.HistorySync {
	h := &waHistorySync.HistorySync{
		SyncType:   waHistorySync.HistorySync_INITIAL_BOOTSTRAP.Enum(),
		ChunkOrder: proto.Uint32(1),
		Progress:   proto.Uint32(40),
	}
	for c := 0; c < convs; c++ {
		conv := &waHistorySync.Conversation{ID: proto.String(fmt.Sprintf("55119%08d@s.whatsapp.net", c))}
		for m := 0; m < msgsPer; m++ {
			conv.Messages = append(conv.Messages, &waHistorySync.HistorySyncMsg{
				Message: &waWeb.WebMessageInfo{Message: &waE2E.Message{
					Conversation: proto.String(strings.Repeat("mensagem de histórico ", 8)),
				}},
			})
		}
		h.Conversations = append(h.Conversations, conv)
	}
	return &waEvents.HistorySync{Data: h}
}

func TestHistorySummaryIsSmall(t *testing.T) {
	ev := fakeHistory(300, 40)
	raw, _ := json.Marshal(ev)
	sum, _ := json.Marshal(historySummary(ev, false))
	t.Logf("history.sync: cru=%d KB, resumo=%d bytes", len(raw)/1024, len(sum))
	if len(sum) > 512 {
		t.Fatalf("resumo grande demais: %d bytes", len(sum))
	}
	var m map[string]any
	_ = json.Unmarshal(sum, &m)
	if m["conversations"].(float64) != 300 || m["messages"].(float64) != 12000 || m["syncType"] != "INITIAL_BOOTSTRAP" {
		t.Fatalf("contagens erradas: %v", m)
	}
	if _, ok := historySummary(ev, true)["raw"]; !ok {
		t.Fatal("com rawEvents o struct cru deveria ir junto")
	}
}
