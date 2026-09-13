// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const wakePrompt = "hi"
const wakeRunTimeout = 120 * time.Second
const wakeMaxRuns = 200
const wakeMinIntervalMinutes = 15
const wakeMaxIntervalMinutes = 24 * 60

// WakeTask wakes an account on a schedule by running a tiny CLI request with
// the account's own credentials. This both verifies the account still works
// and starts its quota window early, so the window has reset by the time the
// account is actually needed.
type WakeTask struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	AccountID       string `json:"accountid"`
	IntervalMinutes int    `json:"intervalminutes"`
	Model           string `json:"model,omitempty"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       int64  `json:"createdat"`
	LastRun         int64  `json:"lastrun,omitempty"`
	LastStatus      string `json:"laststatus,omitempty"`
	LastError       string `json:"lasterror,omitempty"`
	LastDurationMs  int64  `json:"lastdurationms,omitempty"`
}

type WakeRun struct {
	TaskID     string `json:"taskid"`
	Provider   string `json:"provider"`
	AccountID  string `json:"accountid"`
	StartedAt  int64  `json:"startedat"`
	DurationMs int64  `json:"durationms"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	Output     string `json:"output,omitempty"`
}

type wakeFile struct {
	Version int        `json:"version"`
	Tasks   []WakeTask `json:"tasks"`
	Runs    []WakeRun  `json:"runs,omitempty"`
}

var wakeStoreLock sync.Mutex
var wakeRunLock sync.Mutex

func wakeFilePath() string {
	return filepath.Join(accountsDir(), "wake.json")
}

func loadWakeFileLocked() (*wakeFile, error) {
	data, err := os.ReadFile(wakeFilePath())
	if os.IsNotExist(err) {
		return &wakeFile{Version: 1, Tasks: []WakeTask{}, Runs: []WakeRun{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading wake store: %w", err)
	}
	var file wakeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("error parsing wake store: %w", err)
	}
	if file.Tasks == nil {
		file.Tasks = []WakeTask{}
	}
	return &file, nil
}

func saveWakeFileLocked(file *wakeFile) error {
	if len(file.Runs) > wakeMaxRuns {
		file.Runs = file.Runs[len(file.Runs)-wakeMaxRuns:]
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling wake store: %w", err)
	}
	return atomicWriteFile(wakeFilePath(), data, 0600)
}

func clampWakeInterval(minutes int) int {
	if minutes < wakeMinIntervalMinutes {
		return wakeMinIntervalMinutes
	}
	if minutes > wakeMaxIntervalMinutes {
		return wakeMaxIntervalMinutes
	}
	return minutes
}

func (m *Manager) ListWakeTasks(providerID string) ([]WakeTask, error) {
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	file, err := loadWakeFileLocked()
	if err != nil {
		return nil, err
	}
	rtn := []WakeTask{}
	for _, task := range file.Tasks {
		if providerID != "" && task.Provider != providerID {
			continue
		}
		rtn = append(rtn, task)
	}
	return rtn, nil
}

func (m *Manager) CreateWakeTask(providerID string, accountID string, intervalMinutes int, model string) (*WakeTask, error) {
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}
	if _, ok := provider.(WakeProvider); !ok {
		return nil, fmt.Errorf("%s does not support wake tasks", provider.Info().Name)
	}
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	if findAccount(idx, providerID, accountID) == nil {
		return nil, fmt.Errorf("account not found")
	}
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	file, err := loadWakeFileLocked()
	if err != nil {
		return nil, err
	}
	for _, existing := range file.Tasks {
		if existing.Provider == providerID && existing.AccountID == accountID {
			return nil, fmt.Errorf("this account already has a wake task")
		}
	}
	task := WakeTask{
		ID:              makeAccountID("wake", providerID+"|"+accountID+"|"+fmt.Sprintf("%d", nowMillis())),
		Provider:        providerID,
		AccountID:       accountID,
		IntervalMinutes: clampWakeInterval(intervalMinutes),
		Model:           strings.TrimSpace(model),
		Enabled:         true,
		CreatedAt:       nowMillis(),
	}
	file.Tasks = append(file.Tasks, task)
	if err := saveWakeFileLocked(file); err != nil {
		return nil, err
	}
	return &task, nil
}

