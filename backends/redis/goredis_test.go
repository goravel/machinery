package redis_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goravel/machinery/backends/iface"
	"github.com/goravel/machinery/backends/redis"
	"github.com/goravel/machinery/config"
	"github.com/goravel/machinery/tasks"
)

func getRedisG() iface.Backend {
	// host1:port1,host2:port2
	redisURL := os.Getenv("REDIS_URL_GR")
	//redisPassword := os.Getenv("REDIS_PASSWORD")
	if redisURL == "" {
		return nil
	}
	backend := redis.New(new(config.Config), strings.Split(redisURL, ","), 0)
	return backend
}

func TestGroupCompletedGR(t *testing.T) {
	backend := getRedisG()
	if backend == nil {
		t.Skip()
	}

	groupUUID := "testGroupUUID"
	task1 := &tasks.Signature{
		UUID:      "testTaskUUID1",
		GroupUUID: groupUUID,
	}
	task2 := &tasks.Signature{
		UUID:      "testTaskUUID2",
		GroupUUID: groupUUID,
	}

	// Cleanup before the test
	assert.NoError(t, backend.PurgeState(task1.UUID))
	assert.NoError(t, backend.PurgeState(task2.UUID))
	assert.NoError(t, backend.PurgeGroupMeta(groupUUID))

	groupCompleted, err := backend.GroupCompleted(groupUUID, 2)
	if assert.Error(t, err) {
		assert.False(t, groupCompleted)
		assert.Equal(t, "redis: nil", err.Error())
	}

	assert.NoError(t, backend.InitGroup(groupUUID, []string{task1.UUID, task2.UUID}))

	groupCompleted, err = backend.GroupCompleted(groupUUID, 2)
	if assert.Error(t, err) {
		assert.False(t, groupCompleted)
		assert.Equal(t, "redis: nil", err.Error())
	}

	assert.NoError(t, backend.SetStatePending(task1))
	assert.NoError(t, backend.SetStateStarted(task2))
	groupCompleted, err = backend.GroupCompleted(groupUUID, 2)
	if assert.NoError(t, err) {
		assert.False(t, groupCompleted)
	}

	taskResults := []*tasks.TaskResult{new(tasks.TaskResult)}
	assert.NoError(t, backend.SetStateStarted(task1))
	assert.NoError(t, backend.SetStateSuccess(task2, taskResults))
	groupCompleted, err = backend.GroupCompleted(groupUUID, 2)
	if assert.NoError(t, err) {
		assert.False(t, groupCompleted)
	}

	assert.NoError(t, backend.SetStateFailure(task1, "Some error"))
	groupCompleted, err = backend.GroupCompleted(groupUUID, 2)
	if assert.NoError(t, err) {
		assert.True(t, groupCompleted)
	}
}

func TestGetStateGR(t *testing.T) {
	backend := getRedisG()
	if backend == nil {
		t.Skip()
	}

	signature := &tasks.Signature{
		UUID:      "testTaskUUID",
		GroupUUID: "testGroupUUID",
	}

	assert.NoError(t, backend.PurgeState("testTaskUUID"))

	var (
		taskState *tasks.TaskState
		err       error
	)

	taskState, err = backend.GetState(signature.UUID)
	assert.Error(t, err)
	assert.Nil(t, taskState)

	//Pending State
	assert.NoError(t, backend.SetStatePending(signature))
	taskState, err = backend.GetState(signature.UUID)
	assert.NoError(t, err)
	assert.Equal(t, signature.Name, taskState.TaskName)
	createdAt := taskState.CreatedAt

	//Received State
	assert.NoError(t, backend.SetStateReceived(signature))
	taskState, err = backend.GetState(signature.UUID)
	assert.NoError(t, err)
	assert.Equal(t, signature.Name, taskState.TaskName)
	assert.Equal(t, createdAt, taskState.CreatedAt)

	//Started State
	assert.NoError(t, backend.SetStateStarted(signature))
	taskState, err = backend.GetState(signature.UUID)
	assert.NoError(t, err)
	assert.Equal(t, signature.Name, taskState.TaskName)
	assert.Equal(t, createdAt, taskState.CreatedAt)

	//Success State
	taskResults := []*tasks.TaskResult{
		{
			Type:  "float64",
			Value: 2,
		},
	}
	assert.NoError(t, backend.SetStateSuccess(signature, taskResults))
	taskState, err = backend.GetState(signature.UUID)
	assert.NoError(t, err)
	assert.Equal(t, signature.Name, taskState.TaskName)
	assert.Equal(t, createdAt, taskState.CreatedAt)
	assert.NotNil(t, taskState.Results)
}

func TestPurgeStateGR(t *testing.T) {
	backend := getRedisG()
	if backend == nil {
		t.Skip()
	}

	signature := &tasks.Signature{
		UUID:      "testTaskUUID",
		GroupUUID: "testGroupUUID",
	}

	assert.NoError(t, backend.SetStatePending(signature))
	taskState, err := backend.GetState(signature.UUID)
	assert.NotNil(t, taskState)
	assert.NoError(t, err)

	assert.NoError(t, backend.PurgeState(taskState.TaskUUID))
	taskState, err = backend.GetState(signature.UUID)
	assert.Nil(t, taskState)
	assert.Error(t, err)
}
