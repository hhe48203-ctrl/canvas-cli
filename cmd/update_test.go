package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRootHasUpdateFlag(t *testing.T) {
	if flag := newRootCommand().Flags().Lookup("update"); flag == nil {
		t.Fatal("root command is missing --update")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "canvas")
	built := filepath.Join(dir, "built")
	if err := os.WriteFile(target, []byte("old"), 0o751); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(built, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(target, built); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("replacement = %q", data)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o751 {
		t.Fatalf("replacement mode = %o", info.Mode().Perm())
	}
}
