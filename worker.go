package machinery

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/goravel/machinery/brokers/errs"
	"github.com/goravel/machinery/log"
	"github.com/goravel/machinery/retry"
	"github.com/goravel/machinery/tasks"
)

// Worker represents a single worker process
type Worker struct {
	server            *Server
	ConsumerTag       string
	Concurrency       int
	Queue             string
	errorHandler      func(err error)
	preTaskHandler    func(*tasks.Signature)
	postTaskHandler   func(*tasks.Signature)
	preConsumeHandler func(*Worker) bool
}

var (
	// ErrWorkerQuitGracefully is return when worker quit gracefully
	ErrWorkerQuitGracefully = errors.New("worker quit gracefully")
	// ErrWorkerQuitAbruptly is return when worker quit abruptly
	ErrWorkerQuitAbruptly = errors.New("worker quit abruptly")
)

// Launch starts a new worker process. The worker subscribes
// to the default queue and processes incoming registered tasks
func (worker *Worker) Launch() error {
	errorsChan := make(chan error)

	worker.LaunchAsync(errorsChan)

	return <-errorsChan
}

// LaunchAsync is a non blocking version of Launch
func (worker *Worker) LaunchAsync(errorsChan chan<- error) {
	cnf := worker.server.GetConfig()
	broker := worker.server.GetBroker()

	// Log some useful information about worker configuration
	log.INFO("Launching a worker with the following settings:")
	log.INFO("- Broker: %s", RedactURL(cnf.Broker))
	if worker.Queue == "" {
		log.INFO("- DefaultQueue: %s", cnf.DefaultQueue)
	} else {
		log.INFO("- CustomQueue: %s", worker.Queue)
	}
	log.INFO("- ResultBackend: %s", RedactURL(cnf.ResultBackend))

	var signalWG sync.WaitGroup
	// Goroutine to start broker consumption and handle retries when broker connection dies
	go func() {
		for {
			re, err := broker.StartConsuming(worker.ConsumerTag, worker.Concurrency, worker)

			if re {
				if worker.errorHandler != nil {
					worker.errorHandler(err)
				} else {
					log.WARNING("Broker failed with error: %s", err)
				}
			} else {
				signalWG.Wait()
				errorsChan <- err // stop the goroutine
				return
			}
		}
	}()
	if !cnf.NoUnixSignals {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		var signalsReceived uint

		// Goroutine Handle SIGINT and SIGTERM signals
		go func() {
			for s := range sig {
				log.WARNING("Signal received: %v", s)
				signalsReceived++

				if signalsReceived < 2 {
					// After first Ctrl+C start quitting the worker gracefully
					log.WARNING("Waiting for running tasks to finish before shutting down")
					signalWG.Add(1)
					go func() {
						worker.Quit()
						errorsChan <- ErrWorkerQuitGracefully
						signalWG.Done()
					}()
				} else {
					// Abort the program when user hits Ctrl+C second time in a row
					errorsChan <- ErrWorkerQuitAbruptly
				}
			}
		}()
	}
}

// CustomQueue returns Custom Queue of the running worker process
func (worker *Worker) CustomQueue() string {
	return worker.Queue
}

// Quit tears down the running worker process
func (worker *Worker) Quit() {
	worker.server.GetBroker().StopConsuming()
}

