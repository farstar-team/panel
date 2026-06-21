package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/farstar-team/panel/internal/config"
	"github.com/farstar-team/panel/internal/engine"
	"github.com/farstar-team/panel/internal/httpapi"
	"github.com/farstar-team/panel/internal/manager"
	"github.com/farstar-team/panel/internal/security"
	"github.com/farstar-team/panel/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve()
	case "setup":
		err = setup()
	case "run-tunnel":
		err = runTunnel()
	case "version", "--version", "-v":
		fmt.Println("Farstar Tunnel Panel", version)
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func serve() error {
	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	database, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	count, err := database.UserCount(context.Background())
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no admin account exists; pipe a strong password to: farstar setup --username admin --password-stdin")
	}
	vault, err := security.LoadOrCreateVault(cfg.MasterKey)
	if err != nil {
		return err
	}
	mgr := manager.New(database, cfg)
	if err := mgr.Reconcile(context.Background()); err != nil {
		log.Printf("runtime reconciliation warning: %v", err)
	}
	go mgr.StartAutostart(context.Background())
	go cleanupSessions(database)

	api := httpapi.New(database, vault, mgr, cfg)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	scheme := "http"
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		scheme = "https"
	}
	log.Printf("Farstar Tunnel Panel %s listening on %s://%s", version, scheme, cfg.ListenAddr)
	if scheme == "https" {
		err = server.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		err = server.ListenAndServe()
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func setup() error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	username := flags.String("username", "", "admin username")
	password := flags.String("password", "", "admin password (minimum 10 characters)")
	passwordStdin := flags.Bool("password-stdin", false, "read admin password from standard input")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *passwordStdin {
		value, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		*password = strings.TrimSpace(string(value))
	}
	if *username == "" || *password == "" {
		return fmt.Errorf("--username and either --password or --password-stdin are required")
	}
	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	database, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	count, err := database.UserCount(context.Background())
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("an admin account already exists")
	}
	hash, err := security.HashPassword(*password)
	if err != nil {
		return err
	}
	if _, err := security.LoadOrCreateVault(cfg.MasterKey); err != nil {
		return err
	}
	if err := database.CreateAdmin(context.Background(), *username, hash); err != nil {
		return err
	}
	fmt.Println("Farstar admin account created successfully.")
	return nil
}

func runTunnel() error {
	flags := flag.NewFlagSet("run-tunnel", flag.ContinueOnError)
	id := flags.String("id", "", "tunnel id")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	database, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	vault, err := security.LoadOrCreateVault(cfg.MasterKey)
	if err != nil {
		return err
	}
	tunnel, err := database.Tunnel(context.Background(), *id)
	if err != nil {
		return fmt.Errorf("load tunnel: %w", err)
	}
	secret, err := vault.Decrypt(tunnel.SecretCipher)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	runner := &engine.Runner{
		Store: database, Tunnel: tunnel, Secret: secret,
		Logger: log.New(os.Stdout, time.Now().Format("2006-01-02")+" ", log.LstdFlags|log.Lmicroseconds),
	}
	err = runner.Run(ctx)
	message := ""
	if err != nil && err != context.Canceled {
		message = err.Error()
	}
	_ = database.MarkStopped(context.Background(), tunnel.ID, message)
	return err
}

func cleanupSessions(database *store.Store) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		database.CleanupSessions(context.Background())
	}
}

func usage() {
	fmt.Println(`Farstar Tunnel Panel

Usage:
  farstar serve
  printf 'strong-password' | farstar setup --username admin --password-stdin
  farstar run-tunnel --id <tunnel-id>
  farstar version`)
}