func (m *Manager) UpdateWakeTask(taskID string, enabled bool, intervalMinutes int, model string) (*WakeTask, error) {
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	file, err := loadWakeFileLocked()
	if err != nil {
		return nil, err
	}
	for i := range file.Tasks {
		if file.Tasks[i].ID != taskID {
			continue
		}
		file.Tasks[i].Enabled = enabled
		file.Tasks[i].IntervalMinutes = clampWakeInterval(intervalMinutes)
		file.Tasks[i].Model = strings.TrimSpace(model)
		if err := saveWakeFileLocked(file); err != nil {
			return nil, err
		}
		task := file.Tasks[i]
		return &task, nil
	}
	return nil, fmt.Errorf("wake task not found")
}

func (m *Manager) DeleteWakeTask(taskID string) error {
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	file, err := loadWakeFileLocked()
	if err != nil {
		return err
	}
	tasks := file.Tasks[:0]
	found := false
	for _, task := range file.Tasks {
		if task.ID == taskID {
			found = true
			continue
		}
		tasks = append(tasks, task)
	}
	if !found {
		return fmt.Errorf("wake task not found")
	}
	file.Tasks = tasks
	runs := file.Runs[:0]
	for _, run := range file.Runs {
		if run.TaskID != taskID {
			runs = append(runs, run)
		}
	}
	file.Runs = runs
	return saveWakeFileLocked(file)
}

func (m *Manager) ListWakeRuns(taskID string, limit int) ([]WakeRun, error) {
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	file, err := loadWakeFileLocked()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rtn := []WakeRun{}
	for i := len(file.Runs) - 1; i >= 0 && len(rtn) < limit; i-- {
		if file.Runs[i].TaskID != taskID {
			continue
		}
		rtn = append(rtn, file.Runs[i])
	}
	return rtn, nil
}

func wakeTaskDue(task WakeTask, now time.Time) bool {
	if !task.Enabled {
		return false
	}
	interval := time.Duration(clampWakeInterval(task.IntervalMinutes)) * time.Minute
	if task.LastRun == 0 {
		return true
	}
	return now.Sub(time.UnixMilli(task.LastRun)) >= interval
}

// RunWakeTask runs one wake task synchronously (used by both the scheduler and
// the "Run now" button) and records the result in the history.
func (m *Manager) RunWakeTask(taskID string) (*WakeRun, error) {
	wakeStoreLock.Lock()
	file, err := loadWakeFileLocked()
	if err != nil {
		wakeStoreLock.Unlock()
		return nil, err
	}
	var task *WakeTask
	for i := range file.Tasks {
		if file.Tasks[i].ID == taskID {
			taskCopy := file.Tasks[i]
			task = &taskCopy
			break
		}
	}
	wakeStoreLock.Unlock()
	if task == nil {
		return nil, fmt.Errorf("wake task not found")
	}
	run := m.executeWakeRun(task)
	return run, nil
}

func (m *Manager) executeWakeRun(task *WakeTask) *WakeRun {
	wakeRunLock.Lock()
	defer wakeRunLock.Unlock()
	run := &WakeRun{
		TaskID:    task.ID,
		Provider:  task.Provider,
		AccountID: task.AccountID,
		StartedAt: nowMillis(),
	}
	start := time.Now()
	output, err := m.executeWake(task)
	run.DurationMs = time.Since(start).Milliseconds()
	run.Output = output
	if err != nil {
		run.Status = "error"
		run.Error = err.Error()
	} else {
		run.Status = "ok"
	}
	m.recordWakeRun(task.ID, run)
	// refresh quota right after the wake so the dashboard reflects the account
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m.RefreshQuota(ctx, task.Provider, task.AccountID)
	return run
}

