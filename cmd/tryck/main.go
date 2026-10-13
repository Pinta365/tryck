// Command tryck is a static site generator for technical documentation.
package main

import (
	"context"
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
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/devserver"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/output"
)

// version is set at release time with -ldflags "-X main.version=v0.1.0".
var version = "dev"

// Exit codes (spec §11).
const (
	exitOK     = 0
	exitBuild  = 1 // build errors
	exitConfig = 2 // usage or config errors
)

const usage = `tryck builds static documentation sites from Markdown.

Usage:
  tryck build [--config tryck.config.yaml] [--output ./dist] [--drafts] [--base-url URL] [--workers N] [--verbose]
  tryck dev   [--config tryck.config.yaml] [--port 8080] [--host 127.0.0.1] [--open] [--verbose]
  tryck init  [dir]
  tryck version

Run "tryck <command> -h" for the flags of a command.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitConfig
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "build":
		return runBuild(rest, stdout, stderr)
	case "dev":
		return runDev(rest, stdout, stderr)
	case "init":
		return runInit(rest, stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "tryck %s\n", version)
		return exitOK
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "tryck: unknown command %q\n\n%s", cmd, usage)
		return exitConfig
	}
}

// newFlagSet returns a FlagSet that reports errors instead of exiting.
func newFlagSet(name, synopsis string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("tryck "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: tryck %s %s\n\nFlags:\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses args and maps -h to exit 0 and other errors to a
// usage error. It returns ok=false with the exit code when the command
// should stop.
func parseFlags(fs *flag.FlagSet, args []string, maxArgs int, stderr io.Writer) (code int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, false
		}
		return exitConfig, false
	}
	if fs.NArg() > maxArgs {
		fmt.Fprintf(stderr, "%s: unexpected argument %q\n", fs.Name(), fs.Arg(maxArgs))
		fs.Usage()
		return exitConfig, false
	}
	return 0, true
}

// setFlags returns the names of flags given explicitly on the command line.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

// loadConfig loads and validates the config, printing all diagnostics.
// It returns nil if the config has errors.
func loadConfig(path string, explicit bool, ov config.Overrides, stderr io.Writer) (*config.Config, []diag.Diagnostic) {
	cfg, ds := config.Load(path, explicit)
	if !diag.HasErrors(ds) {
		cfg.Apply(ov)
		ds = append(ds, cfg.Validate()...)
	}
	diag.Sort(ds)
	for _, d := range ds {
		fmt.Fprintln(stderr, diag.Format(d))
	}
	if diag.HasErrors(ds) {
		return nil, ds
	}
	return cfg, ds
}

func newLogger(stderr io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
}

func runBuild(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("build", "[flags]", stderr)
	configPath := fs.String("config", config.DefaultPath, "config file")
	outputDir := fs.String("output", "", "output directory (overrides build.outputDir)")
	drafts := fs.Bool("drafts", false, "include draft pages")
	baseURL := fs.String("base-url", "", "site base URL (overrides site.baseUrl)")
	workers := fs.Int("workers", 0, "parallel workers, 0 = number of CPUs (overrides build.workers)")
	verbose := fs.Bool("verbose", false, "debug logging with per-stage timings")
	if code, ok := parseFlags(fs, args, 0, stderr); !ok {
		return code
	}

	set := setFlags(fs)
	var ov config.Overrides
	if set["output"] {
		ov.OutputDir = outputDir
	}
	if set["drafts"] {
		ov.Drafts = drafts
	}
	if set["base-url"] {
		ov.BaseURL = baseURL
	}
	if set["workers"] {
		ov.Workers = workers
	}

	log := newLogger(stderr, *verbose)
	cfg, cfgDiags := loadConfig(*configPath, set["config"], ov, stderr)
	if cfg == nil {
		return exitConfig
	}
	log.Debug("config loaded", "path", cfg.Path, "root", cfg.Root, "baseUrl", cfg.Site.BaseURL, "workers", cfg.WorkerCount())

	start := time.Now()
	site, err := build.Build(context.Background(), cfg, build.Options{Logger: log})
	if err != nil {
		fmt.Fprintf(stderr, "tryck build: %v\n", err)
		return exitBuild
	}
	for _, d := range site.Diags {
		if d.Severity > diag.Info || *verbose {
			fmt.Fprintln(stderr, diag.Format(d))
		}
	}
	log.Debug("site built", "pages", len(site.Pages), "duration", time.Since(start))

	var st output.Stats
	if !site.HasErrors() {
		emitStart := time.Now()
		st, err = output.Write(site)
		if err != nil {
			fmt.Fprintf(stderr, "tryck build: %v\n", err)
			return exitBuild
		}
		log.Debug("stage done", "stage", "emit", "duration", time.Since(emitStart), "files", st.Pages+st.Copied)
	}

	warnings, errs := 0, 0
	for _, d := range append(cfgDiags, site.Diags...) {
		switch d.Severity {
		case diag.Warning:
			warnings++
		case diag.Error:
			errs++
		}
	}
	fmt.Fprintf(stdout, "Built %d pages, copied %d files, %d warnings, %d errors in %s\n",
		len(site.Pages), st.Copied, warnings, errs, roundDuration(time.Since(start)))
	if errs > 0 {
		return exitBuild
	}
	return exitOK
}

func runDev(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("dev", "[flags]", stderr)
	configPath := fs.String("config", config.DefaultPath, "config file")
	port := fs.Int("port", 8080, "port to listen on")
	host := fs.String("host", "127.0.0.1", "host to bind to")
	open := fs.Bool("open", false, "open the site in a browser")
	verbose := fs.Bool("verbose", false, "debug logging and info diagnostics")
	if code, ok := parseFlags(fs, args, 0, stderr); !ok {
		return code
	}
	if *port < 1 || *port > 65535 {
		fmt.Fprintf(stderr, "tryck dev: invalid port %d\n", *port)
		return exitConfig
	}

	// dev always includes drafts and serves from its own address (spec §5.4, §12).
	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
	siteURL := "http://" + addr + "/"
	ov := config.Overrides{BaseURL: ptr(siteURL), Drafts: ptr(true)}
	explicit := setFlags(fs)["config"]
	cfg, _ := loadConfig(*configPath, explicit, ov, stderr)
	if cfg == nil {
		return exitConfig
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "tryck dev: %v\n", err)
		return exitBuild
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgFile := *configPath
	if cfg.Path == "" && !explicit {
		cfgFile = config.DefaultPath // not there yet; picked up if created
	}
	srv := devserver.New(cfg, devserver.Options{
		ConfigPath:     cfgFile,
		ConfigExplicit: explicit,
		Overrides:      ov,
		Out:            stderr,
		Logger:         newLogger(stderr, *verbose),
		Verbose:        *verbose,
	})
	start := time.Now()
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "tryck dev: %v\n", err)
		return exitBuild
	}
	fmt.Fprintf(stdout, "Built in %s. Serving %s (Ctrl+C to stop)\n", roundDuration(time.Since(start)), siteURL)

	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdown)
	}()
	if *open {
		openBrowser(siteURL)
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "tryck dev: %v\n", err)
		return exitBuild
	}
	<-srv.Done()
	return exitOK
}

// openBrowser opens url with the platform's default handler.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func runInit(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("init", "[dir]", stderr)
	if code, ok := parseFlags(fs, args, 1, stderr); !ok {
		return code
	}
	fmt.Fprintln(stderr, "tryck init: not implemented yet")
	return exitOK
}

// roundDuration keeps build times readable: 412µs, 37ms, 1.2s.
func roundDuration(d time.Duration) time.Duration {
	switch {
	case d < time.Millisecond:
		return d.Round(time.Microsecond)
	case d < time.Second:
		return d.Round(time.Millisecond)
	default:
		return d.Round(100 * time.Millisecond)
	}
}

func ptr[T any](v T) *T { return &v }