// Process handles received tasks and triggers success/error callbacks
func (worker *Worker) Process(signature *tasks.Signature) error {
	// If the task is not registered with this worker, do not continue
	// but only return nil as we do not want to restart the worker process
	if !worker.server.IsTaskRegistered(signature.Name) {
		return nil
	}

	taskFunc, err := worker.server.GetRegisteredTask(signature.Name)
	if err != nil {
		return nil
	}

	// Update task state to RECEIVED
	if err = worker.server.GetBackend().SetStateReceived(signature); err != nil {
		return fmt.Errorf("Set state to 'received' for task %s returned error: %s", signature.UUID, err)
	}

	// Prepare task for processing
	task, err := tasks.NewWithSignature(taskFunc, signature)
	// if this failed, it means the task is malformed, probably has invalid
	// signature, go directly to task failed without checking whether to retry
	if err != nil {
		_ = worker.taskFailed(signature, err)
		return err
	}

	// Update task state to STARTED
	if err = worker.server.GetBackend().SetStateStarted(signature); err != nil {
		return fmt.Errorf("Set state to 'started' for task %s returned error: %s", signature.UUID, err)
	}

	//Run handler before the task is called
	if worker.preTaskHandler != nil {
		worker.preTaskHandler(signature)
	}

	//Defer run handler for the end of the task
	if worker.postTaskHandler != nil {
		defer worker.postTaskHandler(signature)
	}

	// Call the task
	results, err := task.Call()
	if err != nil {
		// If a tasks.ErrRetryTaskLater was returned from the task,
		// retry the task after specified duration
		retriableErr, ok := any(err).(tasks.ErrRetryTaskLater)
		if ok {
			return worker.retryTaskIn(signature, retriableErr.RetryIn())
		}

		// Otherwise, execute default retry logic based on signature.RetryCount
		// and signature.RetryTimeout values
		if signature.RetryCount > 0 {
			return worker.taskRetry(signature)
		}

		return worker.taskFailed(signature, err)
	}

	return worker.taskSucceeded(signature, results)
}

// retryTask decrements RetryCount counter and republishes the task to the queue
func (worker *Worker) taskRetry(signature *tasks.Signature) error {
	// Update task state to RETRY
	if err := worker.server.GetBackend().SetStateRetry(signature); err != nil {
		return fmt.Errorf("Set state to 'retry' for task %s returned error: %s", signature.UUID, err)
	}

	// Decrement the retry counter, when it reaches 0, we won't retry again
	signature.RetryCount--

	// Increase retry timeout
	signature.RetryTimeout = retry.FibonacciNext(signature.RetryTimeout)

	// Delay task by signature.RetryTimeout seconds
	eta := time.Now().UTC().Add(time.Second * time.Duration(signature.RetryTimeout))
	signature.ETA = &eta

	log.WARNING("Task %s failed. Going to retry in %d seconds.", signature.UUID, signature.RetryTimeout)

	// Send the task back to the queue
	_, err := worker.server.SendTask(signature)
	return err
}

// taskRetryIn republishes the task to the queue with ETA of now + retryIn.Seconds()
func (worker *Worker) retryTaskIn(signature *tasks.Signature, retryIn time.Duration) error {
	// Update task state to RETRY
	if err := worker.server.GetBackend().SetStateRetry(signature); err != nil {
		return fmt.Errorf("Set state to 'retry' for task %s returned error: %s", signature.UUID, err)
	}

	// Delay task by retryIn duration
	eta := time.Now().UTC().Add(retryIn)
	signature.ETA = &eta

	log.WARNING("Task %s failed. Going to retry in %.0f seconds.", signature.UUID, retryIn.Seconds())

	// Send the task back to the queue
	_, err := worker.server.SendTask(signature)
	return err
}

