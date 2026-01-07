# Dispatch
[![Go Reference](https://pkg.go.dev/badge/github.com/nicois/dispatch.svg)](https://pkg.go.dev/github.com/nicois/dispatch)

Run multiple variations of a command, controlling concurrency, retries, etc.

## Features

- run multiple jobs concurrently
- captures STDOUT and STDERR of each job separately, for both successful and unsuccessful jobs
- allows job arguments to be described in a variety of formats (one per line, JSON per line, or CSV)
- sensible console status messages providing a progress summary
- repeated CTRL-Cs are used to progressively increase jobs' termination priority

Optionally:

- skips and/or deprioritises jobs which have already been run (unless instructed otherwise, based on time since last successful execution)
- define timeouts
- abort on job failure
- inject STDIN to each job
- use S3(-compatible) backend to store state

... and more!

## Installation

```bash
go install github.com/nicois/dispatch/dispatch@latest
```

The binary will be installed into `~/go/bin/`

## Usage

```
Usage:
  dispatch [OPTIONS]

preparation:
      --csv                     interpret STDIN as a CSV
      --debounce-failures=      re-run failed jobs outside the debounce period, even if they would normally be skipped
      --debounce-successes=     re-run successful jobs outside the debounce period, even if they would normally be skipped
      --defer-delay=            when deferring reruns, wait some time before beginning processing
      --defer-reruns            give priority to jobs which have not previously been run
      --json-line               interpret STDIN as JSON objects, one per line
      --shuffle                 disregard the order in which the jobs were given
      --skip-failures           skip jobs which have already been run unsuccessfully
      --skip-successes          skip jobs which have already been run successfully

execution:
      --abort-on-error          stop running (as though CTRL-C were pressed) if a job fails
      --cache-location=         path (or S3 URI) to record successes and failures
      --concurrency=            run this many jobs in dispatch (default: 10)
      --dry-run                 simulate what would be run
      --input=                  send the input string (plus newline) forever as STDIN to each job
      --rate-limit=             prevent jobs starting more than this often
      --rate-limit-bucket-size= allow a burst of up to this many jobs when enforcing the rate limit
      --timeout=                cancel each job after this much time

output:
      --debug                   show more detailed log messages
      --hide-failures           do not display a message each time a job fails
      --hide-successes          do not display a message each time a job succeeds
      --show-stderr             send a copy of each job's STDERR to the console
      --show-stdout             send a copy of each job's STDOUT to the console
```

## Examples

#### Basic operation

Run three variations of `echo`, substituting `{{.value}}` with each input line in turn

```bash
$ echo -e 'one\ntwo\nthree' \
    | dispatch -- echo {{.value}}
Dec 22 08:09:29.670 INF Success command="{command:[echo three] input:}" "combined output"="three\n"
Dec 22 08:09:29.670 INF Success command="{command:[echo two] input:}" "combined output"="two\n"
Dec 22 08:09:29.670 INF Success command="{command:[echo one] input:}" "combined output"="one\n"
Dec 22 08:09:29.670 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3; Elapsed time: 0s
```

In fact, if `dispatch` is run without supplying a command, it does the same thing:

```bash
$ echo -e 'one\ntwo\nthree' | dispatch
Dec 22 08:10:00.810 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:10:00.914 INF Success command="{command:[echo value is two] input:}" "combined output"="value is two\n"
Dec 22 08:10:00.914 INF Success command="{command:[echo value is one] input:}" "combined output"="value is one\n"
Dec 22 08:10:00.914 INF Success command="{command:[echo value is three] input:}" "combined output"="value is three\n"
Dec 22 08:10:00.914 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3; Elapsed time: 0s
```

#### JSON parsing

Parse each input line as a JSON object

```bash
$ echo -e '{"animal": "cat", "name": "Scarface Claw"}\n{"animal": "dog", "name": "Bitzer Maloney"}' \
    | dispatch --json-line -- echo the {{.animal}} is called {{.name}}
Dec 22 08:10:19.143 INF Success command="{command:[echo the cat is called Scarface Claw] input:}" "combined output"="the cat is called Scarface Claw\n"
Dec 22 08:10:19.143 INF Success command="{command:[echo the dog is called Bitzer Maloney] input:}" "combined output"="the dog is called Bitzer Maloney\n"
Dec 22 08:10:19.144 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Elapsed time: 0s
```

#### CSV parsing

```bash
$ echo -e 'animal,name\ncat,Scarface Claw\ndog,Bitzer Maloney' \
    | dispatch --csv -- echo the {{.animal}} is called {{.name}}
Dec 22 08:10:41.909 INF Success command="{command:[echo the cat is called Scarface Claw] input:}" "combined output"="the cat is called Scarface Claw\n"
Dec 22 08:10:41.909 INF Success command="{command:[echo the dog is called Bitzer Maloney] input:}" "combined output"="the dog is called Bitzer Maloney\n"
Dec 22 08:10:41.909 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Elapsed time: 0s

```

#### Status logging

Every 10 seconds an interim status is generated, as well as at completion. An estimate of the remaining time will be shown,
based solely on the rate of completion of earlier jobs.
Duplicate status messages, where nothing has changed, will be suppressed for up to a minute.

```bash
$ seq 1 10 \
    | dispatch --concurrency 4 -- bash -c 'echo {{.value}} ; sleep 4'
Dec 22 08:12:07.262 INF Success command="{command:[bash -c echo 2 ; sleep 4] input:}" "combined output"="2\n"
Dec 22 08:12:07.262 INF Success command="{command:[bash -c echo 3 ; sleep 4] input:}" "combined output"="3\n"
Dec 22 08:12:07.262 INF Success command="{command:[bash -c echo 1 ; sleep 4] input:}" "combined output"="1\n"
Dec 22 08:12:07.262 INF Success command="{command:[bash -c echo 4 ; sleep 4] input:}" "combined output"="4\n"
Dec 22 08:12:10.002 INF Queued: 2; In progress: 4; Succeeded: 4; Failed: 0; Aborted: 0; Total: 10; Estimated time remaining: 6 seconds
Dec 22 08:12:11.267 INF Success command="{command:[bash -c echo 8 ; sleep 4] input:}" "combined output"="8\n"
Dec 22 08:12:11.267 INF Success command="{command:[bash -c echo 6 ; sleep 4] input:}" "combined output"="6\n"
Dec 22 08:12:11.267 INF Success command="{command:[bash -c echo 7 ; sleep 4] input:}" "combined output"="7\n"
Dec 22 08:12:11.267 INF Success command="{command:[bash -c echo 5 ; sleep 4] input:}" "combined output"="5\n"
Dec 22 08:12:15.272 INF Success command="{command:[bash -c echo 9 ; sleep 4] input:}" "combined output"="9\n"
Dec 22 08:12:15.272 INF Success command="{command:[bash -c echo 10 ; sleep 4] input:}" "combined output"="10\n"
Dec 22 08:12:15.272 INF Queued: 0; In progress: 0; Succeeded: 10; Failed: 0; Aborted: 0; Total: 10; Elapsed time: 12s
```

#### Skipping previously-run jobs

If a job has already been attempted, and should not be re-attempted, use `--skip-successes` and/or `--skip-failures` as applicable:

```bash
$ seq 2 | dispatch --skip-successes
Dec 22 08:12:48.198 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:12:48.301 INF Success command="{command:[echo value is 1] input:}" "combined output"="value is 1\n"
Dec 22 08:12:48.301 INF Success command="{command:[echo value is 2] input:}" "combined output"="value is 2\n"
Dec 22 08:12:48.301 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Elapsed time: 0s


$ seq 5 | dispatch --skip-successes
Dec 22 08:12:53.395 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:12:53.498 INF Success command="{command:[echo value is 3] input:}" "combined output"="value is 3\n"
Dec 22 08:12:53.498 INF Success command="{command:[echo value is 4] input:}" "combined output"="value is 4\n"
Dec 22 08:12:53.498 INF Success command="{command:[echo value is 5] input:}" "combined output"="value is 5\n"
Dec 22 08:12:53.499 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3 (+2 skipped); Elapsed time: 0s
```

Notice the `skipped` value in the stats line.

#### Debounce period

If you only want to skip jobs which haven't succeeded/failed recently, you can provide a debounce period using `--debounce-successes` and/or `--debounce-failures`.
Be aware that this period is assessed when the STDIN record is parsed, not when the job is about to start.

Below, 2 jobs are run, then 3 more 10 seconds later. With a debounce of 10s, this means the third execution skips the 3 recent jobs:

```bash
$ seq 2 | dispatch --skip-successes ; sleep 10; seq 5 | dispatch --skip-successes ; seq 5 | dispatch --skip-successes --debounce-successes 10s
Dec 22 08:15:19.995 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:15:20.000 INF Queued: 2; In progress: 0; Succeeded: 0; Failed: 0; Aborted: 0; Total: 2; Elapsed time: 0s
Dec 22 08:15:20.099 INF Success command="{command:[echo value is 2] input:}" "combined output"="value is 2\n"
Dec 22 08:15:20.099 INF Success command="{command:[echo value is 1] input:}" "combined output"="value is 1\n"
Dec 22 08:15:20.099 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Elapsed time: 0s

Dec 22 08:15:30.113 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:15:30.216 INF Success command="{command:[echo value is 3] input:}" "combined output"="value is 3\n"
Dec 22 08:15:30.216 INF Success command="{command:[echo value is 4] input:}" "combined output"="value is 4\n"
Dec 22 08:15:30.216 INF Success command="{command:[echo value is 5] input:}" "combined output"="value is 5\n"
Dec 22 08:15:30.217 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3 (+2 skipped); Elapsed time: 0s

Dec 22 08:15:30.226 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:15:30.329 INF Success command="{command:[echo value is 1] input:}" "combined output"="value is 1\n"
Dec 22 08:15:30.329 INF Success command="{command:[echo value is 2] input:}" "combined output"="value is 2\n"
Dec 22 08:15:30.330 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2 (+3 skipped); Elapsed time: 0s

```

#### Deprioritising recently-run jobs

By default, jobs are started in the order they are provided via STDIN.
If desired `--defer-reruns` will notice if a job has been run previously (whether successful or not), and will run other jobs first.
Where multiple jobs are reruns, priority is given to least recently-run jobs.
Where jobs have never been run before, the order provided in STDIN is respected.

```bash
$ seq 5 | dispatch --concurrency=5 ; seq 10 | dispatch --defer-reruns --concurrency=5
Dec 22 08:42:30.726 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:42:30.727 INF Success command="{command:[echo value is 2] input:}" "combined output"="value is 2\n"
Dec 22 08:42:30.727 INF Success command="{command:[echo value is 1] input:}" "combined output"="value is 1\n"
Dec 22 08:42:30.727 INF Success command="{command:[echo value is 3] input:}" "combined output"="value is 3\n"
Dec 22 08:42:30.727 INF Success command="{command:[echo value is 5] input:}" "combined output"="value is 5\n"
Dec 22 08:42:30.727 INF Success command="{command:[echo value is 4] input:}" "combined output"="value is 4\n"
Dec 22 08:42:30.728 INF Queued: 0; In progress: 0; Succeeded: 5; Failed: 0; Aborted: 0; Total: 5; Elapsed time: 0s

Dec 22 08:42:30.730 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Dec 22 08:42:30.834 INF Success command="{command:[echo value is 9] input:}" "combined output"="value is 9\n"
Dec 22 08:42:30.834 INF Success command="{command:[echo value is 6] input:}" "combined output"="value is 6\n"
Dec 22 08:42:30.834 INF Success command="{command:[echo value is 10] input:}" "combined output"="value is 10\n"
Dec 22 08:42:30.834 INF Success command="{command:[echo value is 7] input:}" "combined output"="value is 7\n"
Dec 22 08:42:30.834 INF Success command="{command:[echo value is 8] input:}" "combined output"="value is 8\n"
Dec 22 08:42:30.836 INF Success command="{command:[echo value is 1] input:}" "combined output"="value is 1\n"
Dec 22 08:42:30.836 INF Success command="{command:[echo value is 4] input:}" "combined output"="value is 4\n"
Dec 22 08:42:30.836 INF Success command="{command:[echo value is 3] input:}" "combined output"="value is 3\n"
Dec 22 08:42:30.836 INF Success command="{command:[echo value is 2] input:}" "combined output"="value is 2\n"
Dec 22 08:42:30.836 INF Success command="{command:[echo value is 5] input:}" "combined output"="value is 5\n"
Dec 22 08:42:30.836 INF Queued: 0; In progress: 0; Succeeded: 10; Failed: 0; Aborted: 0; Total: 10; Elapsed time: 0s

```

To make `--defer-reruns` more effective, a small delay is introduced before jobs start being executed.
During this period, jobs are collected and sorted, making it more likely that the right jobs will be run first.
`--defer-delay` can override the length of this delay, which defaults to 100ms.

#### Suppressing success and/or failure messages

If you want a less noisy output, you can suppress success and/or failure messages. STDOUT and STDERR are still logged to
the filesystem as normal:

```bash
$ seq 1 254 | dispatch --hide-failures --concurrency 100 --debounce 10s --timeout 10s -- nc -vz 192.168.4.{{.value}} 443
Dec 22 08:47:59.126 INF Success command="{command:[nc -vz 192.168.4.53 443] input:}" "combined output"="Ncat: Version 7.92 ( https://nmap.org/ncat )\nNcat: Connected to 192.168.4.53:443.\nNcat: 0 bytes sent, 0 bytes received in 0.06 seconds.\n"
Dec 22 08:48:00.000 INF Queued: 144; In progress: 100; Succeeded: 1; Failed: 9; Aborted: 0; Total: 254; Estimated time remaining: 88 seconds
Dec 22 08:48:05.378 INF Success command="{command:[nc -vz 192.168.4.222 443] input:}" "combined output"="Ncat: Version 7.92 ( https://nmap.org/ncat )\nNcat: Connected to 192.168.4.222:443.\nNcat: 0 bytes sent, 0 bytes received in 0.09 seconds.\n"
Dec 22 08:48:09.429 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 252; Aborted: 0; Total: 254; Estimated time remaining: 3 seconds
```

### Rate limiting

Sometimes, despite wanting to run jobs concurrently, you want to place a limit on the maximum rate jobs can be started at. For example, you might want to run 4 jobs at a time, but wait 2 seconds between them:

```bash
dispatch --rate-limit 2s --concurrency 4
```

If bursting is acceptable, `--rate-limit-bucket-size` allows this.

If you want to issue some API commands, ensuring no more than 1 is started per second, with a burst of 3 (but allowing 10 to run concurrently):

```bash
dispatch --rate-limit 1s --rate-limit-bucket-size 3
```

### Dry-run

Want to ensure the right command will be run with the correct inputs? `--dry-run` will do this. Nothing will actually be executed.
An implicit 1 second sleep will be substituted for the actual execution of each command:

```bash
$ seq 8 | dispatch --dry-run --debounce 5s --concurrency 1 --input y -- rm -f foo.{{.value}}
Dec 22 08:49:02.035 INF Success command="{command:[rm -f foo.1] input:y}" "combined output"="(dry run)"
Dec 22 08:49:03.036 INF Success command="{command:[rm -f foo.2] input:y}" "combined output"="(dry run)"
Dec 22 08:49:04.037 INF Success command="{command:[rm -f foo.3] input:y}" "combined output"="(dry run)"
Dec 22 08:49:05.038 INF Success command="{command:[rm -f foo.4] input:y}" "combined output"="(dry run)"
Dec 22 08:49:06.039 INF Success command="{command:[rm -f foo.5] input:y}" "combined output"="(dry run)"
Dec 22 08:49:07.040 INF Success command="{command:[rm -f foo.6] input:y}" "combined output"="(dry run)"
Dec 22 08:49:08.041 INF Success command="{command:[rm -f foo.7] input:y}" "combined output"="(dry run)"
Dec 22 08:49:09.042 INF Success command="{command:[rm -f foo.8] input:y}" "combined output"="(dry run)"
Dec 22 08:49:09.042 INF Queued: 0; In progress: 0; Succeeded: 8; Failed: 0; Aborted: 0; Total: 8; Elapsed time: 8s

```

### Shuffle / randomise

Usually, if you want to run the jobs in a random order, you can pipe STDIN via `shuf` beforehand.
However, if the source of jobs is dynamic, you might not want to wait until all jobs are generated before
any jobs are started.

`--shuffle` will disregard the order in which jobs were received, but will work as expected with respect to
`--defer-reruns`. This means your jobs will start being processed without delay, and reruns will still be
run only after new jobs, but the new jobs will be run in a random order. (The rerun jobs are not randomised,
as they are selected based on the time the job was last attempted.)

### Job cancellations and timeouts

Defining a timeout will cause jobs to be terminated if it is reached:

```bash
$ seq 1 7 \
    | dispatch --concurrency 2 --timeout 5s -- bash -c 'echo {{.value}} ; sleep {{.value}}'
Dec 22 08:50:10.000 INF Queued: 5; In progress: 2; Succeeded: 0; Failed: 0; Aborted: 0; Total: 7; Elapsed time: 1s
Dec 22 08:50:10.059 INF Success command="{command:[bash -c echo 1 ; sleep 1] input:}" "combined output"="1\n"
Dec 22 08:50:11.059 INF Success command="{command:[bash -c echo 2 ; sleep 2] input:}" "combined output"="2\n"
Dec 22 08:50:13.064 INF Success command="{command:[bash -c echo 3 ; sleep 3] input:}" "combined output"="3\n"
Dec 22 08:50:15.064 INF Success command="{command:[bash -c echo 4 ; sleep 4] input:}" "combined output"="4\n"
Dec 22 08:50:18.067 WRN Failure command="{command:[bash -c echo 5 ; sleep 5] input:}" "combined output"="5\n" error="signal: killed"
Dec 22 08:50:20.001 INF Queued: 0; In progress: 2; Succeeded: 4; Failed: 1; Aborted: 0; Total: 7; Estimated time remaining: 3 seconds
Dec 22 08:50:20.065 WRN Failure command="{command:[bash -c echo 6 ; sleep 6] input:}" "combined output"="6\n" error="signal: killed"
Dec 22 08:50:23.070 WRN Failure command="{command:[bash -c echo 7 ; sleep 7] input:}" "combined output"="7\n" error="signal: killed"
Dec 22 08:50:23.070 INF Queued: 0; In progress: 0; Succeeded: 4; Failed: 3; Aborted: 0; Total: 7; Elapsed time: 14s
```

Cancelling (e.g. with CTRL-C) while running will stop any further jobs from being started, and will exit
when all currently-running jobs have completed.
Pressing CTRL-C a second time will send SIGTERM to all running jobs.
A third CTRL-C will send SIGKILL to all remaining running jobs.
A fourth and final CTRL-C will send SIGKILL to all remaining running jobs, as well as other processes in
their process groups.

```bash
$ seq 80 | dispatch --concurrency 5 --defer-reruns  -- bash -c 'trap noop SIGTERM ; sleep {{.value}}'
Dec 22 08:50:40.001 INF Queued: 75; In progress: 5; Succeeded: 0; Failed: 0; Aborted: 0; Total: 80; Elapsed time: 1s
Dec 22 08:50:40.498 INF Success command="{command:[bash -c trap noop SIGTERM ; sleep 1] input:}" "combined output"=""
^CDec 22 08:50:40.934 WRN received cancellation signal. Waiting for current jobs to finish before exiting. Hit CTRL-C again to exit sooner
Dec 22 08:50:40.934 INF Queued: 0; In progress: 5; Succeeded: 1; Failed: 0; Aborted: 0; Total: 6; Estimated time remaining: 1 seconds
Dec 22 08:50:41.498 INF Success command="{command:[bash -c trap noop SIGTERM ; sleep 2] input:}" "combined output"=""
Dec 22 08:50:42.000 INF Queued: 0; In progress: 4; Succeeded: 2; Failed: 0; Aborted: 0; Total: 6; Elapsed time: 3s
^CDec 22 08:50:42.222 WRN second CTRL-C received. Sending SIGTERM to running jobs. Hit CTRL-C again to use SIGKILL instead
Dec 22 08:50:42.499 INF Success command="{command:[bash -c trap noop SIGTERM ; sleep 3] input:}" "combined output"="bash: line 1: noop: command not found\n"
^CDec 22 08:50:42.948 WRN third CTRL-C received. Sending SIGKILL to running jobs. Hit CTRL-C again to kill all subprocesses too
Dec 22 08:50:43.001 INF Queued: 0; In progress: 3; Succeeded: 3; Failed: 0; Aborted: 0; Total: 6; Elapsed time: 4s
Dec 22 08:50:43.497 WRN Failure command="{command:[bash -c trap noop SIGTERM ; sleep 4] input:}" "combined output"="" error="signal: killed"
^CDec 22 08:50:43.712 WRN fourth CTRL-C received. Sending SIGKILL to running jobs and their subprocesses
Dec 22 08:50:43.712 WRN Failure command="{command:[bash -c trap noop SIGTERM ; sleep 6] input:}" "combined output"="" error="signal: killed"
Dec 22 08:50:43.712 WRN Failure command="{command:[bash -c trap noop SIGTERM ; sleep 5] input:}" "combined output"="" error="signal: killed"
Dec 22 08:50:43.713 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 3; Aborted: 0; Total: 6; Estimated time remaining: 1 seconds
Dec 22 08:50:43.713 ERR user-initiated shutdown

```

If you want to stop processing if a job fails, use `--abort-on-error`:

```bash
$ seq 1 10 \
    | dispatch --abort-on-error --concurrency 2 --timeout 5s -- bash -c 'echo {{.value}} ; sleep {{.value}}'
Dec 22 08:51:40.001 INF Queued: 8; In progress: 2; Succeeded: 0; Failed: 0; Aborted: 0; Total: 10; Elapsed time: 1s
Dec 22 08:51:40.253 INF Success command="{command:[bash -c echo 1 ; sleep 1] input:}" "combined output"="1\n"
Dec 22 08:51:41.253 INF Success command="{command:[bash -c echo 2 ; sleep 2] input:}" "combined output"="2\n"
Dec 22 08:51:43.258 INF Success command="{command:[bash -c echo 3 ; sleep 3] input:}" "combined output"="3\n"
Dec 22 08:51:45.258 INF Success command="{command:[bash -c echo 4 ; sleep 4] input:}" "combined output"="4\n"
Dec 22 08:51:48.261 WRN Failure command="{command:[bash -c echo 5 ; sleep 5] input:}" "combined output"="5\n" error="signal: killed"
Dec 22 08:51:49.000 INF Queued: 4; In progress: 1; Succeeded: 4; Failed: 1; Aborted: 0; Total: 10; Estimated time remaining: 15 seconds
Dec 22 08:51:50.259 WRN Failure command="{command:[bash -c echo 6 ; sleep 6] input:}" "combined output"="6\n" error="signal: killed"
Dec 22 08:51:50.260 INF Queued: 4; In progress: 0; Succeeded: 4; Failed: 2; Aborted: 0; Total: 10; Estimated time remaining: 15 seconds
Dec 22 08:51:50.260 ERR nonzero exit code
```

### Simulating STDIN

If each job expects input from STDIN, this can be supplied with `--input` (similar to the `yes` command).
Note that the input text can be the same for each job, or can be parameterised using the same inputs as the command itself:

```bash
$ echo -e 'animal,name,emotion\ncat,Scarface Claw,hungry' \
    | dispatch --input '{{.emotion}}' --csv -- /bin/bash -c 'read emotion; echo the {{.animal}} is called {{.name}} and is $emotion'
Dec 22 08:52:17.013 INF Success command="{command:[/bin/bash -c read emotion; echo the cat is called Scarface Claw and is $emotion] input:hungry}" "combined output"="the cat is called Scarface Claw and is hungry\n"
Dec 22 08:52:17.014 INF Queued: 0; In progress: 0; Succeeded: 1; Failed: 0; Aborted: 0; Total: 1; Elapsed time: 0s

```

### Caching results

By default, `~/.cache/dispatch` is used to store the STDOUT/STDERR of each job, along with whether it succeeded.
An alternative location can be provided using `--cache-location`.

#### S3 caching

It is possible to use a S3 bucket to cache the results: `--cache-location s3://my-bucket/my-prefix`

As long as you have valid AWS environment variables/credentials, this should "just work". You may also need to ensure that the `AWS_REGION` environment variable is set correctly.
Note that metadata (filename, last-modified time) for all assets in the S3 bucket under the nominated prefix will be read each time the application is run.
For more than a few thousand records, this may take a few seconds. This data is stored in a temporary sqlite database,
which is deleted when the process exits.

If an error is detected while writing to the S3 bucket, this will stop subsequent jobs from running. The most likely cause is your AWS credentials have expired.
