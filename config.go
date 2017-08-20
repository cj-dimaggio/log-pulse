package main

import (
	"io/ioutil"
	"os/exec"

	"gopkg.in/yaml.v2"
)

// CommandConfig is the configuration of a command to execute
type CommandConfig struct {
	Program string   `yaml:"program"`
	Args    []string `yaml:"args"`
}

// LogConfig contains all of the configuration options for each
// log entry
type LogConfig struct {
	// The log file to monitor
	File string `yaml:"file"`
	// The regex pattern to look for in lines
	Pattern string `yaml:"pattern"`
	// The system command to execute when the regex pattern matches
	Command CommandConfig `yaml:"command"`

	// The amount of time (in seconds) to wait for the pattern to appear before executing TimeoutCommand
	Timeout int `yaml:"timeout"`

	// Should the timout command only execute once until a new pattern shows up? By default it will execute
	// over and over, using Timeout as an interval.
	TimeoutOnce bool `yaml:"timeoutOnce"`

	// The system command to execute when the regex hasn't been matched since the Timeout
	TimeoutCommand CommandConfig `yaml:"timeoutCommand"`

	// The following will be passed into the Tail config (http://godoc.org/github.com/hpcloud/tail#Config)
	MustExist   bool `yaml:"mustExist"` // Fail early if the file does not exist (The application will still process other files, just not this one)
	Poll        bool // Poll for file changes instead of using inotify
	Pipe        bool // Is a named pipe (mkfifo)
	MaxLineSize int  `yaml:"maxLineSize"` // If non-zero, split longer lines into multiple lines
}

// Config contains the list of every logfile that should be monitored
type Config []LogConfig

// ParseConfigFile reads in a yaml file and converts it to our config datatypes
func ParseConfigFile(filename string) (config Config, err error) {
	config = Config{}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}

	err = yaml.Unmarshal(data, &config)

	return
}

// Cmd creates an exec.Cmd from the config
func (commandConfig CommandConfig) Cmd() *exec.Cmd {
	return exec.Command(commandConfig.Program, commandConfig.Args...)
}
