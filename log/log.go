package log

import (
	"log/slog"
)

var (
	// DEBUG ...
	DEBUG = slog.Debug
	// INFO ...
	INFO = slog.Info
	// WARNING ...
	WARNING = slog.Warn
	// ERROR ...
	ERROR = slog.Error
)
