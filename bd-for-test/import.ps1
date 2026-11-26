param(
  [string]$HostName = 'localhost',
  [int]$Port = 5432,
  [string]$User = 'postgres',
  [string]$Database = 'recsys',
  [string]$Password = '',
  [string]$File = (Join-Path $PSScriptRoot 'recommendations.sql')
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command psql -ErrorAction SilentlyContinue)) {
  throw 'psql не найден. Нужен клиент PostgreSQL 17.x (в дампе есть \restrict / \unrestrict).'
}
if (-not (Test-Path $File)) { throw "Файл дампа не найден: $File" }

if ($Password) { $env:PGPASSWORD = $Password }
elseif (-not $env:PGPASSWORD) {
  $secure = Read-Host 'Пароль PostgreSQL' -AsSecureString
  $env:PGPASSWORD = [System.Net.NetworkCredential]::new('', $secure).Password
}

Write-Host "Импорт $File в $Database ..."
& psql -h $HostName -p $Port -U $User -d $Database -v ON_ERROR_STOP=1 -f $File
if ($LASTEXITCODE -ne 0) { throw "psql завершился с кодом $LASTEXITCODE" }
Write-Host 'Готово.'
