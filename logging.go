package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"boot.dev/linko/internal/linkoerr"
	pkgerr "github.com/pkg/errors"
)

type stackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

type multiError interface {
	error
	Unwrap() []error
}

type closeFunc func() error

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == "error" {
		err, ok := a.Value.Any().(error)
		if !ok {
			return a
		}

		if multiErr, ok := errors.AsType[multiError](err); ok {
			var errAttrs []slog.Attr
			for i, err := range multiErr.Unwrap() {
				attrs := errorAttrs(err)
				errAttrs = append(errAttrs, slog.GroupAttrs(fmt.Sprintf("error_%d", i+1), attrs...))
			}
			return slog.GroupAttrs("errors", errAttrs...)
		}

		attrs := errorAttrs(err)
		return slog.GroupAttrs("error", attrs...)
	}

	return a
}

func errorAttrs(err error) []slog.Attr {
	var attrs []slog.Attr
	attrs = append(attrs, slog.Attr{
		Key:   "message",
		Value: slog.StringValue(err.Error()),
	})
	if stackErr, ok := errors.AsType[stackTracer](err); ok {
		attrs = append(attrs, slog.Attr{
			Key:   "stack_trace",
			Value: slog.StringValue(fmt.Sprintf("%+v", stackErr.StackTrace())),
		})
	}
	attrs = append(attrs, linkoerr.Attrs(err)...)
	return attrs
}

func initializeLogger() (*slog.Logger, closeFunc, error) {
	logFile := os.Getenv("LINKO_LOG_FILE")
	debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	})
	if logFile == "" {
		// function isn't nil so as to make sure it's callable.
		// making it nil puts the burden on the caller to check if closer is nil.
		// we don't want that.
		return slog.New(debugHandler), func() error { return nil }, nil
	}

	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}

	bufferedWriter := bufio.NewWriterSize(file, 8192)
	infoHandler := slog.NewJSONHandler(bufferedWriter, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: replaceAttr,
	})
	logger := slog.New(slog.NewMultiHandler(
		debugHandler,
		infoHandler,
	))

	var closer closeFunc = func() error {
		err := bufferedWriter.Flush()
		return err
	}
	return logger, closer, nil
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			logger.Info("Served request",
				"method", r.Method,
				"path", r.URL.Path,
				"client_ip", r.RemoteAddr,
			)
		})
	}

}
