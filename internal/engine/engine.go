package engine

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"

	"github.com/farstar-team/panel/internal/security"
	"github.com/farstar-team/panel/internal/store"
)

type Runner struct {
	Store  *store.Store
	Tunnel store.Tunnel
	Secret string
	Logger *log.Logger

	activeConns atomic.Int64
	bytesIn     atomic.Int64
	bytesOut    atomic.Int64
	lastError   atomic.Value
}

func (r *Runner) Run(ctx context.Context) error {
	if err := Validate(r.Tunnel, r.Secret); err != nil {
		return err
	}
	pid := os.Getpid()
	_ = r.Store.UpdateRuntime(ctx, r.Tunnel.ID, store.Runtime{Status: "starting", PID: pid})

	metricsCtx, cancelMetrics := context.WithCancel(ctx)
	defer cancelMetrics()
	go r.publishMetrics(metricsCtx, pid)

	var err error
	switch r.Tunnel.Role {
	case "server":
		err = r.runServer(ctx)
	case "client":
		err = r.runClient(ctx)
	default:
		err = fmt.Errorf("unsupported role %q", r.Tunnel.Role)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		r.setError(err)
		return err
	}
	return nil
}

func Validate(t store.Tunnel, secret string) error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("tunnel name is required")
	}
	if t.Role != "server" && t.Role != "client" {
		return errors.New("role must be server or client")
	}
	if t.Protocol != "tcp" && t.Protocol != "wss" {
		return errors.New("protocol must be tcp or wss")
	}
	if len(secret) < 16 {
		return errors.New("tunnel secret must contain at least 16 characters")
	}
	if t.Role == "server" {
		if _, err := net.ResolveTCPAddr("tcp", t.ListenAddr); err != nil {
			return fmt.Errorf("invalid listen address: %w", err)
		}
		if len(t.PublicPorts) == 0 || len(t.PublicPorts) > math.MaxUint16 {
			return errors.New("server requires 1 to 65535 public listeners")
		}
		for _, addr := range t.PublicPorts {
			if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
				return fmt.Errorf("invalid public listener %q: %w", addr, err)
			}
		}
		if t.Protocol == "wss" && (t.TLSCert == "" || t.TLSKey == "") {
			return errors.New("WSS server requires TLS certificate and key paths")
		}
	} else {
		if strings.TrimSpace(t.RemoteAddr) == "" {
			return errors.New("client remote address is required")
		}
		if len(t.LocalServices) == 0 {
			return errors.New("client requires at least one local service")
		}
		for _, addr := range t.LocalServices {
			if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
				return fmt.Errorf("invalid local service %q: %w", addr, err)
			}
		}
	}
	return nil
}

func (r *Runner) runServer(ctx context.Context) error {
	active := &activeSession{}
	var listeners []net.Listener
	for index, address := range r.Tunnel.PublicPorts {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			closeListeners(listeners)
			return fmt.Errorf("listen on public address %s: %w", address, err)
		}
		listeners = append(listeners, listener)
		go r.acceptPublic(ctx, listener, uint16(index), active)
		r.Logger.Printf("public listener ready: %s -> mapping %d", address, index)
	}
	defer closeListeners(listeners)
	_ = r.Store.UpdateRuntime(ctx, r.Tunnel.ID, store.Runtime{Status: "running", PID: os.Getpid()})

	var err error
	switch r.Tunnel.Protocol {
	case "tcp":
		err = r.serveTCP(ctx, active)
	case "wss":
		err = r.serveWSS(ctx, active)
	}
	return err
}

func (r *Runner) serveTCP(ctx context.Context, active *activeSession) error {
	listener, err := net.Listen("tcp", r.Tunnel.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen for TCP tunnel: %w", err)
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	r.Logger.Printf("TCP tunnel listener ready: %s", r.Tunnel.ListenAddr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.Logger.Printf("tunnel accept failed: %v", err)
			continue
		}
		go func() {
			if err := authenticateServer(conn, r.Secret); err != nil {
				r.Logger.Printf("TCP authentication failed from %s: %v", conn.RemoteAddr(), err)
				_ = conn.Close()
				return
			}
			r.attachSession(ctx, conn, active)
		}()
	}
}

