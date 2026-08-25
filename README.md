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
