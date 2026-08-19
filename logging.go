package main

import (
	"bufio"
	"log/slog"
	"net/http"
	"os"
)

type closeFunc func() error

func initializeLogger() (*slog.Logger, closeFunc, error) {
	logFile := os.Getenv("LINKO_LOG_FILE")
	debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
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
		Level: slog.LevelInfo,
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
