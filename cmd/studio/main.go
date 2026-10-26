// Command studio runs Podcast AI Studio: a localhost-only web application for
// turning long podcast recordings into captioned short-form clips.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

var (
	shutdownOnce sync.Once
	shutdownCh   = make(chan struct{})
)

// requestShutdown asks the main loop to stop (used by /api/quit).
func requestShutdown() { shutdownOnce.Do(func() { close(shutdownCh) }) }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Podcast AI Studio:", err)
		os.Exit(1)
	}
}

func run() error {
	host := flag.String("host", "127.0.0.1", "loopback address to bind; other addresses are refused")
	port := flag.Int("port", 0, "HTTP port (0 picks a free port)")
	noBrowser := flag.Bool("no-browser", false, "do not open the app window")
	noToken := flag.Bool("no-token", false, "disable the per-launch session token (development only)")
	remotionRoot := flag.String("remotion", "", "path to the Remotion project folder (defaults to the one beside the executable)")
	stateDirFlag := flag.String("state-dir", "", "folder for settings, logs and cache (defaults to the user config directory)")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn or error")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Podcast AI Studio %s (%s, built %s)\n", version, commit, date)
		return nil
	}
	if ip := net.ParseIP(*host); ip == nil || !ip.IsLoopback() {
		return errors.New("only loopback binding is supported, to keep local video files private")
	}
	if *remotionRoot != "" {
		abs, err := filepath.Abs(*remotionRoot)
		if err != nil {
			return err
		}
		studio.SetRemotionRoot(abs)
	}

	stateDir := *stateDirFlag
	if stateDir == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			stateDir = filepath.Join(dir, "PodcastAIStudio")
		}
	}
	if stateDir != "" {
		if err := os.MkdirAll(stateDir, 0o700); err != nil {
			return fmt.Errorf("state folder: %w", err)
		}
	}
	logger, closeLog := newLogger(stateDir, *logLevel)
	defer closeLog()
	logger.Info("starting", "version", version, "commit", commit, "os", runtime.GOOS, "stateDir", stateDir)

	auth, err := newChatAuth(stateDir, logger)
	if err != nil {
		return fmt.Errorf("ChatGPT credential storage: %w", err)
	}
	a := newApplication(stateDir, auth, logger)
	if !*noToken {
		a.token = newToken()
	}
	a.loadState()
	studio.CleanStaleJobs()

	listener, err := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
	if err != nil {
		return fmt.Errorf("could not open a local port: %w", err)
	}
	allowed := allowedHostsFor(listener.Addr())
	base := "http://" + listener.Addr().String()
	launch := base + "/session?token=" + a.token
	if a.token == "" {
		launch = base
	}
	srv := &http.Server{Handler: a.routes(allowed), ReadHeaderTimeout: 8 * time.Second, IdleTimeout: 70 * time.Second}

	fmt.Println("Podcast AI Studio", version)
	fmt.Println("Open this address if the app window does not appear:")
	fmt.Println("  " + launch)
	logger.Info("listening", "address", base)
	if !*noBrowser {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openWindow(launch)
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(listener) }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("interrupt received; shutting down")
	case <-shutdownCh:
		logger.Info("quit requested from the app")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a.shutdown(shutdownCtx)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("server shutdown", "error", err)
	}
	logger.Info("stopped")
	return nil
}

func newToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// newLogger writes structured logs to studio.log in the state folder and
// mirrors warnings to stderr so the console stays readable.
func newLogger(stateDir, level string) (*slog.Logger, func()) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	writers := []io.Writer{os.Stderr}
	closer := func() {}
	if stateDir != "" {
		path := filepath.Join(stateDir, "studio.log")
		rotateLog(path, 4*1024*1024)
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			writers = append(writers, f)
			closer = func() { _ = f.Close() }
		}
	}
	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: lvl})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, closer
}

// rotateLog keeps one previous log file once the current one grows past max.
func rotateLog(path string, max int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < max {
		return
	}
	_ = os.Rename(path, path+".1")
}

// openWindow opens the app. On Windows an installed Edge gives an app-style
// window without bundling a browser; otherwise the default browser is used.
func openWindow(url string) {
	if runtime.GOOS == "windows" {
		for _, exe := range []string{
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		} {
			if _, err := os.Stat(exe); err == nil {
				if exec.Command(exe, "--app="+url, "--window-size=1440,900").Start() == nil {
					return
				}
			}
		}
	}
	openBrowser(url)
}
