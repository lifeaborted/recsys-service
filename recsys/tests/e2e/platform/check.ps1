#!/usr/bin/env pwsh
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Ensure Go toolchain is discoverable in non-interactive PowerShell sessions.
$scoopShims = Join-Path $HOME "scoop\shims"
if ((Test-Path $scoopShims) -and ($env:PATH -notlike "*$scoopShims*")) {
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
$pgUser = "e2e_user_$runId"
$pgPass = "e2e_pass_$runId"
$pgDb = "recsys_e2e_$runId"
$pgTxDb = "recsys_tx_$runId"

# Scrub any ambient DB env vars so we NEVER touch a working/production database.
foreach ($name in @("DATABASE_URL", "RECSYS_DATABASE_URL", "RECSYS_ENV_FILE", "HOST", "PORT", "USER", "PASSWORD", "DATABASE", "DATADASE", "SSLMODE")) {
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

    # Build and launch cmd/api against the seeded ephemeral DB.
    $apiExe = Join-Path $tempBinDir "recsys-api.exe"
    go build -o $apiExe ./cmd/api
    if ($LASTEXITCODE -ne 0) { throw "Failed to build cmd/api" }

    $env:DATABASE_URL = $e2eDsn
    $env:RECSYS_ADDR = "127.0.0.1:$apiPort"
    $apiProcess = Start-Process -FilePath $apiExe -WorkingDirectory $recsysRoot -PassThru -WindowStyle Hidden

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

    # Verify Swagger UI HTML and OpenAPI YAML served from the same OpenAPI document.
    $swUi = Invoke-WebRequest -Uri "$baseUrl/swagger/" -Method GET -SkipHttpErrorCheck
    if ($swUi.StatusCode -ne 200 -or ([string]$swUi.Content) -notmatch "/swagger/openapi.yaml") {
        throw "Swagger UI HTML check failed (status=$($swUi.StatusCode))"
    }
    $sw = Invoke-WebRequest -Uri "$baseUrl/swagger/openapi.yaml" -Method GET -SkipHttpErrorCheck
    $swText = if ($sw.Content -is [byte[]]) { [System.Text.Encoding]::UTF8.GetString($sw.Content) } else { [string]$sw.Content }
    if ($sw.StatusCode -ne 200 -or $swText -notmatch "operationId: getCurrentRecommendations") {
        throw "Swagger OpenAPI spec check failed (status=$($sw.StatusCode))"
    }

    # Verify 401 without token + X-Request-ID on error.
    $unauth = Invoke-WebRequest -Uri "$baseUrl/api/v1/recommendations/me" -Method GET -Headers @{"X-Request-ID" = "e2e-unauth-1"} -SkipHttpErrorCheck
    if ($unauth.StatusCode -ne 401 -or ($unauth.Headers["X-Request-ID"] -join "") -ne "e2e-unauth-1") {
        throw "Expected 401 with echoed X-Request-ID, got status=$($unauth.StatusCode)"
    }

    # Login two isolated profiles: demo-b2b and demo-b2c.
    $login1 = Invoke-RestMethod -Uri "$baseUrl/api/v1/auth/login" -Method POST -ContentType "application/json" -Body '{"login":"demo-b2b","password":"e2e-b2b-password"}'
    $login2 = Invoke-RestMethod -Uri "$baseUrl/api/v1/auth/login" -Method POST -ContentType "application/json" -Body '{"login":"demo-b2c","password":"e2e-b2c-password"}'
    $hdr1 = @{ Authorization = "Bearer $($login1.access_token)" }
    $hdr2 = @{ Authorization = "Bearer $($login2.access_token)" }

    $me1 = Invoke-RestMethod -Uri "$baseUrl/api/v1/users/me" -Method GET -Headers $hdr1
    $me2 = Invoke-RestMethod -Uri "$baseUrl/api/v1/users/me" -Method GET -Headers $hdr2
    if ($me1.profile_id -eq $me2.profile_id -or $me1.client_group -ne "B2B" -or $me2.client_group -ne "B2C") {
        throw "Unexpected profile metadata: me1=$($me1 | ConvertTo-Json -Compress) me2=$($me2 | ConvertTo-Json -Compress)"
    }

    # Verify 400 on profile spoofing in /purchases.
    $spoof = Invoke-WebRequest -Uri "$baseUrl/api/v1/purchases" -Method POST -Headers $hdr1 -ContentType "application/json" -Body '{"item_id":"DEMO-BASE","profile_id":"u2"}' -SkipHttpErrorCheck
    if ($spoof.StatusCode -ne 400) {
        throw "Expected 400 on profile override attempt, got $($spoof.StatusCode)"
    }

    # Profile 1 buys DEMO-BASE -> sees DEMO-RENEW-12 with 12/24 variants -> buys DEMO-RENEW-12 -> DEMO-RENEW-12 disappears.
    $buyBase = Invoke-WebRequest -Uri "$baseUrl/api/v1/purchases" -Method POST -Headers $hdr1 -ContentType "application/json" -Body '{"item_id":"DEMO-BASE"}' -SkipHttpErrorCheck
    if ($buyBase.StatusCode -ne 201) { throw "Purchase DEMO-BASE failed: $($buyBase.StatusCode) $($buyBase.Content)" }

    $rec1Before = Invoke-RestMethod -Uri "$baseUrl/api/v1/recommendations/me" -Method GET -Headers $hdr1
    $renewCards = @($rec1Before.items | Where-Object { $_.item_id -eq "DEMO-RENEW-12" })
    if ($renewCards.Count -eq 0 -or @($renewCards[0].variants).Count -lt 2) {
        throw "Expected DEMO-RENEW-12 renewal card with 12/24 variants after DEMO-BASE purchase, got: $($rec1Before | ConvertTo-Json -Depth 6)"
    }

    $buyRenew12 = Invoke-WebRequest -Uri "$baseUrl/api/v1/purchases" -Method POST -Headers $hdr1 -ContentType "application/json" -Body '{"item_id":"DEMO-RENEW-12"}' -SkipHttpErrorCheck
    if ($buyRenew12.StatusCode -ne 201) { throw "Purchase DEMO-RENEW-12 failed: $($buyRenew12.StatusCode)" }

    $rec1After = Invoke-RestMethod -Uri "$baseUrl/api/v1/recommendations/me" -Method GET -Headers $hdr1
    if (@($rec1After.items | Where-Object { $_.item_id -eq "DEMO-RENEW-12" }).Count -gt 0) {
        throw "Purchased item DEMO-RENEW-12 did not disappear from Profile 1 recommendations!"
    }

    # Verify Profile 2 remained completely isolated in PostgreSQL and HTTP.
    $rec2 = Invoke-RestMethod -Uri "$baseUrl/api/v1/recommendations/me" -Method GET -Headers $hdr2
    $p2Purchases = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc "SELECT count(*) FROM recommendations.online_purchases WHERE client_id = 'DEMO-NEW';").Trim()
    $p1Purchases = (docker compose -f $composeFile -p $projectName exec -T postgres psql -U $pgUser -d $pgDb -tAc "SELECT count(*) FROM recommendations.online_purchases WHERE client_id = 'DEMO-BUYER';").Trim()
    if ($p1Purchases -ne "2" -or $p2Purchases -ne "0") {
        throw "Profile purchase isolation failed: DEMO-BUYER=$p1Purchases DEMO-NEW=$p2Purchases"
    }

    Write-Host "[check] SUCCESS: PG17 migrations, 2x seed idempotency, pgx transactions, and two-profile isolation verified (DEMO-BUYER purchases=$p1Purchases, DEMO-NEW purchases=$p2Purchases, B2C items=$(@($rec2.items).Count))."
} finally {
    if ($null -ne $apiProcess -and -not $apiProcess.HasExited) {
        Stop-Process -Id $apiProcess.Id -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $tempBinDir) {
        Remove-Item -Recurse -Force $tempBinDir -ErrorAction SilentlyContinue
    }
    Write-Host "[check] Cleaning up isolated Compose project $projectName..."
    docker compose -f $composeFile -p $projectName down -v --remove-orphans 2>$null | Out-Null
    Pop-Location -ErrorAction SilentlyContinue
}
