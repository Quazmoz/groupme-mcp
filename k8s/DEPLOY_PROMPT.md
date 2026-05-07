# Kubernetes Deployment Prompt for GroupMe MCP Server

Use the following prompt to instruct an AI or DevOps engineer to deploy the server to your cluster.

---

**Prompt:**

Please generate Kubernetes manifests to deploy my GroupMe MCP server to the cluster.

**1. Configuration Details:**
*   **Image**: `quazmoz/quazmoz:latest` (This is a **private** Docker Hub repository).
*   **Port**: `5000`
*   **Transport Mode**: `http` (Required for OpenWebUI compatibility).
...
        *   `MCP_TRANSPORT`: `"http"` (Critical!)
**2. Requirements:**

*   **Secrets Management**:
    *   Create a Docker Registry Secret (named `regcred`) to allow K8s to pull from my private repo.
    *   Create an Opaque Secret (named `groupme-mcp-secret`) to securely store my `GROUPME_ACCESS_TOKEN`.

*   **Deployment Manifest**:
    *   Use the image `quazmoz/quazmoz:latest`.
    *   Reference the `regcred` in `imagePullSecrets`.
    *   Set the following Environment Variables:
        *   `MCP_TRANSPORT`: `"http"` (Critical!)
        *   `PORT`: `"5000"`
        *   `LOG_LEVEL`: `"INFO"`
        *   `GROUPME_ACCESS_TOKEN`: Map this from the `groupme-mcp-secret`.
    *   Configure Liveness and Readiness probes pointing to the `/health` endpoint on port 5000.

*   **Service Manifest**:
    *   Create a `ClusterIP` service named `groupme-mcp-server` exposing port 80 (mapping to container port 5000).

**3. Output Request:**
Please provide the YAML files (`deployment.yaml`, `service.yaml`) and the `kubectl` commands needed to generate the secrets.
