$ErrorActionPreference = "Stop"

$root = "D:\code\auto_inspection"
$python = "python"

$backend = Start-Process -FilePath $python -ArgumentList "backend_server.py","--host","127.0.0.1","--port","18080" -WorkingDirectory $root -PassThru
$mcp = Start-Process -FilePath $python -ArgumentList "auto_inspection_mcp.py","--host","127.0.0.1","--port","18081" -WorkingDirectory $root -PassThru

Write-Output "backend pid=$($backend.Id) url=http://127.0.0.1:18080"
Write-Output "mcp pid=$($mcp.Id) url=http://127.0.0.1:18081/mcp"
