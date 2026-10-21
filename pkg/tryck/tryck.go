// Package tryck is the public entry point of the tryck command line. A
// custom binary that compiles in extra plugins is a single call:
//
//	func main() { tryck.Main(&myplugin.Plugin{}) }
package tryck

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
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	starter "github.com/Pinta365/tryck"
	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/devserver"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/output"
	"github.com/Pinta365/tryck/internal/scaffold"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// modulePath is the module path Go records in build info.
const modulePath = "github.com/Pinta365/tryck"

// version is set at release time with
// -ldflags "-X github.com/Pinta365/tryck/pkg/tryck.version=v0.1.0".
var version = "dev"

// Exit codes.
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
  tryck snapshot [--config tryck.config.yaml] <name>
  tryck version

Run "tryck <command> -h" for the flags of a command.
`

// Main runs the tryck command line with plugins compiled in next to the
// built-ins, and exits. A custom binary's main is one call:
//
//	func main() { tryck.Main(&myplugin.Plugin{}) }
//
// Each plugin value is reused for every build, including each rebuild in
// tryck dev, so it should reset per-build state in Init. OnPageParsed and
// OnPageRendered run on several pages at once.
func Main(plugins ...plugin.Plugin) {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr, plugins...))
}

// Run runs the command line with args (without the program name) and
// returns the exit code: 0 ok, 1 build errors, 2 usage or config errors.
func Run(args []string, stdout, stderr io.Writer, plugins ...plugin.Plugin) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitConfig
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "build":
		return runBuild(rest, stdout, stderr, plugins)
	case "dev":
		return runDev(rest, stdout, stderr, plugins)
	case "init":
		return runInit(rest, stdout, stderr)
	case "snapshot":
		return runSnapshot(rest, stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "tryck %s\n", resolveVersion(version, debug.ReadBuildInfo))
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

func runBuild(args []string, stdout, stderr io.Writer, plugins []plugin.Plugin) int {
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
	site, err := build.Build(context.Background(), cfg, build.Options{Logger: log, Plugins: plugins})
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

func runDev(args []string, stdout, stderr io.Writer, plugins []plugin.Plugin) int {
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

	// dev always includes drafts and serves from its own address, so it
	// never needs a correct site.baseUrl.
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
		Plugins:        plugins,
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
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	files, err := scaffold.Init(dir, starter.Starter())
	if err != nil {
		fmt.Fprintf(stderr, "tryck init: %v\n", err)
		return exitBuild
	}
	fmt.Fprintf(stdout, "Created %d files in %s\n\nNext:\n", len(files), dir)
	if dir != "." {
		fmt.Fprintf(stdout, "  cd %s\n", dir)
	}
	fmt.Fprintln(stdout, "  tryck dev")
	return exitOK
}

// runSnapshot copies the current content to versions/<name>, making it
// an older version, and prints the config to add. It does not edit the
// config file, which keeps its comments and layout.
func runSnapshot(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("snapshot", "[flags] <name>", stderr)
	configPath := fs.String("config", config.DefaultPath, "config file")
	if code, ok := parseFlags(fs, args, 1, stderr); !ok {
		return code
	}
	name := fs.Arg(0)
	if !config.ValidVersionName(name) {
		fmt.Fprintf(stderr, "tryck snapshot: give the version a name of letters, digits, '.', '-' and '_', such as v1.2\n")
		return exitConfig
	}
	cfg, _ := loadConfig(*configPath, setFlags(fs)["config"], config.Overrides{}, stderr)
	if cfg == nil {
		return exitConfig
	}
	for _, v := range cfg.Versions.Older {
		if v.Name == name {
			fmt.Fprintf(stderr, "tryck snapshot: %s is already an older version in %s\n", name, *configPath)
			return exitConfig
		}
	}
	src := cfg.Dir(cfg.Build.ContentDir)
	dst := filepath.Join("versions", name)
	if _, err := os.Stat(cfg.Dir(dst)); err == nil {
		fmt.Fprintf(stderr, "tryck snapshot: %s already exists\n", filepath.ToSlash(dst))
		return exitBuild
	}
	files, err := scaffold.Init(cfg.Dir(dst), os.DirFS(src))
	if err != nil {
		fmt.Fprintf(stderr, "tryck snapshot: %v\n", err)
		return exitBuild
	}
	contentDir := filepath.ToSlash(filepath.Clean(cfg.Build.ContentDir)) + "/"
	fmt.Fprintf(stdout, "Copied %s to %s/ (%d files).\n\n", contentDir, filepath.ToSlash(dst), len(files))
	fmt.Fprintf(stdout, "Add it to %s as the newest older version, and set current to the\nversion %s holds from now on:\n\nversions:\n  current: <next version>\n  older:\n    - name: %s\n",
		*configPath, contentDir, name)
	for _, v := range cfg.Versions.Older {
		fmt.Fprintf(stdout, "    - name: %s\n", v.Name)
		if v.Dir != "" {
			fmt.Fprintf(stdout, "      dir: %s\n", v.Dir)
		}
	}
	return exitOK
}

// resolveVersion returns the version stamped by the release build, or
// else the module version Go records for go install, or else the version
// of tryck embedded by a custom binary, or else "dev" for local builds.
func resolveVersion(stamped string, buildInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != "dev" {
		return stamped
	}
	info, ok := buildInfo()
	if !ok {
		return stamped
	}
	if info.Main.Path == modulePath {
		if v := releaseVersion(info.Main.Version); v != "" {
			return v
		}
	}
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			if v := releaseVersion(dep.Version); v != "" {
				return v
			}
		}
	}
	return stamped
}

// releaseVersion drops the empty and "(devel)" versions Go reports for
// local builds.
func releaseVersion(v string) string {
	if v == "" || v == "(devel)" {
		return ""
	}
	return v
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
