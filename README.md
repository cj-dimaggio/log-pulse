# Log Pulse
Log Pulse is a small utility for tailing and monitoring log files, checking each incoming line against a regular expression and executing commands in response to the existence of a match or after a timeout of not seeing a match.

## Configuring
Log Pulse is configured through a YAML file. By default it will look for `log-pulse.yml` in the working directory but this can be changed through the `-c` or `--config` flags, such as:
```
log-pulse -c /etc/log-pulse.yml
log-pulse --config=/etc/log-pulse.yml
```

The configuration file itself defines a list of "collectors"; logical units that monitor and tail groups of paths to process and tail their inputs in semi real-time.
Configuration looks like:
```
# The top level element is a list
-
  # Each element in the list denotes one or more files whose inputs you'd like
  # to track

  # Denotes which files should be tracked. (required)
  # You can specify a list of individual files or you can use globbing
  paths:
    - /usr/local/var/nginx.log
    - /var/log/*.log

  # The regular expression pattern to match incoming lines against (required)
  pattern: ^Begins-With

  # Command to be run when a line matching the pattern comes in from any of the tracked
  # files (optional)
  command:
    # command name or path to execute
    # (Go will try to expand program names to paths, such as "touch" => "/usr/bin/touch", automatically for you)
    program: /usr/bin/touch
    # List of arguments to be passed to the executing program
    args:
      - /tmp/pattern-matched

  # Configures actions to be taken if the pattern does not match and of the incoming
  # data for a certain period of time. (optional)
  timeout:
    # The interval to wait for a pattern to match an log line, after which the timeout
    # command will be executed.
    # The value should be parsable by https://golang.org/pkg/time/#ParseDuration
    # (ie: 30s => "30 Seconds", 1h => "1 hour". Lower values are also allowed such as
    # 300ms but the log aggregation is usually relativly too slow for sub-second duration
    # to be reliable)
    interval: 30s

    # The command to execute when a timeout occurs
    command:
      program: /usr/bin/touch
      args:
        - /tmp/timed-out

    # By default "timeout" is used as a kind of interval; so for every N seconds without seeing a pattern
    # the timeout.command will execute. 'timeout.once' allows you to override this behavior so that the command
    # only executes *once* until it see's the pattern again (at which point the timer resets)
    once: true
```

Log Pulse uses [ucfg](https://github.com/elastic/go-ucfg) for its configuration, which also supports dot notation, so the previous could also be written as:
```
- paths: [/usr/local/var/nginx.log, /var/log/*.log]
  pattern: ^Begins-With
  command.program: /usr/bin/touch
  command.args: [/tmp/pattern-matched]
  timeout.interval: 30s
  timeout.command.program: /usr/bin/touch
  timeout.command.args: [/tmp/timed-out]
  timeout.once: true
```

And of course YAML is a superset of JSON, so if you're more comfortable with that format you can equally write:
```
[
  {
    "paths": [
      "/usr/local/var/nginx.log",
      "/var/log/*.log"
    ],
    "pattern": "^Begins-With",
    "command": {
      "program": "/usr/bin/touch",
      "args": [
        "/tmp/pattern-matched"
      ]
    },
    "timeout": {
      "interval": "30s",
      "command": {
        "program": "/usr/bin/touch",
        "args": [
          "/tmp/timed-out"
        ]
      },
      "once": true
    }
  }
]
```

### Advanced Configuration
Log Pulse is built using large components of [Filebeat](https://github.com/elastic/beats). In fact, each element in a Log Pulse array is essentially just a wrapper around a FileBeat "Prospector" and [all of the configurations available for one](https://www.elastic.co/guide/en/beats/filebeat/current/configuration-filebeat-options.html) are equally available here. Most of these don't make much sense in the context of Log Pulse (such as "exclude_lines", "fields", etc) but you're free to set them, along with the more advanced features that dictate how aggressively your files are polled:
```
- paths: [/tmp/test.log]
  pattern: .*
  scan_frequency: 5s
  max_backoff: 2s
```

Because of the responsive nature of Log Pulse you'll have to judge for yourself how close to real-time you need your results against how hard you want to hit your filesystem. Log Pulse overrides some of Filebeat's defaults to make the file collection more responsive out of the box and these can be viewed in the `config.go` file under the `DefaultProspectorConfig` variable.

In fact, you're not even necessarily limited to using log file for input. Filebeat supports a number of input types including Redis, Stdin, and UDP which can be changed by changing the "type" field from its default "log" and all of these can be used with Log Pulse. This has not be thoroughly tested however and is more of just a theoretical.

## Building
Log Pulse is a raw Go application so it should be trivial to build both for your own platform and for cross-platform, assuming [you have a Go environment properly setup](https://golang.org/doc/install). Dependencies are vendored using [Glide](https://github.com/Masterminds/glide) and can be installed with:
```
glide install
```
To compile for your own platform just run:
```
go build
```
which will build a `./log-pulse` binary.

To cross-compile with Go > 1.5 you can use the GOOS and GOARCH environment variables:
```
GOOS=windows GOARCH=amd64 go build
GOOS=darwin GOARCH=amd64 go build
```

## Testing
Traditional Go unit tests have been written and can be run via:
```
go test
```
in the project root.

There are additional ["integration" tests](./integration-test-unix) that, instead of testing the internal logic in Go, runs the build application through the shell in a bash script with a full configuration. This can be run with:
```
./integration-test-unix/run
```

## Structure
I'm not a huge fan of how Go somewhat wants all of your sources in the root directory (at least in the Go tooling sense). We could have nested some things into packages but then we would have needed to import those packages via the github repo url in the source which always make forking a pain. So for the simplicity of anybody who wants to fork this project, we may just need to live with it looking a bit ugly for now. But it's a small enough project.
