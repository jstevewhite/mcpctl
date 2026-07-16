package apperror

import (
	"errors"
	"testing"
)

func TestErrorWrapsAndFormats(t *testing.T) {
	base := errors.New("boom")
	e := Wrap(KindConfig, base, "load %q", "cfg.toml")
	if e.Error() != `load "cfg.toml": boom` {
		t.Fatalf("unexpected message: %q", e.Error())
	}
	if !errors.Is(e, base) {
		t.Fatal("errors.Is should find the wrapped error")
	}
}

func TestErrorNoWrapOmitsColon(t *testing.T) {
	e := Usage("bad flag %s", "--x")
	if e.Error() != "bad flag --x" {
		t.Fatalf("unexpected message: %q", e.Error())
	}
}
