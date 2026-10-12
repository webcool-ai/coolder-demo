package main

import "testing"

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 3); got != "hel" {
		t.Fatalf("truncateRunes = %q, want %q", got, "hel")
	}
	if got := truncateRunes("short", 10); got != "short" {
		t.Fatalf("truncateRunes = %q, want %q", got, "short")
	}
	// multi-byte runes must not be cut in half
	if got := truncateRunes("玩家名字测试", 3); got != "玩家名" {
		t.Fatalf("truncateRunes = %q, want %q", got, "玩家名")
	}
}
