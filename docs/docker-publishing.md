# Docker Publishing

Docker Hub repository: `quazmoz/groupme-mcp`

## Tag Strategy

- `dev`: development branch and test builds
- `prod`: stable production image
- `latest`: same image as `prod`
- `vX.Y.Z`: immutable release tag
- short Git SHA: traceability tag for the exact source revision

## Manual Publish

PowerShell:

```powershell
./scripts/docker-build.ps1 -Tag dev
./scripts/docker-push.ps1 -Tag dev -Build

./scripts/docker-build.ps1 -Tag prod -AdditionalTags latest
./scripts/docker-push.ps1 -Tag prod -AdditionalTags latest -Build

./scripts/docker-build.ps1 -Tag v0.1.0 -AdditionalTags prod,latest
./scripts/docker-push.ps1 -Tag v0.1.0 -AdditionalTags prod,latest -Build
```

Bash:

```bash
TAG=dev ./scripts/docker-build.sh
BUILD=true TAG=dev ./scripts/docker-push.sh

TAG=prod ADDITIONAL_TAGS=latest ./scripts/docker-build.sh
BUILD=true TAG=prod ADDITIONAL_TAGS=latest ./scripts/docker-push.sh

TAG=v0.1.0 ADDITIONAL_TAGS=prod,latest ./scripts/docker-build.sh
BUILD=true TAG=v0.1.0 ADDITIONAL_TAGS=prod,latest ./scripts/docker-push.sh
```

## Docker Hub Access Token

1. Open Docker Hub.
2. Go to `Account Settings > Personal access tokens`.
3. Create a token with write/push permissions for `quazmoz/groupme-mcp`.
4. Use the token only when the publish script prompts for it.

## Security Notes

- Never commit Docker Hub tokens.
- Never put Docker Hub PATs in `.env`.
- Prefer a Docker Hub PAT over your account password.

## Pull And Run

```bash
docker pull quazmoz/groupme-mcp:prod
```

```bash
docker run --rm -i \
  -e MCP_TRANSPORT=stdio \
  -e GROUPME_ACCESS_TOKEN=YOUR_ACCESS_TOKEN \
  quazmoz/groupme-mcp:prod
```

```bash
docker run --rm -p 5000:5000 \
  -e MCP_TRANSPORT=http \
  -e GROUPME_ACCESS_TOKEN=YOUR_ACCESS_TOKEN \
  quazmoz/groupme-mcp:prod
```
