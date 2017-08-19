package main

import (
	"io/ioutil"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hpcloud/tail"
	"github.com/stretchr/testify/assert"
)

func quickRegexCompile(pattern string) *regexp.Regexp {
	r, _ := regexp.Compile(pattern)
	return r
}

func TestLogTrackMatch(t *testing.T) {
	logFile, _ := ioutil.TempFile("", "test.log")
	defer os.Remove(logFile.Name())

	_tail, _ := tail.TailFile(logFile.Name(), tail.Config{Follow: true})
	closeChannel := make(chan struct{})

	patternMatched := false
	wasClosed := false

	lt := LogTracker{
		FileName:   logFile.Name(),
		Pattern:    quickRegexCompile("^LogAlert:"),
		StopSignal: closeChannel,
		Tail:       _tail,

		OnPattern: func(lt *LogTracker, line *tail.Line) {
			patternMatched = true
		},

		OnClose: func(lt *LogTracker) {
			wasClosed = true
		},
	}

	go lt.Track()
	logFile.Write([]byte("LogAlert: Hello!\n"))
	time.Sleep(10 * time.Millisecond)
	assert.True(t, patternMatched)
	patternMatched = false

	logFile.Write([]byte("NotLogAlert: Hello!\n"))
	time.Sleep(10 * time.Millisecond)
	assert.False(t, patternMatched)

	close(closeChannel)
	lt.Cleanup()
	time.Sleep(10 * time.Millisecond)
	assert.True(t, wasClosed)
}

func TestLogTrackTimeout(t *testing.T) {
	logFile, _ := ioutil.TempFile("", "test.log")
	defer os.Remove(logFile.Name())

	_tail, _ := tail.TailFile(logFile.Name(), tail.Config{Follow: true})
	closeChannel := make(chan struct{})

	timedOut := false

	lt := LogTracker{
		FileName:   logFile.Name(),
		Pattern:    quickRegexCompile("^LogAlert:"),
		Timeout:    50 * time.Millisecond,
		StopSignal: closeChannel,
		Tail:       _tail,

		OnTimeout: func(lt *LogTracker, _time time.Time) {
			timedOut = true
		},
	}

	go lt.Track()
	// Wait for timeout
	timedOut = false
	time.Sleep(60 * time.Millisecond)
	assert.True(t, timedOut)

	// Make sure that pattern matches resets our timeout
	timedOut = false
	logFile.Write([]byte("LogAlert: Hello!\n"))
	time.Sleep(20 * time.Millisecond)
	logFile.Write([]byte("LogAlert: Hello!\n"))
	time.Sleep(20 * time.Millisecond)
	logFile.Write([]byte("LogAlert: Hello!\n"))
	time.Sleep(20 * time.Millisecond)
	assert.False(t, timedOut)

	// Make sure that patterns that don't match don't reset the timeout
	timedOut = false
	logFile.Write([]byte("NotLogAlert: Hello!\n"))
	time.Sleep(20 * time.Millisecond)
	logFile.Write([]byte("NotLogAlert: Hello!\n"))
	time.Sleep(20 * time.Millisecond)
	logFile.Write([]byte("NotLogAlert: Hello!\n"))
	time.Sleep(20 * time.Millisecond)
	assert.True(t, timedOut)

	lt.Cleanup()
	close(closeChannel)
}
