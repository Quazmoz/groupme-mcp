# GroupMe MCP Server

A Go-based Model Context Protocol (MCP) server for GroupMe, enabling AI agents to interact with GroupMe groups, messages, DMs, and bots.

## Features

- **48 MCP Tools** covering all GroupMe operations
- **10 Combo Tools** for AI-friendly name-based operations (no IDs needed!)
- **Dual Mode Transport**: `stdio` for Claude/Cursor, `http` for OpenWebUI
- **Enterprise Features**: Rate limiting, structured logging, auto-pagination
- **Go 1.23+** for high performance
- **Docker deployment** with docker-compose

## Quick Start

> **[🚀 SEE SETUP.md FOR DETAILED SETUP GUIDE (OpenWebUI, VSCode, K8s)](SETUP.md)**

```bash
# Configure credentials
cp .env.example .env
# Edit .env and add your GROUPME_ACCESS_TOKEN

# Build and run
docker-compose up -d
```

## Local Go Dev

For local Windows builds/tests without polluting global Go cache locations, use:

```powershell
./local-go.ps1
```

Useful options:

```powershell
./local-go.ps1 -Action build
./local-go.ps1 -Action test
./local-go.ps1 -Action clean
./local-go.ps1 -CleanAfter
```

This keeps temporary Go artifacts inside `.gocache` and `.gotmp`, which are ignored by git.

## Available Tools (48 Total)

### 🔤 Combo Tools (Name-Based - No IDs Needed!)
| Tool | Description |
|------|-------------|
| `groupme_get_group_messages` | Get messages from a group by NAME |
| `groupme_send_to_group_by_name` | Send message to group by NAME |
| `groupme_get_dm_by_name` | Get DMs with a person by NAME |
| `groupme_send_dm_by_name` | Send DM to a person by NAME |
| `groupme_search_in_group_by_name` | Search messages in group by NAME |
| `groupme_send_with_mention` | @mention a user by NAME |
| `groupme_send_to_all` | @everyone - notify ALL members |
| `groupme_send_image` | Send image (secure base64 upload) |
| `groupme_send_location` | Send map location |

### 🔍 Discovery & Reliability Tools
| Tool | Description |
|------|-------------|
| `groupme_who_is` | Find a person across ALL groups & DMs |
| `groupme_list_group_members` | List members BEFORE mentioning |
| `groupme_list_matching_groups` | Show all groups matching a name |
| `groupme_probe_api_endpoints` | Safely compare read-only endpoint candidates for undocumented GroupMe APIs |

Example probe call:

```json
{
  "tool": "groupme_probe_api_endpoints",
  "arguments": {
    "candidates_json": "[{\"label\":\"subgroups-list\",\"endpoint\":\"/groups/{parent_group_id}/subgroups?page=1&per_page=25\"},{\"label\":\"subgroup-messages\",\"endpoint\":\"/groups/{subgroup_id}/messages?limit=5\"},{\"label\":\"baseline-groups\",\"endpoint\":\"/groups?page=1&per_page=5\"}]",
    "stop_on_first_success": false,
    "max_body_bytes": 4000
  }
}
```

Use this when the official docs are incomplete and you want the agent to compare several likely read-only endpoints side by side.

Subgroup routing guidance:
- List subgroups/subtopics with `/groups/{parent_group_id}/subgroups`
- Read subgroup messages with `/groups/{subgroup_id}/messages`
- Treat subgroup IDs as group-like IDs for message retrieval
- Do not model subgroup message reads as `/conversations/{id}/messages`

### 📺 Subtopic Automation
| Tool | Description |
|------|-------------|
| `groupme_list_subtopic_messages` | Read subtopic/channel messages using subgroup IDs through the groups namespace |
| `groupme_get_latest_youtube_link_from_subtopic` | Find the latest YouTube link in a subtopic |
| `groupme_forward_latest_youtube_link_from_subtopic` | Find the latest YouTube link in a subtopic and forward it to another group |

### 📁 Groups (9 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_groups` | List all groups |
| `groupme_search_group_by_name` | Find group by name |
| `groupme_get_group` | Get group details |
| `groupme_create_group` | Create a new group |
| `groupme_update_group` | Update group info |
| `groupme_destroy_group` | Delete group (creator only) |
| `groupme_list_former_groups` | List groups you left |
| `groupme_leave_group` | Leave a group |
| `groupme_rejoin_group` | Rejoin a former group |

### 👥 Member Management (3 tools)
| Tool | Description |
|------|-------------|
| `groupme_add_members` | Add members to group |
| `groupme_get_add_results` | Check add member status |
| `groupme_remove_member` | Remove member from group |
| `groupme_update_nickname` | Change your nickname |

### 💬 Messages (6 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_messages` | Get messages from a group |
| `groupme_send_message` | Send a message |
| `groupme_like_message` | Like a message |
| `groupme_unlike_message` | Unlike a message |
| `groupme_search_messages` | Search messages by text |

### 📨 Direct Messages (3 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_chats` | List DM conversations |
| `groupme_list_dm_messages` | Get DM messages |
| `groupme_send_dm` | Send direct message |

### 🤖 Bots (4 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_bots` | List your bots |
| `groupme_create_bot` | Create a new bot |
| `groupme_post_bot_message` | Post as bot |
| `groupme_destroy_bot` | Delete bot |

### 👤 Users (2 tools)
| Tool | Description |
|------|-------------|
| `groupme_get_current_user` | Get your info |
| `groupme_update_user` | Update your profile |