func (r *Runner) serveWSS(ctx context.Context, active *activeSession) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", func(w http.ResponseWriter, req *http.Request) {
		if !security.SecureEqual([]byte(req.Header.Get("Authorization")), []byte("Bearer "+r.Secret)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ws, err := websocket.Accept(w, req, &websocket.AcceptOptions{
			Subprotocols: []string{"farstar-tunnel-v1"},
		})
		if err != nil {
			r.Logger.Printf("websocket upgrade failed: %v", err)
			return
		}
		conn := websocket.NetConn(context.Background(), ws, websocket.MessageBinary)
		go r.attachSession(ctx, conn, active)
	})
	server := &http.Server{
		Addr:              r.Tunnel.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	r.Logger.Printf("WSS tunnel listener ready: %s/tunnel", r.Tunnel.ListenAddr)
	err := server.ListenAndServeTLS(r.Tunnel.TLSCert, r.Tunnel.TLSKey)
	if errors.Is(err, http.ErrServerClosed) {
		return ctx.Err()
	}
	return err
}

func (r *Runner) attachSession(ctx context.Context, conn net.Conn, active *activeSession) {
	config := yamux.DefaultConfig()
	config.EnableKeepAlive = true
	config.KeepAliveInterval = 15 * time.Second
	config.ConnectionWriteTimeout = 20 * time.Second
	config.MaxStreamWindowSize = 4 * 1024 * 1024
	session, err := yamux.Server(conn, config)
	if err != nil {
		r.Logger.Printf("create yamux server session: %v", err)
		_ = conn.Close()
		return
	}
	if !active.TrySet(session) {
		r.Logger.Printf("rejected an additional tunnel client")
		_ = session.Close()
		return
	}
	r.Logger.Printf("tunnel client connected: %s", conn.RemoteAddr())
	select {
	case <-session.CloseChan():
	case <-ctx.Done():
		_ = session.Close()
	}
	active.Clear(session)
	r.Logger.Printf("tunnel client disconnected")
}

func (r *Runner) acceptPublic(ctx context.Context, listener net.Listener, index uint16, active *activeSession) {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				r.Logger.Printf("public accept failed on %s: %v", listener.Addr(), err)
			}
			return
		}
		go r.forwardPublic(conn, index, active)
	}
}

func (r *Runner) forwardPublic(public net.Conn, index uint16, active *activeSession) {
	defer public.Close()
	session := active.Get()
	if session == nil || session.IsClosed() {
		return
	}
	stream, err := session.OpenStream()
	if err != nil {
		r.Logger.Printf("open mux stream: %v", err)
		return
	}
	defer stream.Close()
	var header [2]byte
	binary.BigEndian.PutUint16(header[:], index)
	_ = stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := writeFull(stream, header[:]); err != nil {
		return
	}
	_ = stream.SetWriteDeadline(time.Time{})
	r.activeConns.Add(1)
	defer r.activeConns.Add(-1)
	copyBoth(public, stream, &r.bytesIn, &r.bytesOut)
}

func (r *Runner) runClient(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := r.dialServer(ctx)
		if err != nil {
			r.setError(err)
			r.Logger.Printf("connection failed: %v; retrying in %s", err, backoff)
			if !sleepContext(ctx, backoff+time.Duration(mathrand.IntN(500))*time.Millisecond) {
				return ctx.Err()
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		if err := r.serveClientSession(ctx, conn); err != nil && ctx.Err() == nil {
			r.setError(err)
			r.Logger.Printf("session ended: %v", err)
		}
	}
}

func (r *Runner) dialServer(ctx context.Context) (net.Conn, error) {
	switch r.Tunnel.Protocol {
	case "tcp":
		dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", r.Tunnel.RemoteAddr)
		if err != nil {
			return nil, err
		}
		if err := authenticateClient(conn, r.Secret); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	case "wss":
		tlsConfig, err := r.clientTLSConfig()
		if err != nil {
			return nil, err
		}
		httpClient := &http.Client{Transport: &http.Transport{
			TLSClientConfig:       tlsConfig,
			TLSHandshakeTimeout:   15 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		}}
		header := http.Header{"Authorization": []string{"Bearer " + r.Secret}}
		dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		ws, _, err := websocket.Dial(dialCtx, r.Tunnel.RemoteAddr, &websocket.DialOptions{
			HTTPClient:   httpClient,
			HTTPHeader:   header,
			Subprotocols: []string{"farstar-tunnel-v1"},
		})
		if err != nil {
			return nil, err
		}
		return websocket.NetConn(context.Background(), ws, websocket.MessageBinary), nil
	default:
		return nil, errors.New("unsupported protocol")
	}
}

func (r *Runner) clientTLSConfig() (*tls.Config, error) {
	config := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         r.Tunnel.TLSServerName,
		InsecureSkipVerify: r.Tunnel.SkipTLSVerify, // Explicit opt-in, surfaced as unsafe in the UI.
	}
	if r.Tunnel.TLSCACert != "" {
		pemBytes, err := os.ReadFile(r.Tunnel.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, errors.New("CA certificate contains no valid certificates")
		}
		config.RootCAs = pool
	}
	return config, nil
}

