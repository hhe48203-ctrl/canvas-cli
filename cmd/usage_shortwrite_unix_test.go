//go:build darwin || linux

package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestUsageShortWrite(t *testing.T) {
	args := []string{"api", "describe", "courses.list", "--json"}
	if raw := os.Getenv("CANVAS_USAGE_TEST_FILE_LIMIT"); raw != "" {
		size, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		var limit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		limit.Cur = size
		signal.Ignore(syscall.SIGXFSZ)
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		rootCmd = newRootCommand()
		rootCmd.SetArgs(args)
		Execute()
		os.Exit(0)
	}
	for _, existing := range []bool{false, true} {
		t.Run(strconv.FormatBool(existing), func(t *testing.T) {
			dir := isolateUsage(t)
			var before []byte
			path := filepath.Join(dir, time.Now().UTC().Format(time.DateOnly)+".jsonl")
			wantCount := 1
			if existing {
				if _, _, exit := runUsage(t, true, args...); exit != 0 {
					t.Fatal("could not seed log")
				}
				var err error
				before, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				wantCount++
			}
			// Force a real partial file write, not a simulated error return.
			command := exec.Command(os.Args[0], "-test.run=^TestUsageShortWrite$")
			command.Env = append(os.Environ(), "CANVAS_USAGE_TEST_FILE_LIMIT="+strconv.Itoa(len(before)+100))
			out, err := command.CombinedOutput()
			wantOut, wantErr, wantExit := runUsage(t, false, args...)
			if err != nil || string(out) != wantOut || wantErr != "" || wantExit != 0 {
				t.Fatalf("short write changed command result: %s, %v", out, err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("failed write was not rolled back: %q, %v", after, err)
			}
			if _, _, exit := runUsage(t, true, args...); exit != 0 {
				t.Fatal("command after short write failed")
			}
			if events := readUsage(t, dir); len(events) != wantCount {
				t.Fatalf("got %d records after recovery, want %d", len(events), wantCount)
			}
		})
	}
}
