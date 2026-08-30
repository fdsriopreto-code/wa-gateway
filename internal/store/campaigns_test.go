package store

import "testing"

func TestCampaignJobIDRoundTrip(t *testing.T) {
	id := CampaignJobID("cmp_abc123", 42)
	if id != "camp:cmp_abc123:42" {
		t.Fatalf("id = %q", id)
	}
	cid, n, ok := ParseCampaignJobID(id)
	if !ok || cid != "cmp_abc123" || n != 42 {
		t.Fatalf("parse = %q %d %v", cid, n, ok)
	}
}

func TestParseCampaignJobIDRejects(t *testing.T) {
	for _, bad := range []string{
		"outbox-normal-id",
		"camp:semnumero",
		"camp:cmp_x:notanumber",
		"",
		"camp:",
	} {
		if _, _, ok := ParseCampaignJobID(bad); ok {
			t.Errorf("%q devia falhar o parse", bad)
		}
	}
}