### 🚫 Blocks (4 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_blocks` | List blocked users |
| `groupme_block_user` | Block a user |
| `groupme_unblock_user` | Unblock a user |
| `groupme_block_exists` | Check if block exists |
	
### 📊 Polls (7 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_polls` | List polls in a group |
| `groupme_create_poll` | Create a new poll |
| `groupme_create_poll_by_group_name` | Create a poll by parent group name |
| `groupme_create_poll_in_subgroup_by_name` | Create a poll in a subtopic/subgroup |
| `groupme_get_poll` | Get poll details |
| `groupme_vote_poll` | Vote in a poll |
| `groupme_end_poll` | End a poll immediately |

**Poll Creation Parameters (`groupme_create_poll`):**
- `group_id`: The ID of the group.
- `subject`: The question to ask.
- `options`: JSON array or comma-separated list of choices.
- `expiration`: Relative duration in seconds.
- `expiration_unix`: Absolute Unix timestamp (overrides relative duration).
- `poll_type`: `single` or `multi` (default `multi`).
- `visibility`: `public` or `anonymous` (default `public` - required if operators need to see who voted).

**Subgroup Polling Guidance:**
- Subgroups are discovered with `/groups/{parent_group_id}/subgroups`.
- Subgroup IDs are treated as group-like IDs for poll creation and message retrieval.
- Avoid hardcoding private group IDs; instead, resolve groups by name using `groupme_create_poll_in_subgroup_by_name`.
	
## Enterprise Features

| Feature | Description |
|---------|-------------|
| **Rate Limiting** | Exponential backoff (1s, 2s, 4s) for 429 errors |
| **Structured Logging** | Debug mode with `SetDebug(true)` |
| **Auto-Pagination** | Fetch all groups/messages beyond 100 limit |
| **Input Validation** | Message (1000 chars), nickname (50), group name (140) |

## Configuration

### Single-User Mode (Default)

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `GROUPME_ACCESS_TOKEN` | GroupMe API access token | Yes | - |
| `PORT` | Server port (HTTP mode) | No | 5000 |
| `MCP_TRANSPORT` | Transport: `stdio` or `http` | No | `http` |

### Multi-User Mode (Per-User Tokens via JWT)

Enable multi-user mode by setting `ENCRYPTION_KEY` and `JWT_SECRET`. Users can then register their own GroupMe tokens.

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `ENCRYPTION_KEY` | 32+ char key for AES-256 token encryption | For multi-user | - |
| `JWT_SECRET` | Secret for validating Open WebUI JWTs | For multi-user | - |
| `MCP_SERVER_URI` | Expected JWT audience claim | No | `mcp://groupme.internal` |
| `TOKEN_EXPIRY_DAYS` | Days until stored tokens expire | No | 90 |

#### Auth Endpoints (Multi-User Mode Only)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/register` | Register GroupMe token for user |
| GET | `/auth/status` | Check if user has token registered |
| POST | `/auth/revoke` | Remove user's GroupMe token |

All auth endpoints require `Authorization: Bearer <JWT>` header.

## Authentication & Setup
The server supports two main deployment modes for OpenWebUI / Context Forge:

1.  **Solution A: Single User (Private)** - Hardcoded token, zero friction.
2.  **Solution B: Multi-User (Shared)** - Users log in with their own tokens.

> **[🚀 READ SETUP.md For Detailed Configuration Instructions](SETUP.md)**



## Deployment with Kubernetes

## Connecting to AI Agents

### GitHub Copilot (VS Code)
Add to your VS Code `settings.json`:

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

Then port-forward: `kubectl port-forward svc/groupme-backend 5000:5000 -n apps`

> **[📖 See SETUP.md for all options (Docker, Context Forge, External URL)](SETUP.md)**

### OpenWebUI
Refer to **Solution A** or **Solution B** in [SETUP.md](SETUP.md).

### Claude Desktop / Cursor
```json
{
  "mcpServers": {
    "groupme": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "MCP_TRANSPORT=stdio",
        "-e", "GROUPME_ACCESS_TOKEN=YOUR_TOKEN",
        "quazmoz/quazmoz:groupme"
      ]
    }
  }
}
```

## Docker Hub

```bash
docker pull quazmoz/quazmoz:groupme
```

## Emoji Support ✅

**Yes!** Standard Unicode emoji work in all messages:

```
"Send '🎉 Party time! 🎊' to the Testing group"
```

The AI can include any emoji in message text, and they'll display correctly in GroupMe.

## Known Limitations

### Image Uploads from OpenWebUI
When using OpenWebUI, you cannot directly upload an image from the chat interface to send via GroupMe. This is because:
- OpenWebUI passes images to AI models for vision tasks
- The AI model cannot extract raw image data to pass to MCP tools
- MCP/mcpo doesn't support binary image data passthrough

**Workarounds:**
1. **Use image URLs**: Upload your image somewhere (Imgur, your server) and provide the URL
2. **Use GroupMe directly**: For image-heavy messaging, use the GroupMe app

### Tool Limitations
| Limitation | Description |
|------------|-------------|
| **100 group limit** | Single API call returns max 100 groups (use pagination) |
| **Message length** | Max 1000 characters per message |
| **@all in large groups** | May exceed message limit with many members |

## API Reference

This server implements the [GroupMe API v3](https://dev.groupme.com/docs/v3).

## License

See the repository LICENSE file.
