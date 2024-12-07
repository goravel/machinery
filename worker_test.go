package machinery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goravel/machinery"
)

func TestRedactURL(t *testing.T) {
	t.Parallel()

	broker := "redis://:password@localhost:6379/0"
	redactedURL := machinery.RedactURL(broker)
	assert.Equal(t, "redis://localhost:6379", redactedURL)
}

func BenchmarkRedactURL(b *testing.B) {
	broker := "redis://:password@localhost:6379/0"

	for i := 0; i < b.N; i++ {
		_ = machinery.RedactURL(broker)
	}
}

func TestPreConsumeHandler(t *testing.T) {
	t.Parallel()

	worker := &machinery.Worker{}

	worker.SetPreConsumeHandler(SamplePreConsumeHandler)
	assert.True(t, worker.PreConsumeHandler())
}

func SamplePreConsumeHandler(w *machinery.Worker) bool {
	return true
}
