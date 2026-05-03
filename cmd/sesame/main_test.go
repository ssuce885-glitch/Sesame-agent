package main

import "testing"

func TestConnectAddrNormalizesWildcardHosts(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0:8421": "127.0.0.1:8421",
		":8421":        "127.0.0.1:8421",
		"[::]:8421":    "127.0.0.1:8421",
		"127.0.0.1:9":  "127.0.0.1:9",
		"bad-address":  "bad-address",
	}
	for input, want := range tests {
		if got := connectAddr(input); got != want {
			t.Fatalf("connectAddr(%q) = %q, want %q", input, got, want)
		}
	}
}
