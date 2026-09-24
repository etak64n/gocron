# gocron

[![Go Reference](https://pkg.go.dev/badge/github.com/etak64n/gocron.svg)](https://pkg.go.dev/github.com/etak64n/gocron)
[![CI](https://github.com/etak64n/gocron/actions/workflows/ci.yml/badge.svg)](https://github.com/etak64n/gocron/actions/workflows/ci.yml)

gocron reads cron expressions and works out when they fire.
It works in any time zone, including on the days when daylight saving time starts or ends: a time that the clock skips still fires, and a time that the clock shows twice fires once.
It has no dependencies outside the Go standard library.

gocron computes times only and does not run jobs.
A program calls `Next` and waits for the time it returns with its own timer.

## Installation

```sh
go get github.com/etak64n/gocron
```

gocron needs Go 1.22 or later.

## Usage

```go
package main

import (
	"fmt"
	"time"

	"github.com/etak64n/gocron"
)

func main() {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		panic(err)
	}
	// 9:00 on weekdays, Tokyo time.
	s, err := gocron.ParseInLocation("0 9 * * mon-fri", tokyo)
	if err != nil {
		panic(err)
	}
	fmt.Println(s.Next(time.Now()))
}
```

`Parse` reads an expression in the local time zone.
`MustParse` panics on an invalid expression instead of returning an error, for variables that hold fixed schedules.
`Next` returns the zero time when an expression never fires, such as `0 0 30 2 *` for February 30.

## Expressions

An expression has five fields separated by spaces, or six with a leading seconds field:

| Field | Values | Names |
|---|---|---|
| Seconds (optional) | 0-59 | |
| Minutes | 0-59 | |
| Hours | 0-23 | |
| Day of month | 1-31 | |
| Month | 1-12 | `jan`-`dec` |
| Day of week | 0-6, where 0 is Sunday | `sun`-`sat` |

Without the seconds field, the times fall on second 0.
Names are not case-sensitive.

Each field is a comma-separated list of these terms:

| Term | Meaning |
|---|---|
| `5` | one value |
| `1-5` | a range |
| `*` or `?` | every value |
| `*/15` | every 15th value from the smallest |
| `10-40/10` | every 10th value in a range |
| `30/5` | every 5th value from 30 to the largest |

When both the day of month and the day of week are restricted, a day that matches either of them fires, as in the classic cron.
`0 12 1 * sun` fires at noon on the 1st of every month and on every Sunday.

A descriptor stands for a whole expression:

| Descriptor | Fires |
|---|---|
| `@yearly`, `@annually` | as `0 0 1 1 *` |
| `@monthly` | as `0 0 1 * *` |
| `@weekly` | as `0 0 * * 0` |
| `@daily`, `@midnight` | as `0 0 * * *` |
| `@hourly` | as `0 * * * *` |
| `@every 1h30m` | 1 hour 30 minutes after the time passed to `Next` |

`@every` takes a duration in the format of `time.ParseDuration`, cut to whole seconds with a minimum of one second.
Its times fall on whole seconds.

## Time zones

The times in an expression are readings of a clock.
`ParseInLocation` reads them in the given time zone, and `Parse` in the local one.
A `CRON_TZ=` or `TZ=` prefix sets the zone in the expression itself and takes precedence, as in `CRON_TZ=Europe/Berlin 30 2 * * *`.

`Next` returns times in the location of its argument.

gocron finds time zones with `time.LoadLocation`, which needs a time zone database on the system or in the program.
A program that may run where the system has none, such as on Windows, can embed the database with `import _ "time/tzdata"`.

## Daylight saving time

On the day daylight saving time starts, the clock skips some readings, such as 2:00 to 2:59 in New York.
A time that the clock skips fires at the moment it would have come with the UTC offset from before the change.
The clock shows that moment as a time after the gap:

| Expression | Fires in New York on March 8, 2026 |
|---|---|
| `30 2 * * *` | 3:30 |
| `*/20 2 * * *` | 3:00, 3:20 and 3:40 |

On the day daylight saving time ends, the clock shows some readings twice, such as 1:00 to 1:59 in New York.
A time that the clock shows twice fires once, at the first of the two moments:

| Expression | Fires in New York on November 1, 2026 |
|---|---|
| `30 1 * * *` | 1:30 EDT, and not again at 1:30 EST |

Zones that change the clock at midnight, by half an hour or by two hours follow the same rules.
The tests check 14 expressions around every clock change of 2026 in 11 zones, including such zones, against times worked out by brute force from the readings of the clock.

## License

[MIT](LICENSE)
