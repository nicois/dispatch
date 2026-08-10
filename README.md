# Dispatch

[![Go Reference](https://pkg.go.dev/badge/github.com/nicois/dispatch.svg)](https://pkg.go.dev/github.com/nicois/dispatch)

Run many variations of a command. Control the number of concurrent jobs, the retries and more.

## Features

- Runs many jobs at the same time.
- Records the STDOUT and the STDERR of each job. This applies to successful jobs and to failed jobs.
- Reads job arguments in three formats: one value per line, one JSON object per line, or CSV.
- Shows a summary of the progress on the console.
- Increases the force of each job termination when you press CTRL-C again.

These functions are optional:

- Skips a job that ran before, or gives it a lower priority. The time of the last successful run controls this behaviour.
- Stops a job after a timeout.
- Stops the run if a job fails.
- Sends the same text to the STDIN of each job.
- Keeps the state in an S3 bucket, or in a compatible object store.

## Installation

```bash
go install github.com/nicois/dispatch/dispatch@latest
```

Go puts the program in the `~/go/bin/` directory.

## Usage

```
Application Options:
      --version                 show the version and exit

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

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Each job was successful, or was skipped. |
| 1 | Dispatch could not start the run. The options were not valid, the cache was not available, or the user stopped the run. |
| 2 | Dispatch ran correctly, but a minimum of one job failed. |

Thus your shell script can find a failed job. The script does not need to read the log messages:

```bash
$ seq 3 | dispatch -- false ; echo "exit code: $?"
...
exit code: 2
```

`--dry-run` does not run a command. Therefore it always gives the code 0.

## Examples

#### Basic operation

This example runs three variations of `echo`. Dispatch replaces `{{.value}}` with each input line.

```bash
$ echo -e 'one\ntwo\nthree' \
    | dispatch -- echo {{.value}}
Jan 18 11:05:56.641 INF Success elapsed="3 milliseconds" command="{command:[echo one] input:}" "output ID"=b25ca1783749afbf505d.zstd
Jan 18 11:05:56.642 INF Success elapsed="1 milliseconds" command="{command:[echo two] input:}" "output ID"=504748d73ba659fbbfef.zstd
Jan 18 11:05:56.643 INF Success elapsed="1 milliseconds" command="{command:[echo three] input:}" "output ID"=094fe78ba1fdf4664bf3.zstd
Jan 18 11:05:56.643 INF Queued: 0; In progress: 0; Succeeded: 3; Failed: 0; Aborted: 0; Total: 3; Estimated time remaining: 0 milliseconds
```

Dispatch puts the STDOUT and the STDERR together in one file. It compresses the file with zstd.
It keeps the file in the `~/.cache/dispatch/success/` directory or the `~/.cache/dispatch/failure/` directory.

To also show the STDOUT and the STDERR on the console, use these options:

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

Dispatch can read each input line as a JSON object. This example also hides the `Success` messages:

```bash
$ echo -e '{"animal": "cat", "name": "Scarface Claw"}\n{"animal": "dog", "name": "Bitzer Maloney"}' \
    | dispatch --json-line --hide-successes --show-stdout -- echo the {{.animal}} is called {{.name}}
the cat is called Scarface Claw
the dog is called Bitzer Maloney
Jan 18 10:46:26.424 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Estimated time remaining: 0 milliseconds
```

If a line is not valid JSON, dispatch shows a warning and skips that line. Thus one bad line does not stop the run.

#### CSV parsing

```bash
$ echo -e 'animal,name\ncat,Scarface Claw\ndog,Bitzer Maloney' \
    | dispatch --csv --hide-successes --show-stdout -- echo the {{.animal}} is called {{.name}}
the cat is called Scarface Claw
the dog is called Bitzer Maloney
Jan 18 10:47:53.144 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 0; Aborted: 0; Total: 2; Estimated time remaining: 0 milliseconds
```

A row can have a different number of columns than the header. Dispatch shows a warning, but it runs the job.
If the row is too short, the absent columns become empty text. If the row is too long, dispatch ignores the
additional columns.

#### Status messages

Dispatch shows the status each 10 seconds, and again at the end of the run. The status includes an estimate of
the remaining time. The estimate uses only the durations of the jobs that are complete.
If the status did not change, dispatch does not show it again for one minute.

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

#### How to skip jobs that ran before

A job can run one time only. To prevent a second run, use `--skip-successes`, or `--skip-failures`, or both options:

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

The status line shows the number of skipped jobs.

#### Debounce period

A debounce period makes dispatch skip only the recent jobs. Use `--debounce-successes`, or `--debounce-failures`, or both options.
Dispatch calculates this period when it reads the line from STDIN. It does not calculate the period at the start of the job.

This example runs 2 jobs, then 3 more jobs 10 seconds later. The debounce period is 10 seconds.
Therefore the third command skips the 3 recent jobs:

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

#### How to give recent jobs a lower priority

Dispatch starts the jobs in the sequence that it reads them from STDIN.
`--defer-reruns` changes this behaviour. Dispatch finds each job that ran before, and starts the other jobs first.
The result of the earlier run is not important.
If more than one job ran before, the job with the oldest run starts first.
The jobs that did not run before keep their sequence from STDIN.

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

`--defer-reruns` includes a short delay before the first job starts.
In this period, dispatch collects the jobs and sorts them. Thus it is more probable that the correct jobs start first.
The default delay is 100 milliseconds. `--defer-delay` changes this value.

#### How to hide the success and failure messages

To make the output shorter, hide the success messages, or the failure messages, or both.
Dispatch continues to write the STDOUT and the STDERR to the cache:

```bash
$ seq 1 254 | dispatch --hide-failures --concurrency 100 --timeout 10s -- nc -vz 192.168.4.{{.value}} 443
Jan 18 11:10:28.215 INF Success elapsed="41 milliseconds" command="{command:[nc -vz 192.168.4.53 443] input:}" "output ID"=2765d1e6ba31d75fb28e.zstd
Jan 18 11:10:30.000 INF Queued: 138; In progress: 100; Succeeded: 1; Failed: 15; Aborted: 0; Total: 254; Estimated time remaining: 1324 milliseconds
Jan 18 11:10:34.277 INF Success elapsed="13 milliseconds" command="{command:[nc -vz 192.168.4.222 443] input:}" "output ID"=1e522d01bc6e471afad2.zstd
Jan 18 11:10:37.528 INF Queued: 0; In progress: 0; Succeeded: 2; Failed: 252; Aborted: 0; Total: 254; Estimated time remaining: 0 milliseconds
```

### How to limit the rate

You can run jobs at the same time, and also limit how frequently a new job starts.
In this example, 4 jobs run at the same time, but each new job waits 2 seconds:

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

`--rate-limit-bucket-size` permits a group of jobs to start together.

This example sends API commands. One command starts each second, but a group of 3 can start together.
A maximum of 4 commands run at the same time:

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

`--dry-run` shows you the commands and their inputs. Dispatch does not run a command.
In place of each command, dispatch waits 1 second:

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

### How to use a random sequence

To run the jobs in a random sequence, you can usually send STDIN through `shuf` first.
But sometimes another program makes the jobs slowly. Then you do not want to wait for all of the jobs.

`--shuffle` ignores the sequence of the jobs from STDIN. It also operates correctly with `--defer-reruns`.
Thus the first job starts immediately, and the new jobs run in a random sequence.
The jobs that ran before continue to run last. Their sequence stays the same, because the time of the last run controls it.

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

If you set a timeout, dispatch stops each job that runs for longer than this period:

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

Press CTRL-C to stop the run. Dispatch does not start a new job. It stops when the current jobs are complete.
Press CTRL-C a second time. Dispatch sends SIGTERM to each job that runs.
Press CTRL-C a third time. Dispatch sends SIGKILL to each job that runs.
Press CTRL-C a fourth time. Dispatch sends SIGKILL to each job that runs, and to the other processes in the
process group of the job.

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

To stop the run when a job fails, use `--abort-on-error`:

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

### How to send text to STDIN

A job can read text from STDIN. Use `--input` to send this text. The behaviour is similar to the `yes` command.
The text can be the same for each job. The text can also include the same variables as the command:

```bash
$ echo -e 'animal,name,emotion\ncat,Scarface Claw,hungry' \
    | dispatch --show-stdout --hide-successes --input '{{.emotion}}' --csv -- /bin/bash -c 'read emotion; echo the {{.animal}} is called {{.name}} and is $emotion'
the cat is called Scarface Claw and is hungry
Jan 18 12:02:08.154 INF Queued: 0; In progress: 0; Succeeded: 1; Failed: 0; Aborted: 0; Total: 1; Estimated time remaining: 0 milliseconds

```

### How to keep the results

Dispatch keeps the STDOUT and the STDERR of each job in the `~/.cache/dispatch` directory. It also keeps the
result of the job. To use a different directory, use `--cache-location`.

#### How to keep the results in S3

Dispatch can keep the results in an S3 bucket: `--cache-location s3://my-bucket/my-prefix`

You must have valid AWS credentials. Set the `AWS_REGION` environment variable to the correct value.

At the start of each run, dispatch reads the metadata of each object below the prefix. The metadata includes
the name of the object and the time of the last change. If there are more than a few thousand objects, this
operation can take a few seconds. Dispatch keeps this data in a temporary SQLite database. It deletes the
database at the end of the run.

If dispatch cannot write to the S3 bucket, it stops the run. Usually, the cause is expired AWS credentials.

If the `AWS_EXPIRY_TIME` environment variable holds an RFC 3339 time, dispatch stops 5 minutes before that
time. Thus the current jobs can be complete, and dispatch can record their results.

### Periods of time

Five options take a period of time: `--timeout`, `--rate-limit`, `--debounce-successes`, `--debounce-failures`
and `--defer-delay`. Each of these options accepts the usual Go units, for example `500ms`, `90s`, `1m30s` or `2h`.
Each option also accepts a number of days at the start of the value, for example `3d` or `1d12h`.

## How to use dispatch as a library

A Go program can use the same functions:

```go
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/nicois/dispatch"
)

func main() {
	cache, err := dispatch.NewFileCache("/tmp/my-cache")
	if err != nil {
		log.Fatal(err)
	}

	var opts dispatch.Opts
	opts.Concurrency = 4

	err = dispatch.PrepareAndRun(context.Background(), strings.NewReader("1\n2\n3\n"),
		opts, []string{"echo", "value is {{.value}}"}, cache, make(chan os.Signal, 1))
	if errors.Is(err, dispatch.ErrJobsFailed) {
		log.Println("some jobs failed:", err)
	} else if err != nil {
		log.Fatal(err)
	}
}
```

`PrepareAndRunWithStats` also returns the statistics of the run. These statistics are correct even if the
function returns an error.

To make the jobs in your program, and not read them from a stream, use `NewRenderedCommand`. Use `WithInput`
to add text for the STDIN of the job. Then send the jobs to `Run`.

`SetLogger` selects the destination of the log messages. If you do not call `SetLogger`, dispatch does not
write log messages.

`PrepareAndRun` and `Run` stop each of their goroutines before they return. Therefore a program that runs for a
long time can call these functions many times.
