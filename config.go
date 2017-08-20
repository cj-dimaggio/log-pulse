package main

import (
	"io/ioutil"
	"os/exec"
	"time"

	"github.com/elastic/beats/filebeat/harvester"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

// To maintain interoperability with FileBeat we need to use their format
// for configurations. Fortunately, Elastic has put a lot of time and effort
// into developing a very well refined configuration system. Unfortunately,
// it can be a little bonkers when you first see it in implementation.
//
// The base library is https://github.com/elastic/go-ucfg but we are primarily
// concerning ourselves with the LibBeat wrapper around this (technically it's
// really just a typedef wrapping ucfg but because of Go's weird namespacing
// with vendor libraries we can't actually use raw ucfg and convert to libeat's
// namespace, but I digress).
//
// The way ucfg (Golang universal configuration) seems to work is you start with
// a textual input, in our case YAML which gets parsed and converted to a
// libeat.common.Config object. Now this is just a very very generic highlevel
// representation of the data. You technically *could* extract the parsed information
// form here, but it's so generic it's ugly and not worth it. What you want to do is
// ".Unpack" from this raw object into an instance of a clean, well defined struct
// you've manually defined (such as CollectorConfig below) and then use that for
// for your configuration. This is actually very helpful because you can then use
// that original raw common.Config in other places to parse different structs from
// the same source data. This is exactly what we're going to do here. Here CollectorConfig
// is parsed from the same base configuration as a FileBeat prospector, so we're able to
// piggy back on those already existing fields (such as "type" and "paths") while adding our
// own ("command" and "timeout") and still pass along those generic common.Config representations
// along to FileBeat functions and have it treat them as just regular configs for Prospects.
// This way, we can configure both our own system and FileBeats with the same YAML. Maybe hopefully...

// CommandConfig contains the required arguments for executing a command
// on the system.
type CommandConfig struct {
	Program string   `config:"program"`
	Args    []string `config:"args"`
}

// Cmd creates an exec.Cmd from the configured command
func (commandConfig CommandConfig) Cmd() *exec.Cmd {
	return exec.Command(commandConfig.Program, commandConfig.Args...)
}

// Start the configured command asynchronously and then return the Cmd
func (commandConfig CommandConfig) Start() (*exec.Cmd, error) {
	logp.Info("Executing command: %s", commandConfig)
	// Let's just run it in the background
	cmd := commandConfig.Cmd()
	err := cmd.Start()
	return cmd, err
}

// TimeoutConfig holds the information for executing a command as the
// result of a timeout.
type TimeoutConfig struct {
	Command  CommandConfig `config:"command"`
	Interval time.Duration `config:"interval"`
	Once     bool          `config:"once"`
}

// CollectorConfig contains all of the information necessary
// for setting up collecting an monitoring. This is an extension
// of the FileBeat's Prospector config and the raw ucfg will be
// passed to it.
type CollectorConfig struct {
	Type    string        `config:"type"`
	Paths   []string      `config:"paths"`
	Pattern string        `config:"pattern"`
	Command CommandConfig `config:"command"`
	Timeout TimeoutConfig `config:"timeout"`
}

// LogPulseConfig is the main holder for all of our configs. It is
// an array of collector configurations.
type LogPulseConfig []CollectorConfig

// ParseConfig reads YAML data and converts it to LogPulseConfig. It also
// returns an array of *common.Configs which can be used to initialize
// FileBeat prospectors.
func ParseConfig(data []byte) (*LogPulseConfig, []*common.Config, error) {
	// We start by Parsing the YAML data into a common.Config instance.
	// Now this is where things get a little complicated with ucfg because
	// the incoming YAML (for our system) is an Array (of CollectorConfigs)
	// but this array is *represented* as a common.Config struct. It's not
	// until we unpack this struct that we'll actually get our arrays.
	raw, err := common.NewConfigWithYAML([]byte(data), "")
	if err != nil {
		return nil, nil, err
	}

	// Now that we have our raw config object we want to map the data that it contains
	// to an actual array of CollectorConfig structs so that we can easily use it.
	// LogPulseConfig is simply a typedef of an Array of CollectorConfigs, so we create
	// one and pass the pointer to raw's "Unpack" method so that it can fill out the array
	// with the data it's parsed from the YAML. It's important to note that this is idempotent
	// in regards to common.Config, it still holds all the data it parsed after a call to Unpack,
	// it just copied some of it over.
	config := LogPulseConfig{}
	err = raw.Unpack(&config)
	if err != nil {
		return nil, nil, err
	}

	// This is another conceptually tricky piece of ucfg. See the issue is that our YAML file
	// defines an array of CollectorConfigs, which (and we can do this because we designed our
	// data types to follow a similar structure) we want to also use to configure a bunch of
	// FileBeat Prospectors. So when a user passed in:
	// - type: log
	//   paths: [/var/*.logs]
	//   command:
	//      program: echo
	//   close_inactive: 1h
	//
	// We want to extract what we can from it (type, paths, command) and pass the config along
	// to FileBeat to extract what *it* can from it (type, paths, close_inactive). And we *should*
	// be able to do that, by passing in raw common.Config representation of that data. The issue is,
	// at this stage we have in the "raw" variable is  essentially a representation of an array of
	// FileBeat Prospector data, and all of the FileBeat Prospector functions expect it to be a
	// representation of a struct.
	//
	// I know it's not very intuitvie and I'm not doing a great job of explaining it. But consider
	// "raw" at this point a weird generic struct that kind of simulates an array, we want to unwind it
	// so that we have a real, primitive array of a weird generic structs that simulate a struct.
	var rawArray []*common.Config
	err = raw.Unpack(&rawArray)
	if err != nil {
		return nil, nil, err
	}

	// Change some of Prospector's defaults
	rawArray, err = setProspectorDefaults(rawArray)
	if err != nil {
		return nil, nil, err
	}

	// Now we can return everything we've parsed
	return &config, rawArray, err
}

// ParseConfigFile parses a YAML file and returns a LogPulseConfig as well as an array
// of *common.Configs for creating FileBeat prospectors.
func ParseConfigFile(filename string) (*LogPulseConfig, []*common.Config, error) {
	// Pull out all the data from the file
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}

	// Send it along to ParseConfig
	return ParseConfig(data)
}

