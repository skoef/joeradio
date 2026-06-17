package provider

import (
	"log/slog"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// make sure the logs are silenced during testing
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}
