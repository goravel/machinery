package utils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLockName(t *testing.T) {
	t.Parallel()

	lockName := GetLockName("test", "*/3 * * *")
	if runtime.GOOS == "windows" {
		assert.Equal(t, "machinery_lock_utils.test.exetest*/3 * * *", lockName)
	} else {
		assert.Equal(t, "machinery_lock_utils.testtest*/3 * * *", lockName)
	}
}
