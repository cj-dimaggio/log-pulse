package main

import (
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/elastic/beats/filebeat/channel"
	"github.com/elastic/beats/filebeat/input/file"
	"github.com/elastic/beats/filebeat/prospector"
	"github.com/elastic/beats/filebeat/util"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

// Collector in our program is really just going to be a glorified wrapper
// around a FileBeat Prospector. Mostly because I don't the name is very
// good. We'll be using FileBeat's Prospectors and Harvestors to pool input
// from the system. We'll be treating globs of files as essentially a singal
// input. Each input from each one will be matched against the pattern and
// each will be able to reset the timeout for the entire collection.
type Collector struct {
	// Holds our platform specific configuration
	config CollectorConfig

	// The FileBeat object that will actually be doing the collecting
	prospector *prospector.Prospector
	// Will be triggered with a close when the Prospector's "Stop" is called.
	// This trigger will happen *before* the Prospector waits for its WaitGroup, that
	// is signified by Prospector.Stop returning
	prospectorDone chan struct{}

	Pattern *regexp.Regexp

	// lines is the main channel that the CollecturOutleter will send incoming log lines
	// to for processing. We could send over the entire beat.Event but for now that would
	// just add bloat to our channel and require extra validation. For now we're really just
	// concerned about the message and will be hoping we're reactive enough to be processing
	// things in near real-time
	lines chan string

	// Done is our internal signal to notify ourselves when our Collector processing logic
	// should start shutting down.
	Done chan struct{}
	// Stopped is used to notify when the collector has successfully stopped.
	Stopped chan struct{}

	// Used to track our timeout process
	timeoutChannel <-chan time.Time
	ticker         *time.Ticker
}

// NewCollector initializes a new Collector object along with its associated communication
// channels
func NewCollector(config CollectorConfig, rawConfig *common.Config) (*Collector, error) {

	// Compile the configured pattern
	pattern, err := regexp.Compile(config.Pattern)
	if err != nil {
		logp.Warn("Unable to parse regular expression: %s", err)
		return nil, err
	}

	// Create our Collector with its channel signals
	collector := Collector{
		Pattern: pattern,
		config:  config,

		prospectorDone: make(chan struct{}),
		lines:          make(chan string),
		Done:           make(chan struct{}),
		Stopped:        make(chan struct{}),
	}

	// Initialize our ticker for handling timeouts
	if config.Timeout.Interval > 0 {
		// If a timeout is set then create a new ticker and save wrap its channel with a variable
		collector.ticker = time.NewTicker(config.Timeout.Interval)
		collector.timeoutChannel = collector.ticker.C
	} else {
		// If a timeout is not set then create just a generic channel that will never return.
		// It just makes generalizing the code easier.
		collector.timeoutChannel = make(chan time.Time)
	}

	// Configure a new FileBeat Prospector with our rawConfig that will send it's data to a
	// CollectorOutleter
	p, err := prospector.NewProspector(
		rawConfig,
		collector.collectorOutleterFactory,
		collector.prospectorDone,
		[]file.State{},
	)
	if err != nil {
		return nil, err
	}

	collector.prospector = p
	return &collector, nil
}

// Start begins the underlying prospector and starts processing incoming data. This function
// will return immediately and begin its processing in gorotutines. To wait for it to finish
// you can use the "AllowRun" method which will block until a shutdown signal comes in from
// another routine
func (collector *Collector) Start() {
	// Begin our internal processing first
	go collector.process()

	// Start the prospector to start collecting data
	collector.prospector.Start()
}

// Stop triggers a shutdown of the prospector and the data processor. For we're only going
// to support the ability to Start and Stop the collector *once*, after which a lot of the
// channels will be closed to signal the shutdown even. You will need to recreate he Collector
// if you want to start it back up (This restriction is mostly from what I can grok of FileBeat,
// which seems to have this underlying restriction and I'm more than happy to piggy back on).
// This function waits until the Prospector and it's worker's has been successfully shutdown
func (collector *Collector) Stop() {
	// Stop the underlying Prospector (this should block until all workers shutdown)
	collector.prospector.Stop()

	// Signal our internal processing to stop as well. It's probably safer to do this
	// after we've stopped the prospector just to make sure we handle as much data as possible
	close(collector.Done)
	// Wait for our collector to tell us its finished shutting down.
	<-collector.Stopped

	if collector.ticker != nil {
		collector.ticker.Stop()
	}
}

// LetRun will block until the Collector is stopped and fully shutdown. You'll want to make
// sure you actually call Start and Stop on the collector or else this will never return (duh)
func (collector *Collector) LetRun() {
	// Wait for our signal telling us that the Collector has stopped
	<-collector.Stopped
}

// process is the main business logic of our collector, which will collect data from the Outleter
// and do the regex matching and timeout logic and executing of commands.
func (collector *Collector) process() {
	// Signal that the collector has stopped when we return.
	defer func() {
		close(collector.Stopped)
	}()

	logp.Info("Starting collector processing")

	// What we'll use for keeping track of Timeout.Once, so that a command only executes once
	// between pattern matches and not at an interval
	timedOutOnce := false

	// Continuously select over our channels and signals waiting for an event
	for {
		select {
		case msg := <-collector.lines:
			// We've gotten a new log line
			logp.Debug("log-pulse", "Collector received message: %s", msg)
			if collector.Pattern.MatchString(msg) {
				logp.Debug("log-pulse", "Message matches pattern")

				// The line matches our pattern so reset our timeout
				collector.resetTimeout()

				// Reset our timedOutOnce so that another timeout command can execute
				timedOutOnce = false

				// If a command is configured to be run on pattern matches execute it
				if collector.config.Command.Program != "" {
					logp.Info("Running pattern match command...")
					collector.config.Command.Start()
				}
			}
		case t := <-collector.timeoutChannel:
			logp.Debug("log-pulse", "Timed Out", t)

			// Our ticker has timed-out
			// Only do anything if there's an actual timeout command configured
			if collector.config.Timeout.Command.Program != "" {
				if !(timedOutOnce && collector.config.Timeout.Once) {
					// Only run our command if TimeoutOnce isn't set or, if it is,
					// only if we haven't run the command yet.
					logp.Info("Running timeout command...")
					collector.config.Timeout.Command.Start()
				}
			}
			timedOutOnce = true
		case <-collector.Done:
			// We got a shutdown signal
			logp.Info("Collector received shutdown signal and is going to close")
			return
		}
	}
}

// collectorOutleterFactory is sent to the Prospector to create an Outleter that will recieve the
// log data for all of this prospector's managed files (all defined paths and expanded globs will
// be pooled there)
func (collector *Collector) collectorOutleterFactory(*common.Config) (channel.Outleter, error) {
	// Pass along our channel so we can get messages from the generates Outleter
	return &CollectorOutleter{
		lines: collector.lines,
	}, nil
}

// resetTimeout resets the ticker so that it starts counting again from this point in time
func (collector *Collector) resetTimeout() {
	// We only need to do something if there actually is a ticker (ie: if an interval was specified)
	if collector.ticker != nil {
		// Stop the ticker so it can be garbage collected
		collector.ticker.Stop()

		// From everything I've read the only real way to reset a ticker is to recreate it
		collector.ticker = time.NewTicker(collector.config.Timeout.Interval)
		collector.timeoutChannel = collector.ticker.C
	}
}

// CollectorOutleter gets called when the Prospector emits new events
// or closes
type CollectorOutleter struct {
	lines chan string
}

// OnEvent is called by FileBeat harvesters Forwarder and passes file events and incoming log data. It is
// used by all the harvesters that the Prospector creates, making it the ideal place to aggregate all of our
// Collector's streams.
//
// This is called synchronously from the harvester.Forwarder to ensure that states don't get overwritten.
// As such we should try to return as quickly as possible and just send our data over to another goroutine
// to process.
func (outlet *CollectorOutleter) OnEvent(data *util.Data) bool {
	// We'll actually receive a few spurious blank events that FileBeat likes to use to keep its registry
	// of file offsets up-to-date. We're really only interested in events that have messages, and we're really
	// only concerned with the messages themselves. FileBeat creates the events, typically, in the harvester.
	// To see the generation of these events look at log.harverster's Run method.
	event := data.GetEvent()
	if event.Fields != nil {
		// We only want to send over events that actually have message fields (which should actually be all
		// of them, but just in case). So this is just Go's way of saying "if map event.Fields has a key
		// 'message' (while also storing the value at that key to 'msg')"
		if msg, ok := event.Fields["message"]; ok {
			// "msg" at this stage is just a generic interface{}, which is kind of the closest Go has to
			// a void pointer. We want to try to cast it to a string (which it always should be) before sending
			// it down the wire.
			if str, ok := msg.(string); ok {
				// Send the line over our channel
				outlet.lines <- str
			} else {
				logp.Warn("Encountered non string message field: %s", msg)
			}
		}
	}

	// The boolean we return indicates whether we were able to enqueue the data or not. For our purposes,
	// since we're not actually using a complicated Spool feature like FileBeat, we can just say we were
	// able to.
	return true
}

// Close will be called by the log.Prospector after all of its workers have been shutdown. This would be
// a good place to do some cleanup work.
func (outlet *CollectorOutleter) Close() error {
	// There's really not a whole lot of cleanup we want to do here (Collector will handle it's own shutfown
	// process). So let's just log and return
	logp.Info("CollectorOutleter closing")
	return nil
}

// Collection holds and handles an array of Collector instances
type Collection struct {
	collectors []*Collector

	// Used to wait for all Collectors to finish
	wg sync.WaitGroup
}

// CreateCollection iterates through a LogPulseConfig and returns a Collection object which can run the
// multiple Collectors concurrently.
func CreateCollection(configs LogPulseConfig, rawConfigs []*common.Config) (*Collection, error) {
	if len(configs) != len(rawConfigs) {
		return nil, errors.New("LogPulseConfig and rawConfigs must contain the same number of elements")
	}

	var collectors []*Collector
	for i, conf := range configs {
		if c, err := NewCollector(conf, rawConfigs[i]); err == nil {
			collectors = append(collectors, c)
		} else {
			logp.Warn("Unable to create a collector. Skipping. %s", err)
		}
	}

	if len(collectors) == 0 {
		return nil, errors.New("No Collectors created")
	}

	return &Collection{
		collectors: collectors,
	}, nil
}

// Start begins all of the Collectors associated with the Collection
func (collection *Collection) Start() {
	for _, c := range collection.collectors {
		c.Start()
		collection.wg.Add(1)
	}
}

// Stop all of the Collectors
func (collection *Collection) Stop() {
	for _, c := range collection.collectors {
		c.Stop()
		collection.wg.Done()
	}
}

// LetRun blocks until all of the managed Collectors are stopped
func (collection *Collection) LetRun() {
	collection.wg.Wait()
}
