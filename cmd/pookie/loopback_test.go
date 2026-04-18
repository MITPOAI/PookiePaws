package main

import "testing"

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:18800":  true,
		"127.1.2.3:18800":  true,
		"[::1]:18800":      true,
		"localhost:18800":  true,
		"LocalHost:18800":  true,
		":18800":           false,
		"0.0.0.0:18800":    false,
		"192.168.1.5:8080": false,
		"10.0.0.1:80":      false,
		"":                 false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}
