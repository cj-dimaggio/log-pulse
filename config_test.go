package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfigFile(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "testing.yml")
	assert.Nil(t, err)
	defer os.Remove(tmpfile.Name())

	data := `
- file: /var/log/nginx.log
  pattern: .*
  command:
      program: nginx
      args:
        - "-s"
        - "reload"
  timeout: 40
  timeoutOnce: true
  timeoutCommand:
      program: "hello/world"

- file: /var/log/supervisord.log
  pattern: ERROR
  command:
      program: supervisorctl
      args:
        - reload
  poll: true
  pipe: true
  maxLineSize: 50
  mustExist: true
`
	_, err = tmpfile.Write([]byte(data))
	assert.Nil(t, err)

	config, err := ParseConfigFile(tmpfile.Name())
	assert.Nil(t, err)

	assert.NotNil(t, config)
	assert.Len(t, config, 2)

	assert.Equal(t, "/var/log/nginx.log", config[0].File)
	assert.Equal(t, ".*", config[0].Pattern)
	assert.Equal(t, "nginx", config[0].Command.Program)
	assert.Equal(t, "-s", config[0].Command.Args[0])
	assert.Equal(t, "reload", config[0].Command.Args[1])
	assert.Equal(t, 40, config[0].Timeout)
	assert.Equal(t, true, config[0].TimeoutOnce)
	assert.Equal(t, "hello/world", config[0].TimeoutCommand.Program)

	assert.Equal(t, "/var/log/supervisord.log", config[1].File)
	assert.Equal(t, "ERROR", config[1].Pattern)
	assert.Equal(t, "supervisorctl", config[1].Command.Program)
	assert.Equal(t, "reload", config[1].Command.Args[0])
	assert.Equal(t, true, config[1].Poll)
	assert.Equal(t, true, config[1].Pipe)
	assert.Equal(t, 50, config[1].MaxLineSize)
	assert.Equal(t, true, config[1].MustExist)
}

// Will only work on Unix
func TestCommandCmd(t *testing.T) {
	testFile := "/tmp/log-pulse-test.file"
	commandConfig := CommandConfig{
		Program: "/usr/bin/touch",
		Args:    []string{testFile},
	}

	os.Remove(testFile)
	err := commandConfig.Cmd().Run()
	defer os.Remove(testFile)
	assert.Nil(t, err)
	info, err := os.Stat(testFile)
	assert.Nil(t, err)
	assert.NotNil(t, info)

	// Make sure path resolution works
	commandConfig = CommandConfig{
		Program: "touch",
		Args:    []string{testFile},
	}
	os.Remove(testFile)
	err = commandConfig.Cmd().Run()
	defer os.Remove(testFile)
	assert.Nil(t, err)
	info, err = os.Stat(testFile)
	assert.Nil(t, err)
	assert.NotNil(t, info)

}
