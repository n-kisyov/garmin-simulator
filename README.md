# Garmin FIT Simulator (`fitsim`)

`fitsim` is a CLI tool written in Go that simulates various Garmin activity `.fit` files. It was rewritten from an original collection of Python scripts into a unified Go application using `cobra` and `github.com/tormoder/fit`.

## Supported Activities

The simulator supports creating the following Garmin activities:
- `run`
- `cycle`
- `walk`
- `hike`
- `swim`
- `cardio`
- `strength`
- `yoga`
- `meditation`
- `ski` (Alpine & Cross-Country)
- `row` (Indoor & Outdoor)
- `ebike`
- `field` (Basketball, Soccer, Tennis)

## Building

You can build the project using standard Go tools or the provided PowerShell script:

```powershell
.\build.ps1
```

## Usage

Use the `--help` flag for any command to see the required parameters.

**Example: Simulate a 10km/h run along a KML route**
```bash
.\fitsim.exe run --datetime "25-08-26 12:00:00" --speed 10.0 --kml route.kml --file run.fit
```

**Example: Simulate a 45-minute strength training session**
```bash
.\fitsim.exe strength --datetime "25-08-26 12:00:00" --sets 5 --reps 12 --file strength.fit
```

### Generating several files at once

Every activity accepts `--count`, which writes that many FIT files instead of
one. The files are numbered, and each starts one minute later than the one
before it; nothing else about them differs.

```bash
.\fitsim.exe run --datetime "25-08-26 08:00:00" --speed 10.0 --kml route.kml --file run.fit --count 3
```

writes `run1.fit` starting at 08:00:00, `run2.fit` at 08:01:00 and `run3.fit` at
08:02:00. Leaving `--count` out (or setting it to 1) writes a single file under
exactly the name given, as before. Interactive mode (`.\fitsim.exe` with no
arguments) asks for the same thing.

## Web interface and API

`fitsimweb.exe` serves the browser UI on <http://localhost:8088> and the endpoint
behind it. The UI has a **Files** box next to the start time; set it above 1 to
get a series.

`POST /api/simulate` takes a multipart form with an `activity` field (`run`,
`cycle`, ...), the flags that activity needs (`datetime`, `speed`, `duration`,
`distance`, `reps`, `sets`, `sport`, `type`), an optional `kml_file` upload and
an optional `count`:

- `count` absent or `1` — the response is the `.fit` file itself.
- `count` between 2 and 100 — the response is a zip named after the activity,
  holding `run1.fit`, `run2.fit`, ... one minute apart.

```bash
curl -OJ -F "activity=run" -F "speed=10.0" -F "datetime=25-08-26 08:00:00" \
     -F "count=3" -F "kml_file=@route.kml" http://localhost:8088/api/simulate
```
