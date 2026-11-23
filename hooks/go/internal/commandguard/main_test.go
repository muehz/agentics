package commandguard

import (
	"os"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func TestMain(m *testing.M) {
	os.Exit(hosttest.MainWithIsolatedHome(m))
}
