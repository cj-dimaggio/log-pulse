package main

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMonitorCreateTracker(t *testing.T) {
	closeChannel := make(chan struct{})

	monitor := Monitor{
		stopSignal: closeChannel,
	}

	lt := monitor.createTracker(LogConfig{File: "NotRealFile", MustExist: true})
	assert.Nil(t, lt)

	logFile, _ := ioutil.TempFile("", "test.log")
	lt = monitor.createTracker(LogConfig{
		File:    logFile.Name(),
		Pattern: "s(3",
	})
	assert.Nil(t, lt)

	lt = monitor.createTracker(LogConfig{
		File:    logFile.Name(),
		Pattern: ".*",
	})
	assert.NotNil(t, lt)
}

func TestMonitorStartEmpty(t *testing.T) {
	monitor := CreateMonitor(Config{})
	err := monitor.Start()
	assert.Error(t, err)
}

// Kind of a kitchen sink test. Because it executes actual system commands it will
// probably only pass on Unix systems
func TestMonitorComplete(t *testing.T) {
	matchTouchFile := "/tmp/log-pulse-test.match"
	defer os.Remove(matchTouchFile)
	timeoutTouchFile := "/tmp/log-pulse-test.timeout"
	defer os.Remove(timeoutTouchFile)

	timeoutTouchOnceFile := "/tmp/log-pulse-test.timeout-once"
	defer os.Remove(timeoutTouchOnceFile)

	logFile1, _ := ioutil.TempFile("", "logFile1.log")
	defer os.Remove(logFile1.Name())
	logFile2, _ := ioutil.TempFile("", "logFile2.log")
	defer os.Remove(logFile2.Name())
	logFile3, _ := ioutil.TempFile("", "logFile3.log")
	defer os.Remove(logFile3.Name())

	monitor := CreateMonitor(Config{
		LogConfig{
			File:    logFile1.Name(),
			Pattern: "^Match",
			Command: CommandConfig{
				Program: "/usr/bin/touch",
				Args:    []string{matchTouchFile},
			},
		},

		LogConfig{
			File:    logFile2.Name(),
			Pattern: ".*",
			Timeout: 1,
			TimeoutCommand: CommandConfig{
				Program: "/usr/bin/touch",
				Args:    []string{timeoutTouchFile},
			},
		},
		LogConfig{
			File:    logFile3.Name(),
			Pattern: ".*",
			Timeout: 1,
			TimeoutCommand: CommandConfig{
				Program: "/usr/bin/touch",
				Args:    []string{timeoutTouchOnceFile},
			},
			TimeoutOnce: true,
		},
	})

	os.Remove(matchTouchFile)
	os.Remove(timeoutTouchFile)
	os.Remove(timeoutTouchOnceFile)

	go monitor.Start()
	// Wait for 1.10 seconds
	time.Sleep(1100 * time.Millisecond)
	info, err := os.Stat(timeoutTouchFile)
	assert.Nil(t, err)
	assert.NotNil(t, info)

	info, err = os.Stat(timeoutTouchOnceFile)
	assert.Nil(t, err)
	assert.NotNil(t, info)
	originalTime := info.ModTime()

	// Wait for another timeout
	time.Sleep(1100 * time.Millisecond)
	info, _ = os.Stat(timeoutTouchOnceFile)
	assert.NotNil(t, info)
	assert.True(t, originalTime.Equal(info.ModTime()))

	// Reset the timer
	logFile3.Write([]byte("Anything\n"))
	time.Sleep(1100 * time.Millisecond)
	info, _ = os.Stat(timeoutTouchOnceFile)
	assert.NotNil(t, info)
	assert.True(t, info.ModTime().After(originalTime))

	logFile1.Write([]byte("MatchExample\n"))
	time.Sleep(10 * time.Millisecond)
	info, err = os.Stat(matchTouchFile)
	assert.Nil(t, err)
	assert.NotNil(t, info)

	monitor.Stop()
}
