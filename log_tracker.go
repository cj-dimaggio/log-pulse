package main

import (
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/hpcloud/tail"
)

// LogTrackerOnPattern is called when an incoming log line is matched against the
// regexp pattern
type LogTrackerOnPattern func(lt *LogTracker, line *tail.Line)

// LogTrackerOnTimeout is called when a new matched pattern has been seen for the duration
// of the timeout
type LogTrackerOnTimeout func(*LogTracker, time.Time)

// LogTrackerOnClose is called when the StopSignal is triggered
type LogTrackerOnClose func(*LogTracker)

// LogTracker handles the tracking and monitoring of a single log file
type LogTracker struct {
	FileName string
	Pattern  *regexp.Regexp
	Timeout  time.Duration

	TimeoutOnce bool

	OnPattern LogTrackerOnPattern
	OnTimeout LogTrackerOnTimeout
	OnClose   LogTrackerOnClose

	Tail       *tail.Tail
	StopSignal chan struct{}

	timeoutChannel <-chan time.Time
	ticker         *time.Ticker
}

// Logs with an identifier prefixed to the line
func (lt *LogTracker) log(format string, v ...interface{}) {
	log.Printf("Monitor-'%s': %s", lt.FileName, fmt.Sprintf(format, v...))
}

// Track handles tracking a file for new lines, testing those new lines
// against a regular expression, and then executing a command if that pattern
// matches
func (lt *LogTracker) Track() {
	lt.log("Starting tracking")

	// Initialize our ticker
	if lt.Timeout > 0 {
		// If a timeout is set then create a new ticker
		lt.ticker = time.NewTicker(lt.Timeout)
		lt.timeoutChannel = lt.ticker.C
	} else {
		// If a timeout is not set then create just a generic channel that will never return
		lt.timeoutChannel = make(chan time.Time)
	}

	for {
		select {
		case line := <-lt.Tail.Lines:
			// A new line has just come in
			if lt.Pattern.MatchString(line.Text) {
				// The line matches our pattern
				lt.ResetTimeout()
				if lt.OnPattern != nil {
					lt.OnPattern(lt, line)
				}
			}
		case t := <-lt.timeoutChannel:
			// We timed out waiting for a new line that matches our pattern
			if lt.OnTimeout != nil {
				lt.OnTimeout(lt, t)
			}
		case <-lt.StopSignal:
			lt.log("Received stop signal. Closing routine")
			if lt.OnClose != nil {
				lt.OnClose(lt)
			}
			return
		}
	}
}

// ResetTimeout restarts the ticker from this point in time
func (lt *LogTracker) ResetTimeout() {
	if lt.ticker != nil {
		lt.ticker.Stop()
		lt.ticker = time.NewTicker(lt.Timeout)
		lt.timeoutChannel = lt.ticker.C
	}
}

// Cleanup the tracker before closing
func (lt *LogTracker) Cleanup() {
	lt.log("Cleaning up")
	if lt.ticker != nil {
		lt.ticker.Stop()
	}

	lt.Tail.Cleanup()
}
