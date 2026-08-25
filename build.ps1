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
$buildResult1 = $LASTEXITCODE

Write-Host "Building fitsimweb.exe..."
go build -o fitsimweb.exe ./cmd/fitsimweb
$buildResult2 = $LASTEXITCODE

if ($buildResult1 -eq 0 -and $buildResult2 -eq 0) {
    Write-Host "Build successful!" -ForegroundColor Green
    
    if ($RunAfterBuild) {
        Write-Host "Running fitsim.exe -h..."
        .\$exeName -h
    }
} else {
    Write-Host "Build failed. fitsim exit code: $buildResult1, fitsimweb exit code: $buildResult2" -ForegroundColor Red
}
