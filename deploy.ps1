$ErrorActionPreference = "Stop"

Write-Host "🚀 Starting Deployment to DockerHub..." -ForegroundColor Green

# Check if Docker is running
docker info > $null 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Docker is not running. Please start Docker Desktop and try again." -ForegroundColor Red
    exit 1
}

$IMAGE_NAME = "quazmoz/quazmoz:groupme"

# Build
Write-Host "📦 Building Docker image: $IMAGE_NAME" -ForegroundColor Cyan
docker build -t $IMAGE_NAME .
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Build failed!" -ForegroundColor Red
    exit 1
}

# Push
Write-Host "⬆️ Pushing to DockerHub..." -ForegroundColor Cyan
docker push $IMAGE_NAME
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Push failed! Make sure you are logged in with 'docker login'." -ForegroundColor Red
    exit 1
}

Write-Host "✅ Successfully deployed $IMAGE_NAME to DockerHub!" -ForegroundColor Green
