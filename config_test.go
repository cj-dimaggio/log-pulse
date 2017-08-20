package main

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseConfig(t *testing.T) {
	// Be careful with your text editor; YAML handles tabs weird so make sure your
	// editor doesn't automatically convert the 4 space indentations.
	var data = `
- type: log
  paths:
    - /var/tests/*.log
  pattern: .*
  command:
    program: echo
    args:
      - "Hello, World"
  timeout:
    interval: 30s
    once: true
    command:
      program: ohce
      args:
        - "World, Hello"

- type: log
  paths: ["/var/tests/*.log"]
  pattern: .*
  command.program: echo
  command.args: ["Hello, World"]
  timeout.interval: 30s
  timeout.once: true
  timeout.command.program: ohce
  timeout.command.args: ["World, Hello"]
`
	tmpFile, _ := ioutil.TempFile("", "test.yml")
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte(data))

	config, rawConfigs, err := ParseConfigFile(tmpFile.Name())
	assert.Nil(t, err)
	assert.NotNil(t, config)
	assert.NotNil(t, rawConfigs)

	for _, conf := range *config {
		assert.Equal(t, "log", conf.Type)
		assert.Equal(t, "/var/tests/*.log", conf.Paths[0])
		assert.Equal(t, ".*", conf.Pattern)
		assert.Equal(t, "echo", conf.Command.Program)
		assert.Equal(t, "Hello, World", conf.Command.Args[0])
		assert.Equal(t, 30*time.Second, conf.Timeout.Interval)
		assert.Equal(t, true, conf.Timeout.Once)
		assert.Equal(t, "ohce", conf.Timeout.Command.Program)
		assert.Equal(t, "World, Hello", conf.Timeout.Command.Args[0])
	}
}

func TestSetProspectorDefaults(t *testing.T) {
	var data = `
- type: log
  paths: ["/var/tests/*.log"]
  pattern: .*
`
	config, rawConfig, err := ParseConfig([]byte(data))
	assert.Nil(t, err)
	assert.NotNil(t, config)
	assert.NotNil(t, rawConfig)

	testConfig := prospectorConfig{}
	err = rawConfig[0].Unpack(&testConfig)
	assert.Nil(t, err)

	assert.Equal(t, true, testConfig.TailFiles)
	assert.Equal(t, 250*time.Millisecond, testConfig.Backoff)
	assert.Equal(t, 1, testConfig.BackoffFactor)
	assert.Equal(t, 1*time.Second, testConfig.MaxBackoff)
	assert.Equal(t, 3*time.Second, testConfig.ScanFrequency)
}
