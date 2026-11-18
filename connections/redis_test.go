package connections

import "testing"

func TestRedisDialOptions(t *testing.T) {
	if got := redisDialOptions(""); len(got) != 0 {
		t.Fatalf("no password: want 0 options, got %d", len(got))
	}
	if got := redisDialOptions("secret"); len(got) != 1 {
		t.Fatalf("with password: want 1 option, got %d", len(got))
	}
}
