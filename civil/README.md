# Civil Time

[![Go Reference](https://pkg.go.dev/badge/cloud.google.com/go/civil.svg)](https://pkg.go.dev/cloud.google.com/go/civil)

Go package for civil time: a time-zone-independent representation of time that
follows the rules of the proleptic Gregorian calendar with exactly 24-hour days,
60-minute hours, and 60-second minutes.

## Install

```bash
go get cloud.google.com/go/civil
```

## Stability

The stability of this module is indicated by SemVer.

## Example

```go
import (
    "fmt"
    "time"

    "cloud.google.com/go/civil"
)

// Working with dates
d := civil.Date{Year: 2024, Month: time.March, Day: 15}
fmt.Println(d)                    // 2024-03-15
fmt.Println(d.AddDays(10))        // 2024-03-25
fmt.Println(d.Weekday())          // Friday

parsed, _ := civil.ParseDate("2024-03-15")
fmt.Println(parsed == d)          // true

today := civil.DateOf(time.Now())
fmt.Println(today.After(d))       // depends on current date

// Working with times
t := civil.Time{Hour: 14, Minute: 30, Second: 0}
fmt.Println(t)                    // 14:30:00

// Working with datetimes
dt := civil.DateTime{Date: d, Time: t}
fmt.Println(dt)                   // 2024-03-15T14:30:00

// Convert to time.Time in a specific location
loc, _ := time.LoadLocation("Europe/Berlin")
tt := dt.In(loc)
fmt.Println(tt)                   // 2024-03-15 14:30:00 +0100 CET
```

## Go Version Support

See the [Go Versions Supported](https://github.com/googleapis/google-cloud-go#go-versions-supported)
section in the root directory's README.

## Contributing

Contributions are welcome. Please, see the [CONTRIBUTING](https://github.com/GoogleCloudPlatform/google-cloud-go/blob/main/CONTRIBUTING.md)
document for details.

Please note that this project is released with a Contributor Code of Conduct.
By participating in this project you agree to abide by its terms. See
[Contributor Code of Conduct](https://github.com/GoogleCloudPlatform/google-cloud-go/blob/main/CONTRIBUTING.md#contributor-code-of-conduct)
for more information.
