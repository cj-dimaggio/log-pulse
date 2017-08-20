package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"

	"github.com/elastic/beats/filebeat/harvester"
	"github.com/elastic/beats/filebeat/util"
	"github.com/stretchr/testify/assert"
)

func assertChanEmpty(t *testing.T, c chan string) {
	select {
	case msg := <-c:
		t.Error("Expected an empty channel. Instead found: ", msg)
	default:
		return
	}
}

func assertChanMsg(t *testing.T, c chan string, expected string) {
	select {
	case msg := <-c:
		assert.Equal(t, expected, msg)
	default:
		t.Error("Expected channel to have a message. Instead it was empty")
	}
}

func assertFileExists(t *testing.T, filename string) os.FileInfo {
	info, err := os.Stat(filename)
	assert.Nil(t, err)
	return info
}

func assertFileDoesNotExist(t *testing.T, filename string) {
	_, err := os.Stat(filename)
	assert.NotNil(t, err)
}

func TestCollectorOutleterOnEvent(t *testing.T) {
	pipe := make(chan string, 1)
	outleter := CollectorOutleter{
		lines: pipe,
	}

	// And empty event shouldn't emit anything
	data := util.NewData()
	assert.True(t, outleter.OnEvent(data))
	assertChanEmpty(t, pipe)

	// event with non message field
	data = util.NewData()
	data.Event = beat.Event{
		Fields: common.MapStr{
			"NotaMessage": "Hi",
		},
	}
	assert.True(t, outleter.OnEvent(data))
	assertChanEmpty(t, pipe)

	// event with message field but not a string
	data = util.NewData()
	data.Event = beat.Event{
		Fields: common.MapStr{
			"message": 10,
		},
	}
	assert.True(t, outleter.OnEvent(data))
	assertChanEmpty(t, pipe)

	// Properly formatted event
	data = util.NewData()
	data.Event = beat.Event{
		Fields: common.MapStr{
			"message": "Hello, World",
		},
	}
	assert.True(t, outleter.OnEvent(data))
	assertChanMsg(t, pipe, "Hello, World")
}

func TestCollectorProcessMatch(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "log-pulse-test")
	defer os.RemoveAll(tmpDir)

	touchedFile := filepath.Join(tmpDir, "touched-file")

	collector := Collector{
		prospectorDone: make(chan struct{}),
		lines:          make(chan string),
		Done:           make(chan struct{}),
		Stopped:        make(chan struct{}),
		timeoutChannel: make(chan time.Time),

		config: CollectorConfig{
			Command: CommandConfig{
				Program: "touch",
				Args:    []string{touchedFile},
			},
		},
	}

	collector.Pattern, _ = regexp.Compile("^Match")

	// Make sure no matches don't execute the command
	go collector.process()
	collector.lines <- "NotAMatch"
	time.Sleep(10 * time.Millisecond)
	assertFileDoesNotExist(t, touchedFile)

	// Make sure that matches execute the command
	collector.lines <- "MatchIsWhatItIS"
	time.Sleep(10 * time.Millisecond)
	assertFileExists(t, touchedFile)

	// Make sure we close done
	close(collector.Done)
	<-collector.Stopped
}

func TestCollectorProcessTimeout(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "log-pulse-test")
	defer os.RemoveAll(tmpDir)

	touchedFile := filepath.Join(tmpDir, "touched-file")

	collector := Collector{
		prospectorDone: make(chan struct{}),
		lines:          make(chan string),
		Done:           make(chan struct{}),
		Stopped:        make(chan struct{}),

		config: CollectorConfig{
			Timeout: TimeoutConfig{
				Interval: 50 * time.Millisecond,
				Command: CommandConfig{
					Program: "touch",
					Args:    []string{touchedFile},
				},
			},
		},
	}

	collector.ticker = time.NewTicker(collector.config.Timeout.Interval)
	collector.timeoutChannel = collector.ticker.C

	collector.Pattern, _ = regexp.Compile("^Match")

	go collector.process()

	// Make sure we can stave off the timeout by sending commands
	for i := 0; i < 10; i++ {
		collector.lines <- "MatchIsWhatItIS"
		time.Sleep(10 * time.Millisecond)
		assertFileDoesNotExist(t, touchedFile)

	}

	// Make sure it gets created after timeout
	time.Sleep(60 * time.Millisecond)
	info := assertFileExists(t, touchedFile)
	originalModTime := info.ModTime()

	// Make sure that it gets executed again after another timeout
	// File stats don't have great fidelity to we want to wait for at least a second
	time.Sleep(1100 * time.Millisecond)
	info = assertFileExists(t, touchedFile)
	assert.True(t, info.ModTime().After(originalModTime))

	// Make sure we close done
	close(collector.Done)
	<-collector.Stopped
}

