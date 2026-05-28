package sysinfo

import "testing"

func TestClassifyAdapter(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"wlan0", "wireless"},
		{"wlp3s0", "wireless"},
		{"Wi-Fi", "wireless"},
		{"WIFI", "wireless"},
		{"AirPort", "wireless"},
		{"en0", "wired"},
		{"eth0", "wired"},
		{"vwl0", "wired"},  // regression: substring-match used to flag this wireless
		{"awlan", "wired"}, // ditto
	}
	for _, c := range cases {
		if got := classifyAdapter(c.name); got != c.want {
			t.Errorf("classifyAdapter(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
