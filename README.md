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
      --concurrency=            run this many jobs in dispatch (default: 1)
      --dry-run                 simulate what would be run
      --input=                  send the input string (plus newline) forever as STDIN to each job
      --rate-limit=             prevent jobs starting more than this often
      --rate-limit-bucket-size= allow a burst of up to this many jobs when enforcing the rate limit
      --timeout=                cancel each job after this much time

output:
      --debug                   show more detailed log messages
      --hide-failures           do not display a message each time a job fails
      --hide-successes          do not display a message each time a job succeeds
      --show-stderr             do not suppress each job's STDERR
      --show-stdout             do not suppress each job's STDOUT
```

## Examples

#### Basic operation

Run three variations of `echo`, substituting `{{.value}}` with each input line in turn

```bash
$ echo -e 'one\ntwo\nthree' \
    | dispatch -- echo {{.value}}
Jan 18 11:05:56.641 INF Success elapsed="3 milliseconds" command="{command:[echo one] input:}" "output ID"=b25ca1783749afbf505d.zstd
Jan 18 11:05:56.642 INF Success elapsed="1 milliseconds" command="{command:[echo two] input:}" "output ID"=504748d73ba659fbbfef.zstd
Jan 18 11:05:56.643 INF Success elapsed="1 milliseconds" command="{command:[echo three] input:}" "output ID"=094fe78ba1fdf4664bf3.zstd
Jan 18 11:05:56.643 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3; Estimated time remaining: 0 milliseconds
```

The stdout and stderr are combined and stored (compressed using zstd) in `~/.cache/dispatch/{success,failure}/*`

If you want a copy of stderr and stdout to be shown:

```bash
$ echo -e 'one\ntwo\nthree' \
    | dispatch --show-stdout --show-stderr -- echo {{.value}}
one
Jan 18 11:06:13.798 INF Success elapsed="1 milliseconds" command="{command:[echo one] input:}" "output ID"=b25ca1783749afbf505d.zstd
two
Jan 18 11:06:13.799 INF Success elapsed="1 milliseconds" command="{command:[echo two] input:}" "output ID"=504748d73ba659fbbfef.zstd
three
Jan 18 11:06:13.800 INF Success elapsed="1 milliseconds" command="{command:[echo three] input:}" "output ID"=094fe78ba1fdf4664bf3.zstd
Jan 18 11:06:13.801 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3; Estimated time remaining: 0 milliseconds
```

#### JSON parsing

Parse each input line as a JSON object (also suppressing the "Success" log entries:

```bash
$ echo -e '{"animal": "cat", "name": "Scarface Claw"}\n{"animal": "dog", "name": "Bitzer Maloney"}' \
    | dispatch --json-line --hide-successes --show-stdout -- echo the {{.animal}} is called {{.name}}
the cat is called Scarface Claw
the dog is called Bitzer Maloney
Jan 18 10:46:26.424 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Estimated time remaining: 0 milliseconds
```

#### CSV parsing

```bash
$ echo -e 'animal,name\ncat,Scarface Claw\ndog,Bitzer Maloney' \
    | dispatch --csv --hide-successes --show-stdout -- echo the {{.animal}} is called {{.name}}
the cat is called Scarface Claw
the dog is called Bitzer Maloney
Jan 18 10:47:53.144 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Estimated time remaining: 0 milliseconds
```

#### Status logging

Every 10 seconds an interim status is generated, as well as at completion. An estimate of the remaining time will be shown,
based solely on the rate of completion of earlier jobs.
Duplicate status messages, where nothing has changed, will be suppressed for up to a minute.

```bash
$ seq 1 10 \
    | dispatch --concurrency 4 -- bash -c 'echo {{.value}} ; sleep 4'
Jan 18 11:07:20.001 INF Queued: 6; In progress: 4; Succeeded: 0; Failed: 0; Aborted: 0; Total: 10; Elapsed time: 2s
Jan 18 11:07:22.214 INF Success elapsed="4 seconds" command="{command:[bash -c echo 1 ; sleep 4] input:}" "output ID"=6df52a0cdc6eca57435d.zstd
Jan 18 11:07:22.215 INF Success elapsed="4 seconds" command="{command:[bash -c echo 2 ; sleep 4] input:}" "output ID"=4b36322bcedff1c28a1f.zstd
Jan 18 11:07:22.215 INF Success elapsed="4 seconds" command="{command:[bash -c echo 4 ; sleep 4] input:}" "output ID"=f0a6d3d22d07a6b90dd3.zstd
Jan 18 11:07:22.215 INF Success elapsed="4 seconds" command="{command:[bash -c echo 3 ; sleep 4] input:}" "output ID"=2530f16eae5544001d06.zstd
Jan 18 11:07:26.222 INF Success elapsed="4 seconds" command="{command:[bash -c echo 5 ; sleep 4] input:}" "output ID"=cb4190288a646098d531.zstd
Jan 18 11:07:26.222 INF Success elapsed="4 seconds" command="{command:[bash -c echo 6 ; sleep 4] input:}" "output ID"=408273f1513e41602871.zstd
Jan 18 11:07:26.222 INF Success elapsed="4 seconds" command="{command:[bash -c echo 7 ; sleep 4] input:}" "output ID"=df799aad2e32c063d4a6.zstd
Jan 18 11:07:26.222 INF Success elapsed="4 seconds" command="{command:[bash -c echo 8 ; sleep 4] input:}" "output ID"=9ce5ac37743d948a9860.zstd
Jan 18 11:07:30.003 INF Queued: 0; In progress: 2; Succeeded: 8; Failed: 0; Aborted: 0; Total: 10; Estimated time remaining: 229 milliseconds
Jan 18 11:07:30.228 INF Success elapsed="4 seconds" command="{command:[bash -c echo 10 ; sleep 4] input:}" "output ID"=242a72a9cb9c900a778c.zstd
Jan 18 11:07:30.228 INF Success elapsed="4 seconds" command="{command:[bash -c echo 9 ; sleep 4] input:}" "output ID"=6d306a99edd97b9a2901.zstd
Jan 18 11:07:30.228 INF Queued: 0; In progress: 0; Succeeded: 10; Failed: 0; Aborted: 0; Total: 10; Estimated time remaining: 0 milliseconds
```

#### Skipping previously-run jobs

If a job has already been attempted, and should not be re-attempted, use `--skip-successes` and/or `--skip-failures` as applicable:

```bash
$ seq 2 | dispatch --skip-successes
Jan 18 11:07:55.960 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:07:55.961 INF Success elapsed="1 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:07:55.962 INF Success elapsed="1 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:07:55.962 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Estimated time remaining: 0 milliseconds


$ seq 5 | dispatch --skip-successes
Jan 18 11:08:03.514 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:08:03.517 INF Success elapsed="3 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:08:03.520 INF Success elapsed="2 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:08:03.523 INF Success elapsed="3 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:08:03.523 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3 (+2 skipped); Estimated time remaining: 0 milliseconds
```

Notice the `skipped` value in the stats line.

#### Debounce period

If you only want to skip jobs which haven't succeeded/failed recently, you can provide a debounce period using `--debounce-successes` and/or `--debounce-failures`.
Be aware that this period is assessed when the STDIN record is parsed, not when the job is about to start.

Below, 2 jobs are run, then 3 more 10 seconds later. With a debounce of 10s, this means the third execution skips the 3 recent jobs:

```bash
$ seq 2 | dispatch --skip-successes ; sleep 10; seq 5 | dispatch --skip-successes ; seq 5 | dispatch --skip-successes --debounce-successes 10s
Jan 18 11:09:19.781 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:09:19.783 INF Success elapsed="1 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:09:19.783 INF Success elapsed="1 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:09:19.783 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Estimated time remaining: 0 milliseconds

Jan 18 11:09:29.793 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:09:29.794 INF Success elapsed="1 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:09:29.794 INF Success elapsed="0 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:09:29.796 INF Success elapsed="1 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:09:29.796 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3 (+2 skipped); Estimated time remaining: 0 milliseconds

Jan 18 11:09:29.798 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:09:29.799 INF Success elapsed="1 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:09:29.800 INF Success elapsed="0 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:09:29.800 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2 (+3 skipped); Estimated time remaining: 0 milliseconds

```

#### Deprioritising recently-run jobs

By default, jobs are started in the order they are provided via STDIN.
If desired `--defer-reruns` will notice if a job has been run previously (whether successful or not), and will run other jobs first.
Where multiple jobs are reruns, priority is given to least recently-run jobs.
Where jobs have never been run before, the order provided in STDIN is respected.

```bash
$ seq 5 | dispatch --concurrency=5 ; seq 10 | dispatch --defer-reruns --concurrency=5
Jan 18 11:08:55.177 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:08:55.178 INF Success elapsed="1 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:08:55.179 INF Success elapsed="2 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:08:55.179 INF Success elapsed="2 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:08:55.179 INF Success elapsed="2 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:08:55.179 INF Success elapsed="2 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:08:55.179 INF Queued: 0; In progress: 0; Succeeded: 5; Failed: 0; Aborted: 0; Total: 5; Estimated time remaining: 0 milliseconds

Jan 18 11:08:55.182 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:08:55.288 INF Success elapsed="5 milliseconds" command="{command:[echo value is 9] input:}" "output ID"=baaf0a889c28102b4bab.zstd
Jan 18 11:08:55.288 INF Success elapsed="5 milliseconds" command="{command:[echo value is 6] input:}" "output ID"=3dbd18cb1f87cd44dd8d.zstd
Jan 18 11:08:55.288 INF Success elapsed="5 milliseconds" command="{command:[echo value is 7] input:}" "output ID"=6f97f0389902ee7d6f79.zstd
Jan 18 11:08:55.288 INF Success elapsed="5 milliseconds" command="{command:[echo value is 10] input:}" "output ID"=7ffd061c809b4388d48e.zstd
Jan 18 11:08:55.289 INF Success elapsed="6 milliseconds" command="{command:[echo value is 8] input:}" "output ID"=92d3f33311a0049f63e0.zstd
Jan 18 11:08:55.294 INF Success elapsed="5 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:08:55.294 INF Success elapsed="6 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:08:55.296 INF Success elapsed="7 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:08:55.296 INF Success elapsed="7 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:08:55.296 INF Success elapsed="8 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:08:55.296 INF Queued: 0; In progress: 0; Succeeded: 10; Failed: 0; Aborted: 0; Total: 10; Estimated time remaining: 0 milliseconds

```

To make `--defer-reruns` more effective, a small delay is introduced before jobs start being executed.
During this period, jobs are collected and sorted, making it more likely that the right jobs will be run first.
`--defer-delay` can override the length of this delay, which defaults to 100ms.

#### Suppressing success and/or failure messages

If you want a less noisy output, you can suppress success and/or failure messages. STDOUT and STDERR are still logged to
the filesystem as normal:

```bash
$ seq 1 254 | dispatch --hide-failures --concurrency 100 --timeout 10s -- nc -vz 192.168.4.{{.value}} 443
Jan 18 11:10:28.215 INF Success elapsed="41 milliseconds" command="{command:[nc -vz 192.168.4.53 443] input:}" "output ID"=2765d1e6ba31d75fb28e.zstd
Jan 18 11:10:30.000 INF Queued: 138; In progress: 100; Succeeded: 1; Failed: 15; Aborted: 0; Total: 254; Estimated time remaining: 1324 milliseconds
Jan 18 11:10:34.277 INF Success elapsed="13 milliseconds" command="{command:[nc -vz 192.168.4.222 443] input:}" "output ID"=1e522d01bc6e471afad2.zstd
Jan 18 11:10:37.528 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 252; Aborted: 0; Total: 254; Estimated time remaining: 0 milliseconds
```

### Rate limiting

Sometimes, despite wanting to run jobs concurrently, you want to place a limit on the maximum rate jobs can be started at. For example, you might want to run 4 jobs at a time, but wait 2 seconds between them:

```bash
$ seq 1 5 | dispatch --rate-limit 2s --concurrency 4
Jan 18 11:11:10.853 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:11:10.854 INF Success elapsed="1 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:11:12.857 INF Success elapsed="3 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:11:14.860 INF Success elapsed="4 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:11:16.857 INF Success elapsed="3 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:11:18.859 INF Success elapsed="4 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:11:18.859 INF Queued: 0; In progress: 0; Succeeded: 5; Failed: 0; Aborted: 0; Total: 5; Estimated time remaining: 0 milliseconds
```

If bursting is acceptable, `--rate-limit-bucket-size` allows this.

For example, if you want to issue some API commands, ensuring no more than 1 is started per second, with a burst of 3 (but allowing 4 to run concurrently):

```bash
$ seq 1 5 | dispatch --rate-limit 1s --concurrency 4 --rate-limit-bucket-size 3
Jan 18 11:11:22.642 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:11:22.645 INF Success elapsed="2 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:11:22.645 INF Success elapsed="2 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:11:22.645 INF Success elapsed="2 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:11:23.645 INF Success elapsed="2 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:11:24.645 INF Success elapsed="2 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:11:24.646 INF Queued: 0; In progress: 0; Succeeded: 5; Failed: 0; Aborted: 0; Total: 5; Estimated time remaining: 0 milliseconds
```

### Dry-run

Want to ensure the right command will be run with the correct inputs? `--dry-run` will do this. Nothing will actually be executed.
An implicit 1 second sleep will be substituted for the actual execution of each command:

```bash
$ seq 8 | dispatch --dry-run --debounce-successes 5s --concurrency 1 --input y -- rm -f foo.{{.value}}
Jan 18 11:12:01.720 INF Success elapsed="1001 milliseconds" command="{command:[rm -f foo.1] input:y}" "output ID"=ab8b937c790098be3e55.zstd
Jan 18 11:12:02.721 INF Success elapsed="1001 milliseconds" command="{command:[rm -f foo.2] input:y}" "output ID"=d2643bb44be06524dbd7.zstd
Jan 18 11:12:03.725 INF Success elapsed="1004 milliseconds" command="{command:[rm -f foo.3] input:y}" "output ID"=5b49f64411d226dc7bb4.zstd
Jan 18 11:12:04.728 INF Success elapsed="1002 milliseconds" command="{command:[rm -f foo.4] input:y}" "output ID"=1ea25bc93b7b65a186fd.zstd
Jan 18 11:12:05.730 INF Success elapsed="1002 milliseconds" command="{command:[rm -f foo.5] input:y}" "output ID"=36838f3c883e7673f880.zstd
Jan 18 11:12:06.732 INF Success elapsed="1002 milliseconds" command="{command:[rm -f foo.6] input:y}" "output ID"=31961c03ba1beddf9958.zstd
Jan 18 11:12:07.733 INF Success elapsed="1001 milliseconds" command="{command:[rm -f foo.7] input:y}" "output ID"=d44dbd954201e40a421c.zstd
Jan 18 11:12:08.734 INF Success elapsed="1001 milliseconds" command="{command:[rm -f foo.8] input:y}" "output ID"=2a5955872922219720dd.zstd
Jan 18 11:12:08.734 INF Queued: 0; In progress: 0; Succeeded: 8; Failed: 0; Aborted: 0; Total: 8; Estimated time remaining: 0 milliseconds
```

### Shuffle / randomise

Usually, if you want to run the jobs in a random order, you can pipe STDIN via `shuf` beforehand.
However, if the source of jobs is dynamic, you might not want to wait until all jobs are generated before
any jobs are started.

`--shuffle` will disregard the order in which jobs were received, but will work as expected with respect to
`--defer-reruns`. This means your jobs will start being processed without delay, and reruns will still be
run only after new jobs, but the new jobs will be run in a random order. (The rerun jobs are not randomised,
as they are selected based on the time the job was last attempted.)

```bash
$ seq 5 | dispatch --shuffle --defer-reruns
Jan 18 11:58:21.142 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:58:21.247 INF Success elapsed="3 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:58:21.249 INF Success elapsed="2 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:58:21.252 INF Success elapsed="3 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:58:21.252 INF Success elapsed="1 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:58:21.253 INF Success elapsed="1 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:58:21.253 INF Queued: 0; In progress: 0; Succeeded: 5; Failed: 0; Aborted: 0; Total: 5; Estimated time remaining: 0 milliseconds

$ seq 10 | dispatch --shuffle --defer-reruns
Jan 18 11:58:23.879 INF no command was provided, so just echoing the input commandline="[echo value is {{.value}}]"
Jan 18 11:58:23.984 INF Success elapsed="4 milliseconds" command="{command:[echo value is 7] input:}" "output ID"=6f97f0389902ee7d6f79.zstd
Jan 18 11:58:23.986 INF Success elapsed="2 milliseconds" command="{command:[echo value is 10] input:}" "output ID"=7ffd061c809b4388d48e.zstd
Jan 18 11:58:23.990 INF Success elapsed="4 milliseconds" command="{command:[echo value is 8] input:}" "output ID"=92d3f33311a0049f63e0.zstd
Jan 18 11:58:23.993 INF Success elapsed="3 milliseconds" command="{command:[echo value is 6] input:}" "output ID"=3dbd18cb1f87cd44dd8d.zstd
Jan 18 11:58:23.996 INF Success elapsed="3 milliseconds" command="{command:[echo value is 9] input:}" "output ID"=baaf0a889c28102b4bab.zstd
Jan 18 11:58:23.998 INF Success elapsed="2 milliseconds" command="{command:[echo value is 3] input:}" "output ID"=99bdbf20bfc04b6eb4e1.zstd
Jan 18 11:58:24.000 INF Success elapsed="1 milliseconds" command="{command:[echo value is 5] input:}" "output ID"=f8927c64f1d75b4bcae8.zstd
Jan 18 11:58:24.001 INF Success elapsed="1 milliseconds" command="{command:[echo value is 1] input:}" "output ID"=9bfdb2668ac9919e0db1.zstd
Jan 18 11:58:24.002 INF Success elapsed="1 milliseconds" command="{command:[echo value is 4] input:}" "output ID"=63bab6284a47dd147568.zstd
Jan 18 11:58:24.004 INF Success elapsed="2 milliseconds" command="{command:[echo value is 2] input:}" "output ID"=a2cc2f4538d74fba8b2e.zstd
Jan 18 11:58:24.004 INF Queued: 0; In progress: 0; Succeeded: 10; Failed: 0; Aborted: 0; Total: 10; Estimated time remaining: 0 milliseconds

```

### Job cancellations and timeouts

Defining a timeout will cause jobs to be terminated if it is reached:

```bash
$ seq 1 7 \
    | dispatch --concurrency 2 --timeout 5s -- bash -c 'echo {{.value}} ; sleep {{.value}}'
Jan 18 11:04:15.373 INF Success elapsed="1006 milliseconds" command="{command:[bash -c echo 1 ; sleep 1] input:}" "output ID"=d7fd3b289a22aff57047.zstd
Jan 18 11:04:16.374 INF Success elapsed="2 seconds" command="{command:[bash -c echo 2 ; sleep 2] input:}" "output ID"=bc0d5aced16de4e0816e.zstd
Jan 18 11:04:18.380 INF Success elapsed="3 seconds" command="{command:[bash -c echo 3 ; sleep 3] input:}" "output ID"=60284b3ef8cdc80af521.zstd
Jan 18 11:04:20.000 INF Queued: 2; In progress: 2; Succeeded: 3; Failed: 0; Aborted: 0; Total: 7; Estimated time remaining: 5 seconds
Jan 18 11:04:20.380 INF Success elapsed="4 seconds" command="{command:[bash -c echo 4 ; sleep 4] input:}" "output ID"=f0a6d3d22d07a6b90dd3.zstd
Jan 18 11:04:23.384 WRN Failure elapsed="5 seconds" command="{command:[bash -c echo 5 ; sleep 5] input:}" "output ID"=1d1e837e38a186ee4220.zstd error="signal: killed"
Jan 18 11:04:25.383 WRN Failure elapsed="5 seconds" command="{command:[bash -c echo 6 ; sleep 6] input:}" "output ID"=4bd1d8a81ac68fdfdf23.zstd error="signal: killed"
Jan 18 11:04:28.387 WRN Failure elapsed="5 seconds" command="{command:[bash -c echo 7 ; sleep 7] input:}" "output ID"=b883233d5c600d37eaec.zstd error="signal: killed"
Jan 18 11:04:28.387 INF Queued: 0; In progress: 0; Succeeded: 4; Failed: 3; Aborted: 0; Total: 7; Estimated time remaining: 0 milliseconds

```

Cancelling (e.g. with CTRL-C) while running will stop any further jobs from being started, and will exit
when all currently-running jobs have completed.
Pressing CTRL-C a second time will send SIGTERM to all running jobs.
A third CTRL-C will send SIGKILL to all remaining running jobs.
A fourth and final CTRL-C will send SIGKILL to all remaining running jobs, as well as other processes in
their process groups.

```bash
$ seq 80 | dispatch --concurrency 5 --defer-reruns  -- bash -c 'trap noop SIGTERM ; sleep {{.value}}'
Jan 18 11:59:26.046 INF Success elapsed="1010 milliseconds" command="{command:[bash -c trap noop SIGTERM ; sleep 1] input:}" "output ID"=a7c02935cb86ae82293b.zstd
Jan 18 11:59:27.046 INF Success elapsed="2 seconds" command="{command:[bash -c trap noop SIGTERM ; sleep 2] input:}" "output ID"=e5d121137c2996e5ed41.zstd
^CJan 18 11:59:27.766 INF Queued: 0; In progress: 5; Succeeded: 2; Failed: 0; Aborted: 0; Total: 7; Estimated time remaining: 2 seconds
Jan 18 11:59:27.766 WRN received cancellation signal. Waiting for current jobs to finish before exiting. Hit CTRL-C again to exit sooner
Jan 18 11:59:28.000 INF Queued: 0; In progress: 5; Succeeded: 2; Failed: 0; Aborted: 0; Total: 7; Estimated time remaining: 1776 milliseconds
Jan 18 11:59:28.046 INF Success elapsed="3 seconds" command="{command:[bash -c trap noop SIGTERM ; sleep 3] input:}" "output ID"=ce8670d72f9c21552622.zstd
Jan 18 11:59:29.001 INF Queued: 0; In progress: 4; Succeeded: 3; Failed: 0; Aborted: 0; Total: 7; Estimated time remaining: 1775 milliseconds
^CJan 18 11:59:29.016 WRN second CTRL-C received. Sending SIGTERM to running jobs. Hit CTRL-C again to use SIGKILL instead
Jan 18 11:59:29.048 INF Success elapsed="4 seconds" command="{command:[bash -c trap noop SIGTERM ; sleep 4] input:}" "output ID"=7a01a3a137b433eb128a.zstd
Jan 18 11:59:30.001 INF Queued: 0; In progress: 3; Succeeded: 4; Failed: 0; Aborted: 0; Total: 7; Estimated time remaining: 1777 milliseconds
Jan 18 11:59:30.047 INF Success elapsed="5 seconds" command="{command:[bash -c trap noop SIGTERM ; sleep 5] input:}" "output ID"=f4a21cd7471f7456fde9.zstd
^CJan 18 11:59:30.386 WRN third CTRL-C received. Sending SIGKILL to running jobs. Hit CTRL-C again to kill all subprocesses too
Jan 18 11:59:31.001 INF Queued: 0; In progress: 2; Succeeded: 5; Failed: 0; Aborted: 0; Total: 7; Estimated time remaining: 1776 milliseconds
Jan 18 11:59:32.051 WRN Failure elapsed="6 seconds" command="{command:[bash -c trap noop SIGTERM ; sleep 6] input:}" "output ID"=eabbe4398cf197bdbefb.zstd error="signal: killed"
Jan 18 11:59:33.001 INF Queued: 0; In progress: 1; Succeeded: 5; Failed: 1; Aborted: 0; Total: 7; Estimated time remaining: -58 milliseconds
Jan 18 11:59:34.050 WRN Failure elapsed="7 seconds" command="{command:[bash -c trap noop SIGTERM ; sleep 7] input:}" "output ID"=86a6bb7ada19415fae7b.zstd error="signal: killed"
Jan 18 11:59:34.050 INF Queued: 0; In progress: 0; Succeeded: 5; Failed: 2; Aborted: 0; Total: 7; Estimated time remaining: 0 milliseconds
```

If you want to stop processing if a job fails, use `--abort-on-error`:

```bash
$ seq 1 10 \
    | dispatch --abort-on-error --concurrency 2 --timeout 5s -- bash -c 'echo {{.value}} ; sleep {{.value}}'
Jan 18 12:00:15.993 INF Success elapsed="1003 milliseconds" command="{command:[bash -c echo 1 ; sleep 1] input:}" "output ID"=d7fd3b289a22aff57047.zstd
Jan 18 12:00:16.993 INF Success elapsed="2 seconds" command="{command:[bash -c echo 2 ; sleep 2] input:}" "output ID"=bc0d5aced16de4e0816e.zstd
Jan 18 12:00:18.998 INF Success elapsed="3 seconds" command="{command:[bash -c echo 3 ; sleep 3] input:}" "output ID"=60284b3ef8cdc80af521.zstd
Jan 18 12:00:20.001 INF Queued: 5; In progress: 2; Succeeded: 3; Failed: 0; Aborted: 0; Total: 10; Estimated time remaining: 8 seconds
Jan 18 12:00:20.997 INF Success elapsed="4 seconds" command="{command:[bash -c echo 4 ; sleep 4] input:}" "output ID"=f0a6d3d22d07a6b90dd3.zstd
Jan 18 12:00:24.002 WRN Failure elapsed="5 seconds" command="{command:[bash -c echo 5 ; sleep 5] input:}" "output ID"=1d1e837e38a186ee4220.zstd error="signal: killed"
Jan 18 12:00:25.000 INF Queued: 4; In progress: 1; Succeeded: 4; Failed: 1; Aborted: 0; Total: 10; Estimated time remaining: 10 seconds
Jan 18 12:00:25.999 WRN Failure elapsed="5 seconds" command="{command:[bash -c echo 6 ; sleep 6] input:}" "output ID"=4bd1d8a81ac68fdfdf23.zstd error="signal: killed"
Jan 18 12:00:25.999 INF Queued: 4; In progress: 0; Succeeded: 4; Failed: 2; Aborted: 0; Total: 10; Estimated time remaining: 11 seconds
Jan 18 12:00:25.999 ERR nonzero exit code
```

### Simulating STDIN

If each job expects input from STDIN, this can be supplied with `--input` (similar to the `yes` command).
Note that the input text can be the same for each job, or can be parameterised using the same inputs as the command itself:

```bash
$ echo -e 'animal,name,emotion\ncat,Scarface Claw,hungry' \
    | dispatch --show-stdout --hide-successes --input '{{.emotion}}' --csv -- /bin/bash -c 'read emotion; echo the {{.animal}} is called {{.name}} and is $emotion'
the cat is called Scarface Claw and is hungry
Jan 18 12:02:08.154 INF Queued: 0; In progress: 0; Succeeded: 1; Failed: 0; Aborted: 0; Total: 1; Estimated time remaining: 0 milliseconds

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
