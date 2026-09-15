package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const updateModule = "github.com/hhe48203-ctrl/canvas-cli@main"

var (
	updateExecutable = os.Executable
	updateCommand    = exec.Command
	updateReplace    = replaceExecutable
)

func updateCLI(stderr io.Writer) error {
	var diagnostics bytes.Buffer
	target, err := updateExecutable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	if target, err = filepath.EvalSymlinks(target); err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}

	work, err := os.MkdirTemp("", "canvas-update-*")
	if err != nil {
		return fmt.Errorf("create update directory: %w", err)
	}
	defer os.RemoveAll(work)

	fmt.Fprintln(&diagnostics, "Updating canvas...")
	build := updateCommand("go", "install", updateModule)
	build.Env = append(os.Environ(), "GOBIN="+work)
	build.Stdout, build.Stderr = &diagnostics, &diagnostics
	if err := build.Run(); err != nil {
		return updateFailure("build update", err, &diagnostics)
	}
	binary := "canvas-cli"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	built := filepath.Join(work, binary)
	check := updateCommand(built, "--help")
	check.Env = append(os.Environ(), "CANVAS_USAGE_LOG=0")
	check.Stdout, check.Stderr = &diagnostics, &diagnostics
	if err := check.Run(); err != nil {
		return updateFailure("verify update", err, &diagnostics)
	}
	if err := updateReplace(target, built); err != nil {
		return updateFailure("install update", err, &diagnostics)
	}
	_, _ = io.Copy(stderr, &diagnostics)
	return nil
}

func updateFailure(action string, err error, diagnostics *bytes.Buffer) error {
	if text := strings.TrimSpace(diagnostics.String()); text != "" {
		return fmt.Errorf("%s: %w: %s", action, err, text)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func replaceExecutable(target, built string) error {
	current, err := os.Stat(target)
	if err != nil {
		return err
	}
	src, err := os.Open(built)
	if err != nil {
		return err
	}
	defer src.Close()

	tmp, err := os.CreateTemp(filepath.Dir(target), ".canvas-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = io.Copy(tmp, src); err == nil {
		err = tmp.Chmod(current.Mode().Perm())
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}