func TestCollectorProcessTimeoutOnce(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "log-pulse-test")
	defer os.RemoveAll(tmpDir)

	touchedFile := filepath.Join(tmpDir, "touched-file")

	collector := Collector{
		prospectorDone: make(chan struct{}),
		lines:          make(chan string),
		Done:           make(chan struct{}),
		Stopped:        make(chan struct{}),

		config: CollectorConfig{
			Timeout: TimeoutConfig{
				Interval: 50 * time.Millisecond,
				Command: CommandConfig{
					Program: "touch",
					Args:    []string{touchedFile},
				},
				Once: true,
			},
		},
	}

	collector.ticker = time.NewTicker(collector.config.Timeout.Interval)
	collector.timeoutChannel = collector.ticker.C

	collector.Pattern, _ = regexp.Compile("^Match")

	go collector.process()

	// Make sure we can stave off the timeout by sending commands
	for i := 0; i < 10; i++ {
		collector.lines <- "MatchIsWhatItIS"
		time.Sleep(10 * time.Millisecond)
		assertFileDoesNotExist(t, touchedFile)

	}

	// Make sure it gets created after timeout
	time.Sleep(60 * time.Millisecond)
	info := assertFileExists(t, touchedFile)
	originalModTime := info.ModTime()

	// Make sure that it doesn't get executed until we send in a new pattern
	// File stats don't have great fidelity to we want to wait for at least a second
	time.Sleep(1100 * time.Millisecond)
	info = assertFileExists(t, touchedFile)
	assert.True(t, info.ModTime().Equal(originalModTime))

	// Send a new matching line and then wait for a timeout
	collector.lines <- "MatchIsWhatItIS"
	time.Sleep(60 * time.Millisecond)
	info = assertFileExists(t, touchedFile)
	assert.True(t, info.ModTime().After(originalModTime))

	// Make sure we close done
	close(collector.Done)
	<-collector.Stopped
}

// A bit of a kitchen sink test where we try to test the entire system.
// It doesn't goes as in depth trying to evaluate every edge case but it should
// be a good smoke test. Note that it can take sometime for the FileBeat's prospector's
// to initialize and parse files at first so this test might take some time to run (~5 seconds).
func TestCollection(t *testing.T) {

	// Will hold our log files
	logFolder, _ := ioutil.TempDir("", "logFolder")
	defer os.RemoveAll(logFolder)

	// Will hold the touched files which demonstrate a command has been run
	touchFolder, _ := ioutil.TempDir("", "touchFolder")
	defer os.RemoveAll(touchFolder)

	logFile1, _ := ioutil.TempFile(logFolder, "collector1.log")
	logFile2, _ := ioutil.TempFile(logFolder, "collector1.var")

	logFile3, _ := ioutil.TempFile(logFolder, "collector2.log")
	logFile4, _ := ioutil.TempFile(logFolder, "collector2.var")

	touchedFile1 := filepath.Join(touchFolder, "collector1.touched")
	touchedFile2 := filepath.Join(touchFolder, "collector2.touched")

	configs := LogPulseConfig{
		CollectorConfig{
			Type:    harvester.LogType,
			Pattern: "^Match",
			Paths:   []string{filepath.Join(logFolder, "collector1.*")},
			Command: CommandConfig{
				Program: "touch",
				Args:    []string{touchedFile1},
			},
			Timeout: TimeoutConfig{
				Interval: 100 * time.Second, // Exercise our tick creation logic
			},
		},
		CollectorConfig{
			Type:    harvester.LogType,
			Pattern: "^Match",
			Paths:   []string{filepath.Join(logFolder, "collector2.*")},
			Command: CommandConfig{
				Program: "touch",
				Args:    []string{touchedFile2},
			},
		},
	}

	// Convert our config to FileBeat configs
	raw, err := common.NewConfigFrom(configs)
	assert.Nil(t, err)
	var rawConfigs []*common.Config
	err = raw.Unpack(&rawConfigs)
	assert.Nil(t, err)

	// We want to use slightly more aggressive defaults than the global
	// so that the test executes faster so we're going to reimplement a lot
	// of setProspectorDefaults here

	for _, rawConfig := range rawConfigs {
		conf := prospectorConfig{
			TailFiles:     true,
			Backoff:       10 * time.Millisecond,
			BackoffFactor: 1,
			MaxBackoff:    30 * time.Millisecond,
			ScanFrequency: 30 * time.Millisecond,
		}

		err = rawConfig.Unpack(&conf)
		assert.Nil(t, err)
		err = rawConfig.Merge(conf)
		assert.Nil(t, err)
	}

	collection, err := CreateCollection(configs, rawConfigs)
	assert.Nil(t, err)
	assert.NotNil(t, collection)
	collection.Start()
	time.Sleep(2000 * time.Millisecond)

	// Make sure every file executed the command

	assertFileDoesNotExist(t, touchedFile1)
	logFile1.Write([]byte("MatchIsWhatItIS\n"))
	time.Sleep(200 * time.Millisecond)
	assertFileExists(t, touchedFile1)
	os.Remove(touchedFile1)

	assertFileDoesNotExist(t, touchedFile1)
	logFile2.Write([]byte("MatchIsWhatItIS\n"))
	time.Sleep(200 * time.Millisecond)
	assertFileExists(t, touchedFile1)
	os.Remove(touchedFile1)

	assertFileDoesNotExist(t, touchedFile2)
	logFile3.Write([]byte("MatchIsWhatItIS\n"))
	time.Sleep(200 * time.Millisecond)
	assertFileExists(t, touchedFile2)
	os.Remove(touchedFile2)

	assertFileDoesNotExist(t, touchedFile2)
	logFile4.Write([]byte("MatchIsWhatItIS\n"))
	time.Sleep(200 * time.Millisecond)
	assertFileExists(t, touchedFile2)
	os.Remove(touchedFile2)

	// Make sure that the collection closes cleanly
	collection.Stop()
	collection.LetRun()
}
