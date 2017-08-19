package main

import (
	"log"
	"regexp"
	"time"

	"github.com/hpcloud/tail"
)

// NoTrackedFilesError signifies that no files could be tracked by the Monitor
type NoTrackedFilesError struct{}

func (err NoTrackedFilesError) Error() string {
	return "No files were setup to be tracked"
}

// Monitor holds references to log files to be monitored, tails them,
// and executes commands if the particular log file matches the defined
// pattern
type Monitor struct {
	config Config

	trackers   []*LogTracker
	stopSignal chan struct{}
	stopped    chan struct{}
}

// CreateMonitor initializes a new instance of a Monitor
func CreateMonitor(config Config) Monitor {
	return Monitor{
		config: config,

		stopSignal: make(chan struct{}),
		stopped:    make(chan struct{}),
	}
}

// Start begins tailing the defined log files and begins the monitoring process.
// This function does not return until Stop is called from another Go routine
func (monitor *Monitor) Start() error {
	// Keep track of all the tails we make so we can close them later
	// It might be more efficient to make this slice explicitly, considering we
	// have the max length as the length of our config, such as:
	//     tails := make([]tail.Tail, len(monitor.config))
	// but because elements may *not* be appended if the log file doesn't exist
	// I would like to use the length of the slice to indicate how many are
	// *actually* running (and avoid having holes in our array). Besides, this
	// is all happening on initialization so we don't have to be too worried
	// about optimization here.

	for _, logConfig := range monitor.config {
		lt := monitor.createTracker(logConfig)
		if lt != nil {
			go lt.Track()
			monitor.trackers = append(monitor.trackers, lt)
		}
	}

	if len(monitor.trackers) == 0 {
		log.Println("No files were registered for tracking. Not starting")
		return NoTrackedFilesError{}
	}

	select {
	case <-monitor.stopSignal:
		log.Println("Received stop signal. Cleaning up tracking")
		for _, lt := range monitor.trackers {
			lt.Cleanup()
		}
		// Trigger another signal signifying we've stopped. Right now, this doesn't actually
		// wait for all of the LogTracker's goroutines to call their onClose functions, just
		// until their cleanup is done. That may have to be fixed in the future
		close(monitor.stopped)
	}

	return nil
}

// createTracker parses the LogConfig and creates a new LogTracker. If there is an error, it will
// return nil
func (monitor *Monitor) createTracker(logConfig LogConfig) *LogTracker {
	log.Println("Registering file tracking for: ", logConfig.File)
	t, err := tail.TailFile(logConfig.File, tail.Config{
		ReOpen: true,
		Follow: true,

		// Start from the end
		Location: &tail.SeekInfo{
			Offset: 0,
			Whence: 2,
		},

		MustExist:   logConfig.MustExist,
		Poll:        logConfig.Poll,
		Pipe:        logConfig.Pipe,
		MaxLineSize: logConfig.MaxLineSize,
	})
	if err != nil {
		// We don't necessarily want to quit the entire process just because
		// one log file is missing or something, so log the error and move
		// on
		log.Println("Error creating tracking for file: ", err)
		return nil
	}
	pattern, err := regexp.Compile(logConfig.Pattern)
	if err != nil {
		log.Printf("Unable to compile regular expression '%s'. Won't track file: '%s': '%s", logConfig.Pattern, logConfig.File, err)
		return nil
	}

	onPattern, onTimeout := monitor.genCallbacks(logConfig)

	lt := LogTracker{
		FileName:   logConfig.File,
		Pattern:    pattern,
		Timeout:    time.Duration(logConfig.Timeout) * time.Second,
		Tail:       t,
		StopSignal: monitor.stopSignal,

		OnPattern: onPattern,
		OnTimeout: onTimeout,
	}
	return &lt
}

// Generate the proper callbacks for onPattern and onTimeout. I want to try to keep
// LogTracker as generic as possible so things like "TimeoutOnce" should be handled
// here
func (monitor *Monitor) genCallbacks(logConfig LogConfig) (LogTrackerOnPattern, LogTrackerOnTimeout) {
	timedOut := false

	// We could probably optimize this so that we return different functions based on conditions
	// (ie: no TimeOutOnce and no Match command? Return a NoOp). But for now let's focus on readability
	onPattern := func(lt *LogTracker, line *tail.Line) {
		if logConfig.Command.Program != "" {
			lt.log("Found pattern. Executing command: %s", logConfig.Command)
			// Let's just run it in the background
			logConfig.Command.Cmd().Start()
		}
		timedOut = false
	}

	onTimeout := func(lt *LogTracker, _time time.Time) {

		if logConfig.TimeoutCommand.Program != "" {
			if !(timedOut && logConfig.TimeoutOnce) {
				lt.log("Timedout waiting for pattern. Executing command: %s", logConfig.TimeoutCommand)
				// Let's just run it in the background
				logConfig.TimeoutCommand.Cmd().Start()
			}
		}
		timedOut = true
	}

	return onPattern, onTimeout
}

// Stop ceases the Monitor's Start process so that proper cleanup can occur and
// control can be returned. Stop should generally only be called when the process
// is to be stopped (for instance, on a keyboard interrupt or exit signal)
func (monitor *Monitor) Stop() {
	// A close, unlike a simple enqueue, will get broadcast to *every* goroutine
	// listening in a select statement
	//
	// We may in the future want to wrap this in a "sync.Once" so that this can
	// safely be called multiple times. We may also want to add some checks to
	// make sure that the Monitor has been started already (and maybe the ability
	// to start it again). But for now let's not add functionality we know we won't
	// be needing yet.
	close(monitor.stopSignal)
	// Wait until the monitor has stopped
	<-monitor.stopped
}