func (m *Manager) recordWakeRun(taskID string, run *WakeRun) {
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	file, err := loadWakeFileLocked()
	if err != nil {
		return
	}
	for i := range file.Tasks {
		if file.Tasks[i].ID != taskID {
			continue
		}
		file.Tasks[i].LastRun = run.StartedAt
		file.Tasks[i].LastStatus = run.Status
		file.Tasks[i].LastError = run.Error
		file.Tasks[i].LastDurationMs = run.DurationMs
		break
	}
	file.Runs = append(file.Runs, *run)
	saveWakeFileLocked(file)
}

func (m *Manager) executeWake(task *WakeTask) (string, error) {
	provider, err := m.GetProvider(task.Provider)
	if err != nil {
		return "", err
	}
	wakeProvider, ok := provider.(WakeProvider)
	if !ok {
		return "", fmt.Errorf("%s does not support wake tasks", provider.Info().Name)
	}
	instanceProvider, ok := provider.(InstanceProvider)
	if !ok {
		return "", fmt.Errorf("%s does not support wake tasks", provider.Info().Name)
	}
	// Keep the stored credentials fresh before the wake uses them.
	m.syncActiveCredentialsToStore(provider)
	credentials, err := loadCredentials(task.Provider, task.AccountID)
	if err != nil {
		return "", err
	}
	idx, err := loadIndex()
	if err != nil {
		return "", err
	}
	account := findAccount(idx, task.Provider, task.AccountID)
	if account == nil {
		return "", fmt.Errorf("account not found")
	}
	wakeDir := filepath.Join(accountsDir(), "wake", task.Provider, task.AccountID)
	if err := os.MkdirAll(wakeDir, 0700); err != nil {
		return "", err
	}
	execPath, _, env, err := instanceProvider.InstancePrepare(account, credentials, wakeDir)
	if err != nil {
		return "", err
	}
	args := wakeProvider.WakeArgs(task.Model)
	return runWakeCommand(execPath, args, env, wakeDir)
}

func runWakeCommand(execPath string, args []string, env []string, dir string) (string, error) {
	if strings.TrimSpace(execPath) == "" {
		return "", fmt.Errorf("no CLI command configured for wake")
	}
	if _, err := exec.LookPath(execPath); err != nil {
		return "", fmt.Errorf("%s CLI not found in PATH", execPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wakeRunTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", append([]string{"/c", execPath}, args...)...)
	} else {
		cmd = exec.Command(execPath, args...)
	}
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		killProcessTree(cmd.Process.Pid)
		<-done
		return summarizeWakeOutput(output.String()), fmt.Errorf("wake timed out after %s", wakeRunTimeout)
	}
	snippet := summarizeWakeOutput(output.String())
	if waitErr != nil {
		if snippet != "" {
			return snippet, fmt.Errorf("%v: %s", waitErr, snippet)
		}
		return snippet, waitErr
	}
	return snippet, nil
}

// killProcessTree terminates a wake run and its children. On Windows the
// direct child is cmd.exe, so killing only it would leave the CLI running
// (and still consuming quota). taskkill failures — including "process not
// found" (exit code 128) — are intentionally ignored: the process being
// already gone is the desired end state, not an error (lesson from the
// cockpit-tools taskkill misclassification bug).
func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	if runtime.GOOS == "windows" {
		kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
		_ = kill.Run()
		return
	}
	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
	}
}

func summarizeWakeOutput(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if len(line) > 400 {
			line = line[:400]
		}
		return line
	}
	return ""
}

func (m *Manager) StartWakeScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Recover per tick: one panicking wake task must not stop the
				// scheduler for every other task.
				func() {
					defer func() {
						recover()
					}()
					m.runDueWakeTasks(ctx)
				}()
			}
		}
	}()
}

func (m *Manager) runDueWakeTasks(ctx context.Context) {
	file, err := loadWakeFile()
	if err != nil {
		return
	}
	now := time.Now()
	for _, task := range file.Tasks {
		if !wakeTaskDue(task, now) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		m.RunWakeTask(task.ID)
	}
}

func loadWakeFile() (*wakeFile, error) {
	wakeStoreLock.Lock()
	defer wakeStoreLock.Unlock()
	return loadWakeFileLocked()
}
