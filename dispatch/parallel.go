package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/lmittmann/tint"
	"github.com/nicois/dispatch"
)

// Exit codes. Anything nonzero means the run did not fully succeed.
const (
	exitSuccess = 0
	// exitFailure covers configuration problems and internal errors.
	exitFailure = 1
	// exitJobsFailed reports that dispatch itself worked, but some jobs failed.
	exitJobsFailed = 2
)

// Build information, populated by the linker at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var logger *slog.Logger

// options is the full set of command-line options: everything the library
// understands, plus those which only make sense for the CLI itself.
type options struct {
	dispatch.Opts
	Version bool `long:"version" description:"show the version and exit"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the real entrypoint, returning an exit code rather than terminating,
// so that it can be exercised by tests.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// collect command-line options
	var opts options
	parser := flags.NewParser(&opts, flags.Default)
	commandLine, err := parser.ParseArgs(args)
	if err != nil {
		// go-flags has already reported the problem, including for --help.
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			return exitSuccess
		}
		return exitFailure
	}

	if opts.Version {
		fmt.Fprintf(stdout, "dispatch %v\ncommit: %v\nbuilt: %v\n", version, commit, date)
		return exitSuccess
	}

	// set up the logger
	handlerOptions := tint.Options{}
	if opts.Debug {
		handlerOptions.Level = slog.LevelDebug
		handlerOptions.AddSource = true
	} else {
		handlerOptions.Level = slog.LevelInfo
	}
	logger = slog.New(tint.NewHandler(stdout, &handlerOptions))
	dispatch.SetLogger(logger)

	// listen for signals
	// to support escalation, do not simply use NotifyContext
	interruptChannel := make(chan os.Signal, 4)
	signal.Notify(interruptChannel, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(interruptChannel)

	// provide stub commands if required
	if len(commandLine) == 0 {
		if opts.CSV || opts.JsonLine {
			commandLine = []string{"echo", "foo is {{.foo}}, bar is {{.bar}}"}
		} else {
			commandLine = []string{"echo", "value is {{.value}}"}
		}
		logger.Info("no command was provided, so just echoing the input", slog.Any("commandline", commandLine))
	}

	ctx, cache, cleanup, err := openCache(context.Background(), opts.Opts)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		logger.Error("cannot initialise the cache", slog.Any("error", err))
		return exitFailure
	}

	// prepare for processing STDIN
	reader := bufio.NewReader(stdin)
	err = dispatch.PrepareAndRun(ctx, reader, opts.Opts, commandLine, cache, interruptChannel)

	switch {
	case err == nil:
		return exitSuccess
	case errors.Is(err, dispatch.ErrUserCancelled):
		// The user asked for this, so it is not an error worth reporting.
		return exitFailure
	case errors.Is(err, dispatch.ErrJobsFailed):
		// dispatch worked correctly; the jobs it ran did not.
		logger.Error(fmt.Sprintf("%v", err))
		return exitJobsFailed
	default:
		logger.Error(fmt.Sprintf("%v", err))
		return exitFailure
	}
}

// openCache builds the cache described by opts, returning the context the run
// should use. For an S3 cache with a known credential expiry, that context
// carries a deadline which stops the run before the credentials lapse.
func openCache(ctx context.Context, opts dispatch.Opts) (context.Context, dispatch.Cache, func(), error) {
	if opts.CacheLocation == nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return ctx, nil, nil, fmt.Errorf("cannot determine the home directory; use --cache-location: %w", err)
		}
		cache, err := dispatch.NewFileCache(filepath.Join(home, ".cache", "dispatch"))
		return ctx, cache, nil, err
	}

	if !strings.HasPrefix(*opts.CacheLocation, "s3://") {
		cache, err := dispatch.NewFileCache(*opts.CacheLocation)
		return ctx, cache, nil, err
	}

	cache, err := dispatch.NewS3Cache(ctx, *opts.CacheLocation)
	if err != nil {
		return ctx, nil, nil, fmt.Errorf("cannot initialise S3 cache: %w", err)
	}

	expiry := dispatch.GetS3ExpiryTime()
	if expiry == nil {
		return ctx, cache, nil, nil
	}

	// Stop before the credentials actually expire, so in-flight jobs can finish
	// and be recorded.
	safetyMargin := expiry.Add(-5 * time.Minute)
	if safetyMargin.Before(time.Now()) {
		return ctx, nil, nil, fmt.Errorf("too close to AWS token expiration: the safety margin ended at %v and the token expires at %v",
			safetyMargin, *expiry)
	}
	logger.Info("shutting down before the AWS token expires",
		slog.Time("shutdown time", safetyMargin),
		slog.Time("token expiry time", *expiry),
		slog.String("duration until safety margin is reached", dispatch.FriendlyDuration(time.Until(safetyMargin))))

	// The deadline is the safety margin, not the expiry itself.
	deadlineCtx, cancel := context.WithDeadlineCause(ctx, safetyMargin,
		errors.New("AWS token will expire soon"))
	return deadlineCtx, cache, cancel, nil
}
