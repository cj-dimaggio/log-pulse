# Log Pulse
Log Pulse is a small utility for tailing and monitoring log files, checking each incoming line against a regular expression and executing commands in response to the existence of a match or after a timeout of not seeing a match.

## Configuring
Log Pulse is configured through a YAML file. By default it will look for `log-pulse.yml` in the working directory but this can be changed through the `-c` or `--config` flags, such as:
```
log-pulse -c /etc/log-pulse.yml
log-pulse --config=/etc/log-pulse.yml
```

The configuration file itself defines a list of log files to track and the logic to use when parsing it:
```
-
  # The path to the log file to track and tail (required)
  file: /var/log/nginx.log

  # The regular expression pattern to match incoming lines against (required)
  pattern: ^Begins-With

  # Command to run when a line matching the pattern comes in (optional)
  command:
    # command name or path to execute
    # (Go will try to expand program names to paths, such as "touch" => "/usr/bin/touch", automatically for you)
    program: /usr/bin/touch
    # List of arguments to be passed to the program
    args:
      - /tmp/pattern-matched

  # Time, in seconds, to wait for a pattern before triggering a timeout (optional)
  timeout: 30

  # Command to be executed when a timeout occurs (optional)
  timeoutCommand:
    program: /usr/bin/touch
    args:
      - /tmp/timed-out

  # By default "timeout" is used as a kind of interval; so for every N seconds without seeing a pattern
  # the timeoutCommand will execute. 'timeoutOnce' allows you to override this behavior so that the command
  # only executes *once* until it see's the pattern again (at which point the timer resets)
  timeoutOnce: true

  # By default if log-pulse is configured for a file that doesn't exist yet the goroutine will wait in
  # the background until it does and start tailing it. 'mustExist' supresses that behavior and will not
  # bother tracking the file if it doesn't exist on startup.
  mustExist: true

  # Allows you to tell the https://github.com/hpcloud/tail to use it's poll strategy.
  poll: false

  # Tells https://github.com/hpcloud/tail that the file is a fifo named pipe
  pipe: false

  # Any non-zero number tells https://github.com/hpcloud/tail to split long lines at this max
  maxLineSize: 0

```

Of course YAML is a superset of JSON, so the configuration can also be written as:
```
[
  {
    "file": "/var/log/nginx.log",
    "pattern": "^Begins-With",
    "command": {
      "program": "/usr/bin/touch",
      "args": [
        "/tmp/pattern-matched"
      ]
    },
    "timeout": 30,
    "timeoutCommand": {
      "program": "/usr/bin/touch",
      "args": [
        "/tmp/timed-out"
      ]
    },
    "timeoutOnce": true,
    "mustExist": true,
    "poll": false,
    "pipe": false,
    "maxLineSize": 0
  }
]
```
if one is more comfortable with JSON.

## Building
Log Pulse is a raw Go application so it should be trivial to build both for your own platform and for cross-platform, assuming [you have a Go environment properly setup](https://golang.org/doc/install). To compile for your own platform just run:
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
