package cmd

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/webhook"
)

// webhookPreRunTimestamp is captured in PersistentPreRun so we can query
// action_log entries created during the command.
var webhookPreRunTimestamp time.Time

// captureWebhookTimestamp saves the current time for later use by dispatchWebhookAsync.
func captureWebhookTimestamp() {
	webhookPreRunTimestamp = time.Now().UTC()
}

// dispatchWebhookAsync checks for new action_log entries since the pre-run
// timestamp, writes a temp file, and spawns a detached child process to POST
// the webhook. The parent does not wait for the child.
func dispatchWebhookAsync() {
	dir := getBaseDir()
	if dir == "" {
		return
	}

	if !webhook.IsEnabled(dir) {
		return
	}

	if webhookPreRunTimestamp.IsZero() {
		return
	}

	database, err := db.Open(dir)
	if err != nil {
		slog.Debug("webhook: open db", "err", err)
		return
	}
	defer database.Close()

	actions, err := database.GetActionsSince(webhookPreRunTimestamp)
	if err != nil {
		slog.Debug("webhook: query actions", "err", err)
		return
	}
	if len(actions) == 0 {
		return
	}

	payload := webhook.BuildPayload(dir, actions)
	tf := &webhook.TempFile{
		URL:     webhook.GetURL(dir),
		Secret:  webhook.GetSecret(dir),
		Payload: payload,
	}

	path, err := webhook.WriteTempFile(tf)
	if err != nil {
		slog.Debug("webhook: write temp file", "err", err)
		return
	}

	// Spawn detached child: td _webhook-send <tempfile>
	child := exec.Command(os.Args[0], "_webhook-send", path)
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Stdout = nil
	child.Stderr = nil
	child.Stdin = nil

	if err := child.Start(); err != nil {
		slog.Debug("webhook: spawn child", "err", err)
		os.Remove(path)
		return
	}

	slog.Debug("webhook: dispatched", "pid", child.Process.Pid, "actions", len(actions))
	// Don't wait — parent exits immediately.
}
