package packs

import "testing"

func TestResolveLimit(t *testing.T) {
	t.Setenv(EnvIndexLimit, "")
	t.Setenv(EnvIndexLimitLegacy, "")
	if ResolveLimit(0) != 0 {
		t.Fatal("want unlimited")
	}
	if ResolveLimit(100) != 100 {
		t.Fatal("want explicit")
	}
	t.Setenv(EnvIndexLimit, "2500")
	if ResolveLimit(0) != 2500 {
		t.Fatal("want env")
	}
}

func TestLookupCategory(t *testing.T) {
	c, ok := LookupCategory("gguf")
	if !ok || c.Filter != "gguf" {
		t.Fatalf("%+v", c)
	}
	if _, ok := LookupCategory("nope"); ok {
		t.Fatal("expected miss")
	}
}
