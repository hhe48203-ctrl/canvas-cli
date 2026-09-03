package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/hhe48203-ctrl/canvas-cli/internal/api"
	"github.com/hhe48203-ctrl/canvas-cli/internal/canvas"
	"github.com/spf13/cobra"
)

const usageDailyLimit = 10 << 20

var errConfirmRequired = errors.New("this is a write operation; repeat with --confirm")

// ponytail: one invocation per process, like the command flags; use context if embedding needs concurrency.
var activeUsage *usageEvent

// Only allowlisted metadata belongs here: never arguments, URLs, or error text.
type usageEvent struct {
	Time        time.Time `json:"time"`
	Version     string    `json:"version"`
	Kind        string    `json:"kind"`
	Command     string    `json:"command"`
	OperationID string    `json:"operation_id,omitempty"`
	DurationMS  int64     `json:"duration_ms"`
	ExitCode    int       `json:"exit_code"`
	ErrorKind   string    `json:"error_kind,omitempty"`
	HTTPStatus  int       `json:"http_status,omitempty"`
	phase       string
}

func executeWithUsage(root *cobra.Command) error {
	if os.Getenv("CANVAS_USAGE_LOG") == "0" {
		return root.Execute()
	}
	start := time.Now()
	event := &usageEvent{Kind: "command", phase: "arguments"}
	activeUsage = event
	help := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		event.Kind = "help"
		help(cmd, args)
	})
	defer func() {
		activeUsage = nil
		root.SetHelpFunc(help)
	}()

	command, err := root.ExecuteC()
	event.DurationMS = time.Since(start).Milliseconds()
	event.Time = time.Now().UTC()
	if command == nil {
		command = root
	}
	event.Command = command.CommandPath()
	for c := command; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help":
			event.Kind = "help"
		case "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			event.Kind = "completion"
		}
	}
	if event.Kind == "command" && command.Parent() != nil && command.Parent().Name() == "api" &&
		(command.Name() == "invoke" || command.Name() == "describe") {
		if args := command.Flags().Args(); len(args) > 0 {
			if op, ok := api.Find(args[0]); ok {
				event.OperationID = op.ID
			}
		}
	}
	if err != nil {
		event.ExitCode = 1
		event.ErrorKind = event.phase
		var httpErr *canvas.HTTPError
		var netErr net.Error
		var fileErr *os.PathError
		switch {
		case errors.Is(err, errConfirmRequired):
			event.ErrorKind = "confirmation_required"
		case errors.As(err, &httpErr):
			event.ErrorKind, event.HTTPStatus = "http", httpErr.StatusCode
		case event.phase == "configuration":
		case errors.As(err, &fileErr):
			event.ErrorKind = "io"
		case errors.As(err, &netErr):
			event.ErrorKind = "network"
		}
	}
	info, _ := debug.ReadBuildInfo()
	event.Version = usageVersion(info)
	if cache, cacheErr := os.UserCacheDir(); cacheErr == nil {
		// Usage logging must not change command output or success/failure.
		_ = appendUsage(filepath.Join(cache, "canvas-cli", "logs"), *event)
	}
	return err
}

type usageTransport struct {
	http.RoundTripper
	event *usageEvent
}

func (t usageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if resp != nil {
		t.event.HTTPStatus = resp.StatusCode
	}
	return resp, err
}

func usageVersion(info *debug.BuildInfo) string {
	if info != nil {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}
	return "devel"
}

func appendUsage(dir string, event usageEvent) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("usage directory is not a directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	today := event.Time.UTC().Truncate(24 * time.Hour)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		date, err := time.Parse(time.DateOnly+".jsonl", entry.Name())
		if err == nil && entry.Type().IsRegular() && date.Before(today.AddDate(0, 0, -6)) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	path := filepath.Join(dir, today.Format(time.DateOnly)+".jsonl")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("usage log is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	info, err = file.Stat()
	if err != nil {
		return err
	}
	// ponytail: soft cap permits concurrent writers to overshoot; add locking only if an exact cap is needed.
	if info.Size() >= usageDailyLimit {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}