var (
	// DefaultProspectorConfig specifies the default fields we want to
	// pass along for Prospector configuration to make file process
	// happen a bit faster
	DefaultProspectorConfig = prospectorConfig{
		Type:          harvester.LogType,
		TailFiles:     true,
		Backoff:       200 * time.Millisecond,
		BackoffFactor: 1,
		MaxBackoff:    1 * time.Second,
		ScanFrequency: 3 * time.Second,
	}
)

// prospectorConfig is a recreation of some of the Prospector configs we want
// to override defaults for.
type prospectorConfig struct {
	Type          string        `config:"type"`
	TailFiles     bool          `config:"tail_files"`
	Backoff       time.Duration `config:"backoff" validate:"min=0,nonzero"`
	BackoffFactor int           `config:"backoff_factor" validate:"min=1"`
	MaxBackoff    time.Duration `config:"max_backoff" validate:"min=0,nonzero"`
	ScanFrequency time.Duration `config:"scan_frequency" validate:"min=0,nonzero"`
}

// setProspectorDefaults is a bit more of ucfg weirdness. FileBeat sets up a bunch
// of default values for it's configuration. These are generally good for log shipping
// but not so great for the more reactive process we're trying to create so we want to
// override FileBeat's defaults with our own (such as tail files by default and lower
// the backoff time). In order to do this we're going to have to parse some of these
// FileBeat specific fields first using our own default configuration and then merge
// that into the canonical common.Config so that FileBeat thinks that it's coming from
// the user.
func setProspectorDefaults(configs []*common.Config) ([]*common.Config, error) {
	for _, rawConfig := range configs {
		// Copy the default config so it doesn't get overwritten
		conf := DefaultProspectorConfig

		// Apply the user's configuration on top of our default configuration
		err := rawConfig.Unpack(&conf)
		if err != nil {
			return nil, err
		}

		// Merge our struct back into the main config file so that FileBeat gets
		// our default configs as user specifications.
		err = rawConfig.Merge(conf)
		if err != nil {
			return nil, err
		}
	}
	return configs, nil
}
