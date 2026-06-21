package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/farstar-team/panel/internal/config"
	"github.com/farstar-team/panel/internal/store"
)

type Manager struct {
	store  *store.Store
	config config.Config
	mu     sync.Mutex
}

func New(s *store.Store, cfg config.Config) *Manager {
	return &Manager{store: s, config: cfg}
}

func (m *Manager) Reconcile(ctx context.Context) error {
	tunnels, err := m.store.ListTunnels(ctx)
	if err != nil {
		return err
	}
	for _, tunnel := range tunnels {
		if tunnel.PID > 0 && processAlive(tunnel.PID) {
			continue
		}
		if tunnel.Status != "stopped" || tunnel.PID != 0 {
			_ = m.store.MarkStopped(ctx, tunnel.ID, tunnel.LastError)
		}
	}
	return nil
}

func (m *Manager) StartAutostart(ctx context.Context) {
	tunnels, err := m.store.AutostartTunnels(ctx)
	if err != nil {
		log.Printf("load autostart tunnels: %v", err)
		return
	}
	for _, tunnel := range tunnels {
		if tunnel.PID > 0 && processAlive(tunnel.PID) {
			continue
		}
		if err := m.Start(ctx, tunnel.ID); err != nil {
			log.Printf("autostart %s: %v", tunnel.Name, err)
		}
	}
}

func (m *Manager) Start(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tunnel, err := m.store.Tunnel(ctx, id)
	if err != nil {
		return err
	}
	if tunnel.PID > 0 && processAlive(tunnel.PID) {
		return errors.New("tunnel is already running")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := filepath.Join(m.config.LogDir, id+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "run-tunnel", "--id", id)
	cmd.Env = append(os.Environ(), "FARSTAR_DATA_DIR="+m.config.DataDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := startDetached(cmd); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start tunnel process: %w", err)
	}
	_ = m.store.UpdateRuntime(ctx, id, store.Runtime{Status: "starting", PID: cmd.Process.Pid})
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		message := ""
		if err != nil {
			message = err.Error()
		}
		_ = m.store.MarkStopped(context.Background(), id, message)
	}()
	return nil
}

func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tunnel, err := m.store.Tunnel(ctx, id)
	if err != nil {
		return err
	}
	if tunnel.PID <= 0 || !processAlive(tunnel.PID) {
		return m.store.MarkStopped(ctx, id, "")
	}
	_ = m.store.UpdateRuntime(ctx, id, store.Runtime{
		Status:      "stopping",
		PID:         tunnel.PID,
		ActiveConns: tunnel.ActiveConns,
		BytesIn:     tunnel.BytesIn,
		BytesOut:    tunnel.BytesOut,
	})
	if err := stopProcess(tunnel.PID); err != nil {
		return err
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(tunnel.PID) {
			return m.store.MarkStopped(ctx, id, "")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := killProcess(tunnel.PID); err != nil {
		return err
	}
	return m.store.MarkStopped(ctx, id, "process required a forced stop")
}

func (m *Manager) Restart(ctx context.Context, id string) error {
	_ = m.Stop(ctx, id)
	return m.Start(ctx, id)
}

func (m *Manager) LogPath(id string) string {
	return filepath.Join(m.config.LogDir, id+".log")
}