func (r *Runner) serveClientSession(ctx context.Context, conn net.Conn) error {
	defer conn.Close()
	config := yamux.DefaultConfig()
	config.EnableKeepAlive = true
	config.KeepAliveInterval = 15 * time.Second
	config.ConnectionWriteTimeout = 20 * time.Second
	config.MaxStreamWindowSize = 4 * 1024 * 1024
	session, err := yamux.Client(conn, config)
	if err != nil {
		return err
	}
	defer session.Close()
	_ = r.Store.UpdateRuntime(ctx, r.Tunnel.ID, store.Runtime{Status: "running", PID: os.Getpid()})
	r.lastError.Store("")
	r.Logger.Printf("tunnel session established")
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return err
		}
		go r.forwardLocal(stream)
	}
}

func (r *Runner) forwardLocal(stream *yamux.Stream) {
	defer stream.Close()
	var header [2]byte
	_ = stream.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(stream, header[:]); err != nil {
		r.Logger.Printf("read stream mapping: %v", err)
		return
	}
	_ = stream.SetReadDeadline(time.Time{})
	index := int(binary.BigEndian.Uint16(header[:]))
	if index >= len(r.Tunnel.LocalServices) {
		r.Logger.Printf("invalid stream mapping %d", index)
		return
	}
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	local, err := dialer.Dial("tcp", r.Tunnel.LocalServices[index])
	if err != nil {
		r.Logger.Printf("local service %s unavailable: %v", r.Tunnel.LocalServices[index], err)
		return
	}
	defer local.Close()
	r.activeConns.Add(1)
	defer r.activeConns.Add(-1)
	copyBoth(local, stream, &r.bytesOut, &r.bytesIn)
}

func authenticateServer(conn net.Conn, secret string) error {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetDeadline(time.Time{})
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	if err := writeFull(conn, nonce); err != nil {
		return err
	}
	response := make([]byte, 32)
	if _, err := io.ReadFull(conn, response); err != nil {
		return err
	}
	if !security.SecureEqual(response, security.ChallengeMAC(secret, nonce)) {
		return errors.New("invalid challenge response")
	}
	return nil
}

func authenticateClient(conn net.Conn, secret string) error {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetDeadline(time.Time{})
	nonce := make([]byte, 32)
	if _, err := io.ReadFull(conn, nonce); err != nil {
		return err
	}
	return writeFull(conn, security.ChallengeMAC(secret, nonce))
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func copyBoth(a, b net.Conn, aToB, bToA *atomic.Int64) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.CopyBuffer(countingWriter{Writer: b, Counter: aToB}, a, make([]byte, 32*1024))
		closeWriteOrConn(b)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.CopyBuffer(countingWriter{Writer: a, Counter: bToA}, b, make([]byte, 32*1024))
		closeWriteOrConn(a)
	}()
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

func closeWriteOrConn(conn net.Conn) {
	if half, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
		return
	}
	_ = conn.Close()
}

type countingWriter struct {
	io.Writer
	Counter *atomic.Int64
}

func (w countingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.Counter.Add(int64(n))
	return n, err
}

type activeSession struct {
	mu      sync.RWMutex
	session *yamux.Session
}

func (a *activeSession) TrySet(session *yamux.Session) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session != nil && !a.session.IsClosed() {
		return false
	}
	a.session = session
	return true
}

func (a *activeSession) Get() *yamux.Session {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.session
}

func (a *activeSession) Clear(session *yamux.Session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session == session {
		a.session = nil
	}
}

func (r *Runner) publishMetrics(ctx context.Context, pid int) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastError, _ := r.lastError.Load().(string)
			_ = r.Store.UpdateRuntime(context.Background(), r.Tunnel.ID, store.Runtime{
				Status:      "running",
				PID:         pid,
				ActiveConns: r.activeConns.Load(),
				BytesIn:     r.bytesIn.Load(),
				BytesOut:    r.bytesOut.Load(),
				LastError:   lastError,
			})
		}
	}
}

func (r *Runner) setError(err error) {
	if err != nil {
		r.lastError.Store(err.Error())
	}
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
