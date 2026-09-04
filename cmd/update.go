package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const updateModule = "github.com/hhe48203-ctrl/canvas-cli@main"

func updateCLI(stdout, stderr io.Writer) error {
	target, err := os.Executable()
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

	fmt.Fprintln(stderr, "Updating canvas...")
	build := exec.Command("go", "install", updateModule)
	build.Env = append(os.Environ(), "GOBIN="+work)
	build.Stdout, build.Stderr = stdout, stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build update: %w", err)
	}
	binary := "canvas-cli"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	built := filepath.Join(work, binary)
	check := exec.Command(built, "--help")
	check.Env = append(os.Environ(), "CANVAS_USAGE_LOG=0")
	if err := check.Run(); err != nil {
		return fmt.Errorf("verify update: %w", err)
	}
	if err := replaceExecutable(target, built); err != nil {
		return fmt.Errorf("install update: %w", err)
	}
	fmt.Fprintln(stderr, "Canvas updated successfully.")
	return nil
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
