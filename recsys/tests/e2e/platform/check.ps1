#!/usr/bin/env pwsh
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Ensure Go toolchain is discoverable in non-interactive PowerShell sessions.
$scoopShims = Join-Path $HOME "scoop\shims"
if ($IsWindows -and (Test-Path $scoopShims) -and ($env:PATH -notlike "*$scoopShims*")) {
    $env:PATH = "$scoopShims;$env:PATH"
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try {
        return ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$recsysRoot = (Resolve-Path (Join-Path $scriptDir "..\..\..")).Path
$composeFile = Join-Path $scriptDir "docker-compose.yml"

$runId = [Guid]::NewGuid().ToString("N").Substring(0, 8)
$projectName = "recsys-platform-e2e-$runId"
$pgPort = Get-FreeTcpPort
$apiPort = Get-FreeTcpPort
$grpcPort = Get-FreeTcpPort
$pgUser = "e2e_user_$runId"
$pgPass = "e2e_pass_$runId"
$pgDb = "recsys_e2e_$runId"
$pgTxDb = "recsys_tx_$runId"

# Scrub any ambient DB env vars so we NEVER touch a working/production database.
foreach ($name in @(
    "DATABASE_URL", "RECSYS_DATABASE_URL", "RECSYS_ENV_FILE", "HOST", "PORT", "USER", "PASSWORD", "DATABASE", "DATADASE", "SSLMODE",
    "RECSYS_GRPC_CERT_FILE", "RECSYS_GRPC_KEY_FILE", "RECSYS_GRPC_CA_FILE", "RECSYS_GRPC_ALLOW_INSECURE", "LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL"
)) {
    Remove-Item "Env:$name" -ErrorAction SilentlyContinue
}

$env:E2E_PG_PORT = "$pgPort"
$env:E2E_PG_USER = $pgUser
$env:E2E_PG_PASSWORD = $pgPass
$env:E2E_PG_DB = $pgDb

Write-Host "[check] Verifying Docker & Docker Compose v2..."
docker compose version | Out-Host
if ($LASTEXITCODE -ne 0) { throw "docker compose v2 is required" }
docker info --format '{{.ServerVersion}}' | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Docker daemon is not running or unreachable" }

$apiProcess = $null
$recommendProcess = $null
$tempBinDir = Join-Path ([System.IO.Path]::GetTempPath()) "recsys-e2e-bin-$runId"
New-Item -ItemType Directory -Path $tempBinDir -Force | Out-Null

try {
    Push-Location $recsysRoot
    Write-Host "[check] Starting isolated PostgreSQL 17 container (project=$projectName, port=$pgPort)..."
    docker compose -f $composeFile -p $projectName up -d --wait
    if ($LASTEXITCODE -ne 0) { throw "Failed to start temporary PG17 container" }

    $pgVersionNum = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc "SHOW server_version_num;").Trim()
    if ([int]$pgVersionNum -lt 170000 -or [int]$pgVersionNum -ge 180000) {
        throw "Expected PostgreSQL 17, got server_version_num=$pgVersionNum"
    }
    Write-Host "[check] PostgreSQL 17 confirmed (server_version_num=$pgVersionNum)"

    $e2eDsn = "postgres://${pgUser}:${pgPass}@127.0.0.1:${pgPort}/${pgDb}?sslmode=disable"
    $env:DATABASE_URL = $e2eDsn
    $env:RECSYS_BCRYPT_COST = "4"
    $env:RECSYS_MOCK = "0"
    $env:DEMO_B2B_LOGIN = "demo-b2b"
    $env:DEMO_B2B_PASSWORD = "e2e-b2b-password"
    $env:DEMO_B2C_LOGIN = "demo-b2c"
    $env:DEMO_B2C_PASSWORD = "e2e-b2c-password"

    $countQuery = @"
SELECT json_build_object(
    'clients', (SELECT count(*) FROM recommendations.clients),
    'stand_accounts', (SELECT count(*) FROM recommendations.stand_accounts),
    'account_users', (SELECT count(*) FROM recommendations.account_users),
    'account_memberships', (SELECT count(*) FROM recommendations.account_memberships),
    'catalog_offers', (SELECT count(*) FROM recommendations.catalog_offers),
    'catalog_version_offers', (SELECT count(*) FROM recommendations.catalog_version_offers),
    'historical_items', (SELECT count(*) FROM recommendations.historical_items),
    'model_snapshots', (SELECT count(*) FROM recommendations.model_snapshots),
    'snapshot_candidates', (SELECT count(*) FROM recommendations.snapshot_candidates),
    'dataset_release', (SELECT count(*) FROM recommendations.dataset_release)
)::text;
"@

    Write-Host "[check] Pass 1: running migrations and seed..."
    go run ./cmd/seed -migrate
    if ($LASTEXITCODE -ne 0) { throw "Seed pass 1 failed" }
    $countsPass1 = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc $countQuery).Trim()
    Write-Host "[check] Pass 1 counts: $countsPass1"

    Write-Host "[check] Pass 2: re-running migrations and seed (idempotency check)..."
    go run ./cmd/seed -migrate
    if ($LASTEXITCODE -ne 0) { throw "Seed pass 2 failed" }
    $countsPass2 = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc $countQuery).Trim()
    Write-Host "[check] Pass 2 counts: $countsPass2"

    if ($countsPass1 -ne $countsPass2) {
        throw "Seed is not idempotent! Pass 1 ($countsPass1) != Pass 2 ($countsPass2)"
    }

    $extCount = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc "SELECT count(*) FROM pg_extension WHERE extname IN ('pg_trgm', 'vector');").Trim()
    if ($extCount -ne "2") {
        throw "Expected both pg_trgm and vector extensions enabled in PG17, found count=$extCount"
    }
    Write-Host "[check] Extensions pg_trgm and vector verified"

    # Run pgx transactional & contract integration tests on a separate DB inside the same ephemeral PG17 container.
    docker compose -f $composeFile -p $projectName exec -T postgres createdb -U $pgUser $pgTxDb
    if ($LASTEXITCODE -ne 0) { throw "Failed to create $pgTxDb" }
    $env:RECSYS_TEST_DATABASE_URL = "postgres://${pgUser}:${pgPass}@127.0.0.1:${pgPort}/${pgTxDb}?sslmode=disable"
    Write-Host "[check] Running Go PostgreSQL adapter & transaction tests..."
    go test -count=1 -v ./internal/store/postgres/...
    if ($LASTEXITCODE -ne 0) { throw "PostgreSQL adapter integration tests failed" }
    Remove-Item Env:RECSYS_TEST_DATABASE_URL -ErrorAction SilentlyContinue

    # Launch the recommendation service against the seeded ephemeral DB.
    $env:DATABASE_URL = $e2eDsn
    $env:RECSYS_GRPC_ADDR = "127.0.0.1:$grpcPort"
    $env:RECSYS_GRPC_TARGET = $env:RECSYS_GRPC_ADDR
    $recommendExe = Join-Path $tempBinDir "recsys-recommend.exe"
    go build -o $recommendExe ./cmd/recommend
    if ($LASTEXITCODE -ne 0) { throw "Failed to build cmd/recommend" }

    $recommendStart = @{ FilePath = $recommendExe; WorkingDirectory = $recsysRoot; PassThru = $true }
    if ($IsWindows) { $recommendStart.WindowStyle = "Hidden" }
    $recommendProcess = Start-Process @recommendStart
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        if ($recommendProcess.HasExited) { throw "Recommendation service stopped during startup" }
        $probe = [System.Net.Sockets.TcpClient]::new()
        try {
            $probe.Connect("127.0.0.1", $grpcPort)
            $ready = $true
            break
        } catch [System.Net.Sockets.SocketException] {
            Start-Sleep -Milliseconds 300
        } finally {
            $probe.Dispose()
        }
    }
    if (-not $ready) { throw "Recommendation service did not start on $env:RECSYS_GRPC_ADDR" }

    # Launch the HTTP API with the gRPC service as its recommendation dependency.
    $apiExe = Join-Path $tempBinDir "recsys-api.exe"
    go build -o $apiExe ./cmd/api
    if ($LASTEXITCODE -ne 0) { throw "Failed to build cmd/api" }

    $env:DATABASE_URL = $e2eDsn
    $env:RECSYS_ADDR = "127.0.0.1:$apiPort"
    $start = @{ FilePath = $apiExe; WorkingDirectory = $recsysRoot; PassThru = $true }
    if ($IsWindows) { $start.WindowStyle = "Hidden" }
    $apiProcess = Start-Process @start

    $baseUrl = "http://127.0.0.1:$apiPort"
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $h = Invoke-WebRequest -Uri "$baseUrl/api/v1/health" -Method GET -SkipHttpErrorCheck
            if ($h.StatusCode -eq 200) {
                $ready = $true
                break
            }
        } catch {}
        Start-Sleep -Milliseconds 300
    }
    if (-not $ready) { throw "API did not become healthy on $baseUrl" }
    Write-Host "[check] API healthy on $baseUrl (X-Request-ID=$($h.Headers['X-Request-ID']))"

    $env:RECSYS_E2E_API = $baseUrl
    node --test tests/e2e/demo/api.test.mjs
    if ($LASTEXITCODE -ne 0) { throw "HTTP integration failed" }
    $p2Purchases = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc "SELECT count(*) FROM recommendations.online_purchases WHERE client_id = 'DEMO-NEW';").Trim()
    if ($p2Purchases -ne "0") { throw "Second profile recorded unexpected purchases" }
    Write-Host "[check] SUCCESS: PG17 extensions, two seed passes, transactions and HTTP → gRPC verified (snapshot=demo-v1)."

} finally {
    if ($null -ne $apiProcess -and -not $apiProcess.HasExited) {
        Stop-Process -Id $apiProcess.Id -Force -ErrorAction SilentlyContinue
    }
    if ($null -ne $recommendProcess -and -not $recommendProcess.HasExited) {
        Stop-Process -Id $recommendProcess.Id -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $tempBinDir) {
        $resolved = (Resolve-Path -LiteralPath $tempBinDir).Path
        $expected = [System.IO.Path]::GetFullPath($tempBinDir)
        if ($resolved -eq $expected -and (Split-Path -Leaf $resolved) -eq "recsys-e2e-bin-$runId") {
            Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    Write-Host "[check] Cleaning up isolated Compose project $projectName..."
    docker compose -f $composeFile -p $projectName down -v --remove-orphans 2>$null | Out-Null
    Pop-Location -ErrorAction SilentlyContinue
}
