param (
    [switch]$RunAfterBuild = $false,
    [switch]$Clean = $false
)

$exeName = "fitsim.exe"

if ($Clean) {
    Write-Host "Cleaning..."
    if (Test-Path $exeName) {
        Remove-Item $exeName -Force
        Write-Host "Removed $exeName"
    }
}

Write-Host "Building $exeName..."
go build -o $exeName
$buildResult = $LASTEXITCODE

if ($buildResult -eq 0) {
    Write-Host "Build successful!" -ForegroundColor Green
    
    if ($RunAfterBuild) {
        Write-Host "Running fitsim.exe -h..."
        .\$exeName -h
    }
} else {
    Write-Host "Build failed with exit code $buildResult." -ForegroundColor Red
}