// taskSucceeded updates the task state and triggers success callbacks or a
// chord callback if this was the last task of a group with a chord callback
func (worker *Worker) taskSucceeded(signature *tasks.Signature, taskResults []*tasks.TaskResult) error {
	// Update task state to SUCCESS
	if err := worker.server.GetBackend().SetStateSuccess(signature, taskResults); err != nil {
		return fmt.Errorf("Set state to 'success' for task %s returned error: %s", signature.UUID, err)
	}

	// Log human readable results of the processed task
	var debugResults = "[]"
	results, err := tasks.ReflectTaskResults(taskResults)
	if err != nil {
		log.WARNING("error", err)
	} else {
		debugResults = tasks.HumanReadableResults(results)
	}
	log.DEBUG("Processed task %s. Results = %s", signature.UUID, debugResults)

	// Trigger success callbacks

	for _, successTask := range signature.OnSuccess {
		if !signature.Immutable {
			// Pass results of the task to success callbacks
			for _, taskResult := range taskResults {
				successTask.Args = append(successTask.Args, tasks.Arg{
					Type:  taskResult.Type,
					Value: taskResult.Value,
				})
			}
		}

		_, _ = worker.server.SendTask(successTask)
	}

	// If the task was not part of a group, just return
	if signature.GroupUUID == "" {
		return nil
	}

	// There is no chord callback, just return
	if signature.ChordCallback == nil {
		return nil
	}

	// Check if all task in the group has completed
	groupCompleted, err := worker.server.GetBackend().GroupCompleted(
		signature.GroupUUID,
		signature.GroupTaskCount,
	)
	if err != nil {
		return fmt.Errorf("Completed check for group %s returned error: %s", signature.GroupUUID, err)
	}

	// If the group has not yet completed, just return
	if !groupCompleted {
		return nil
	}

	// Trigger chord callback
	shouldTrigger, err := worker.server.GetBackend().TriggerChord(signature.GroupUUID)
	if err != nil {
		return fmt.Errorf("Triggering chord for group %s returned error: %s", signature.GroupUUID, err)
	}

	// Chord has already been triggered
	if !shouldTrigger {
		return nil
	}

	// Get task states
	taskStates, err := worker.server.GetBackend().GroupTaskStates(
		signature.GroupUUID,
		signature.GroupTaskCount,
	)
	if err != nil {
		log.ERROR(
			"Failed to get tasks states for group:[%s]. Task count:[%d]. The chord may not be triggered. Error:[%s]",
			signature.GroupUUID,
			signature.GroupTaskCount,
			err,
		)
		return nil
	}

	// Append group tasks' return values to chord task if it's not immutable
	for _, taskState := range taskStates {
		if !taskState.IsSuccess() {
			return nil
		}

		if !signature.ChordCallback.Immutable {
			// Pass results of the task to the chord callback
			for _, taskResult := range taskState.Results {
				signature.ChordCallback.Args = append(signature.ChordCallback.Args, tasks.Arg{
					Type:  taskResult.Type,
					Value: taskResult.Value,
				})
			}
		}
	}

	// Send the chord task
	_, err = worker.server.SendTask(signature.ChordCallback)
	if err != nil {
		return err
	}

	return nil
}

// taskFailed updates the task state and triggers error callbacks
func (worker *Worker) taskFailed(signature *tasks.Signature, taskErr error) error {
	// Update task state to FAILURE
	if err := worker.server.GetBackend().SetStateFailure(signature, taskErr.Error()); err != nil {
		return fmt.Errorf("Set state to 'failure' for task %s returned error: %s", signature.UUID, err)
	}

	if worker.errorHandler != nil {
		worker.errorHandler(taskErr)
	} else {
		log.ERROR("Failed processing task %s. Error = %v", signature.UUID, taskErr)
	}

	// Trigger error callbacks
	for _, errorTask := range signature.OnError {
		// Pass error as a first argument to error callbacks
		args := append([]tasks.Arg{{
			Type:  "string",
			Value: taskErr.Error(),
		}}, errorTask.Args...)
		errorTask.Args = args
		_, _ = worker.server.SendTask(errorTask)
	}

	if signature.StopTaskDeletionOnError {
		return errs.ErrStopTaskDeletion
	}

	return nil
}

// SetErrorHandler sets a custom error handler for task errors
// A default behavior is just to log the error after all the retry attempts fail
func (worker *Worker) SetErrorHandler(handler func(err error)) {
	worker.errorHandler = handler
}

// SetPreTaskHandler sets a custom handler func before a job is started
func (worker *Worker) SetPreTaskHandler(handler func(*tasks.Signature)) {
	worker.preTaskHandler = handler
}

// SetPostTaskHandler sets a custom handler for the end of a job
func (worker *Worker) SetPostTaskHandler(handler func(*tasks.Signature)) {
	worker.postTaskHandler = handler
}

// SetPreConsumeHandler sets a custom handler for the end of a job
func (worker *Worker) SetPreConsumeHandler(handler func(*Worker) bool) {
	worker.preConsumeHandler = handler
}

// GetServer returns server
func (worker *Worker) GetServer() *Server {
	return worker.server
}

func (worker *Worker) PreConsumeHandler() bool {
	if worker.preConsumeHandler == nil {
		return true
	}

	return worker.preConsumeHandler(worker)
}

func RedactURL(urlString string) string {
	u, err := url.Parse(urlString)
	if err != nil {
		return urlString
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}
