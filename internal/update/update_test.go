package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"dev", "v9.9.9", false},
		{"", "v1.0.0", false},
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "v2.0.0", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.3", "v1.2.2", false},
		{"v2.0.0", "v1.9.9", false},
		{"v1.0.0-rc1", "v1.0.0", false}, // pre-release suffix is tolerated and ignored for comparison
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestAssetURL(t *testing.T) {
	rel := &Release{Assets: []Asset{
		{Name: "streamer-remote-windows-amd64.exe", BrowserDownloadURL: "https://example/win.exe"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example/checksums.txt"},
	}}
	url, ok := rel.AssetURL()
	if !ok || url != "https://example/win.exe" {
		t.Fatalf("got %q, %v", url, ok)
	}

	empty := &Release{}
	if _, ok := empty.AssetURL(); ok {
		t.Fatal("expected no asset match on empty release")
	}
}
