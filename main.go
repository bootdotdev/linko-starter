package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boot.dev/linko/internal/store"
	pkgerr "github.com/pkg/errors"
)

type stackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpPort := flag.Int("port", 8899, "port to listen on")
	dataDir := flag.String("data", "./data", "directory to store data")
	flag.Parse()

	status := run(ctx, cancel, *httpPort, *dataDir)
	cancel()
	os.Exit(status)
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == "error" {
		err, ok := a.Value.Any().(error)
		if !ok {
			return a
		}
		if stackErr, ok := errors.AsType[stackTracer](err); ok {
			return slog.GroupAttrs("error", slog.Attr{
				Key:   "message",
				Value: slog.StringValue(stackErr.Error()),
			}, slog.Attr{
				Key:   "stack_trace",
				Value: slog.StringValue(fmt.Sprintf("%+v", stackErr.StackTrace())),
			})
		}
	}
	return a
}

type closeFunc func() error

func initializeLogger(logfile string) (*slog.Logger, closeFunc, error) {

	debughandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: replaceAttr,
		Level:       slog.LevelDebug,
	})

	if env := os.Getenv("LINKO_LOG_FILE"); env != "" {
		file, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %w", err)
		}

		bufferedFile := bufio.NewWriterSize(file, 8192)
		infohandler := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{
			ReplaceAttr: replaceAttr,
			Level:       slog.LevelInfo,
		})
		//both stderr and file
		return slog.New(slog.NewMultiHandler(debughandler, infohandler)), func() error {
			if err := bufferedFile.Flush(); err != nil {
				file.Close()
				return err
			}
			return file.Close()
		}, nil
	}

	return slog.New(debughandler), func() error {
		return nil
	}, nil

}

func run(ctx context.Context, cancel context.CancelFunc, httpPort int, dataDir string) int {

	logFile := os.Getenv("LINKO_LOG_FILE")
	standardLogger, closeLogger, err := initializeLogger(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		return 1
	}

	closeLoggerWrapper := func() {
		if err := closeLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close logger: %v\n", err)

		}
	}
	defer closeLoggerWrapper()
	st, err := store.New(dataDir, standardLogger)
	if err != nil {

		standardLogger.Error("failed to create store", slog.String("error", fmt.Sprintf("%v", err)))
		return 1
	}
	s := newServer(*st, httpPort, cancel, standardLogger)
	var serverErr error
	go func() {
		serverErr = s.start()
	}()

	<-ctx.Done()
	standardLogger.Debug("Linko is shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if err := s.shutdown(shutdownCtx); err != nil {

		standardLogger.Error("server error", slog.String("error", fmt.Sprintf("%v", serverErr)))
		return 1
	}
	if serverErr != nil {

		standardLogger.Error("server error", slog.String("error", fmt.Sprintf("%v", serverErr)))
		return 1
	}
	return 0
}
