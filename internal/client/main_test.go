package client

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testServerPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mcpctl-testserver-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	testServerPath = filepath.Join(dir, "test-server")
	build := exec.Command("go", "build", "-o", testServerPath, "mcpctl/internal/testserver/stdio")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building test server: " + err.Error())
	}
	os.Exit(m.Run())
}
