package fanout

import "testing"

func TestParseInventory_FencedJSON(t *testing.T) {
	text := "Here is the surface:\n```json\n{\"assets\":[" +
		"{\"type\":\"web\",\"value\":\"https://app.example.com\"}," +
		"{\"type\":\"host\",\"value\":\"10.0.0.5\",\"ports\":[22,80]}]}\n```\nDone."
	inv := ParseInventory(text)
	if len(inv.Assets) != 2 {
		t.Fatalf("got %d assets, want 2: %+v", len(inv.Assets), inv.Assets)
	}
	if inv.Assets[0].Type != AssetWeb || inv.Assets[0].ID == "" {
		t.Errorf("unexpected first asset: %+v", inv.Assets[0])
	}
	if inv.Assets[1].Type != AssetHost || len(inv.Assets[1].Ports) != 2 {
		t.Errorf("unexpected second asset: %+v", inv.Assets[1])
	}
}

func TestParseInventory_DedupeAndClassify(t *testing.T) {
	text := "```json\n{\"assets\":[" +
		"{\"value\":\"https://a.example.com/api/v1\"}," +
		"{\"value\":\"https://a.example.com/api/v1\"}," +
		"{\"value\":\"arn:aws:s3:::my-bucket\"}]}\n```"
	inv := ParseInventory(text)
	if len(inv.Assets) != 2 {
		t.Fatalf("dedupe failed: got %d, want 2", len(inv.Assets))
	}
	if inv.Assets[0].Type != AssetAPI {
		t.Errorf("want api classification, got %s", inv.Assets[0].Type)
	}
	if inv.Assets[1].Type != AssetCloud {
		t.Errorf("want cloud classification, got %s", inv.Assets[1].Type)
	}
}

func TestFallbackInventory(t *testing.T) {
	text := "Targets found: https://x.example.com, 192.168.1.10, not-a-host, sub.example.org"
	inv := FallbackInventory(text)
	if len(inv.Assets) != 3 {
		t.Fatalf("got %d assets, want 3: %+v", len(inv.Assets), inv.Assets)
	}
}
