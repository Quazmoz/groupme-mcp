# GroupMe MCP Server Setup Guide

This guide details how to deploy and connect the GroupMe MCP Server to **OpenWebUI / Context Forge**.

## 🏗️ 1. Kubernetes Deployment (Infrastructure)

Before configuring the AI agent, deploy the server to your cluster.

1.  **Create Secrets**:
    Copy `k8s/secret.yaml` and fill in your values.
    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: <service-name>-secrets
      namespace: <namespace>
    type: Opaque
    stringData:
      # Required for Solution 1 (Manual/Private)
      groupme-access-token: "YOUR_ACCESS_TOKEN"
      
      # Required for Solution 2 (Multi-User)
      encryption-key: "YOUR_ENCRYPTION_KEY"
      jwt-secret: "YOUR_JWT_SECRET" # Match your OpenWebUI JWT secret
      redis-password: "YOUR_REDIS_PASSWORD"
    ```
    Apply it: `kubectl apply -f k8s/secret.yaml`

2.  **Deploy**:
    Apply the manifest:
    ```bash
    kubectl apply -f k8s/deployment.yaml
    kubectl apply -f k8s/service.yaml
    ```
    *Ensure `MCP_TRANSPORT` is set to `"http"` in your deployment.*

---

## 🔌 2. Client Configuration

The server can be consumed by different systems. Choose the instructions for your platform.

### Which one do I use?
*   **OpenWebUI**: Can connect **DIRECTLY** to this server. You do NOT need Context Forge unless you are using it for advanced management.
*   **Context Forge**: Use this if you want to route tools through a specific gateway or manage them centrally.

---

### A. OpenWebUI (Direct Connection)
Use this if you are connecting OpenWebUI **directly** to the GroupMe MCP Server pod.

**Method 1: Single User (Private)**
1.  Go to **Admin Panel > Settings > Connections**.
2.  Add a new **MCP Server**.
3.  **URL**: `http://<service-name>.<namespace>.svc.cluster.local:5000/mcp`
4.  **Headers**:
    *   `Authorization`: `Bearer YOUR_ACCESS_TOKEN`

**Method 2: Multi-User (Shared / Public)**
*   **Admin Action**: You add the server **ONCE** in Admin Settings.
*   **User Action**: Users log in via the **chat window** (not settings).

1.  **Admin**: Go to **Admin Panel > Settings > Connections**.
2.  Add a new **MCP Server**.
3.  **URL**: `http://<service-name>.<namespace>.svc.cluster.local:5000/mcp`
4.  **Headers**: (Leave Empty)
    *   *The server will see who is talking and handle their specific session.*

---

### B. Context Forge (Gateway / Management)
Use this if you are using **Context Forge** to manage/proxy your MCP tools.

**1. Create a New Server Definition**
1.  Navigate to the **Context Forge Admin UI**.
2.  Create a **New Server**.
3.  **Name**: `groupme`
4.  **Transport Type**: `HTTP` (Streamable)
5.  **Base URL**: `http://<service-name>.<namespace>.svc.cluster.local:5000/mcp`

**2. Configure Authentication (Virtual Server)**
*   **Auth Type**: `Bearer Token`
*   **Header Name**: `Authorization`
*   **Token Value**: `Bearer YOUR_ACCESS_TOKEN`
    *   *(Note: Context Forge will inject this header when calling the backend)*

---

## 💻 3. VSCode GitHub Copilot Configuration
To use this server in VSCode, add it to your `settings.json`.

### Option A: Direct Connection with Token (Recommended for Single User)
Connect directly to your deployed server and pass your GroupMe token in the header.

1.  Port-forward the service (if not exposed externally):
    ```bash
    kubectl port-forward svc/<service-name> 5000:5000 -n <namespace>
    ```

2.  Add to VS Code `settings.json`:
    ```json
    "mcp": {
        "servers": {
            "groupme": {
                "type": "http",
                "url": "http://localhost:5000/mcp",
                "headers": {
                    "Authorization": "Bearer YOUR_ACCESS_TOKEN"
                }
            }
        }
    }
    ```

    **Using Input Prompt for Token (More Secure):**
    ```json
    "mcp": {
        "inputs": [
            {
                "id": "groupme-token",
                "type": "promptString",
                "description": "Enter your GroupMe Access Token",
                "password": true
            }
        ],
        "servers": {
            "groupme": {
                "type": "http",
                "url": "http://localhost:5000/mcp",
                "headers": {
                    "Authorization": "Bearer ${input:groupme-token}"
                }
            }
        }
    }
    ```
    This prompts you for the token each session instead of storing it in settings.

### Option B: Via Context Forge Gateway
If you have added the server to Context Forge, use its proxy URL.

1.  Get the **Server ID** from the Context Forge URL (e.g., `.../servers/YOUR_ID_HERE/mcp`).
2.  Add to `settings.json`:

```json
"mcp": {
    "servers": {
        "groupme": {
            "type": "http",
            "url": "http://<your-domain>/servers/<server-id>/mcp"
        }
    }
}
```

### Option C: Local Docker (Standalone - No Server Required)
Run the server locally on your machine using Docker:

```json
"mcp": {
    "servers": {
        "groupme": {
            "command": "docker",
            "args": [
                "run",
                "-i",
                "--rm",
                "-e", "MCP_TRANSPORT=stdio",
                "-e", "GROUPME_ACCESS_TOKEN=${input:groupme-token}",
                "quazmoz/quazmoz:groupme"
            ]
        }
    },
    "inputs": [
        {
            "id": "groupme-token",
            "type": "promptString",
            "description": "Enter your GroupMe Access Token",
            "password": true
        }
    ]
}
```

### Option D: External URL (If Exposed via Ingress)
If your server is exposed externally (e.g., via Cloudflare Tunnel or Ingress):

```json
"mcp": {
    "servers": {
        "groupme": {
            "type": "http",
            "url": "https://<your-domain>/mcp",
            "headers": {
                "Authorization": "Bearer ${input:groupme-token}"
            }
        }
    },
    "inputs": [
        {
            "id": "groupme-token",
            "type": "promptString",
            "description": "Enter your GroupMe Access Token",
            "password": true
        }
    ]
}
```

---

## 🔑 Getting Your GroupMe Access Token

1. Go to [dev.groupme.com](https://dev.groupme.com/)
2. Log in with your GroupMe account
3. Click "Access Token" in the top right
4. Copy the token (starts with a long alphanumeric string)

---

## ✅ Verifying Your Setup

After configuring, test with GitHub Copilot:

1. Open VS Code Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`)
2. Type "MCP: List Servers" and verify `groupme` appears
3. Ask Copilot: "List my GroupMe groups"

If it works, you'll see your groups listed!

## Subgroup Routing Notes

When prompting agents or testing subgroup behavior manually:

- List subgroup/subtopic metadata with `/groups/{parent_group_id}/subgroups`
- Read subgroup messages with `/groups/{subgroup_id}/messages`
- Treat subgroup IDs as group-like IDs for message retrieval
- Do not teach or prompt subgroup reads as `/conversations/{id}/messages`

## 🤖 Automating with n8n

For creating scheduled automations such as meal polls in a specific group/subtopic, see the specialized guide:

> **[📝 Automating GroupMe Meal Polls with n8n](docs/n8n-meal-poll-automation.md)**

**Timezone Guidance:**
When creating scheduled polls via an automation engine like n8n, compute absolute expiration timestamps using the appropriate timezone (e.g., `America/New_York`) and pass `expiration_unix`. Using `expiration_unix` avoids timezone and duration bugs.
