with open("README.md", "r", encoding="utf-8") as f:
    content = f.read()

content = content.replace("- **46 MCP Tools** covering all GroupMe operations", "- **48 MCP Tools** covering all GroupMe operations")
content = content.replace("## Available Tools (46 Total)", "## Available Tools (48 Total)")

poll_target = """### 📊 Polls (3 tools)
| Tool | Description |
|------|-------------|
| `groupme_list_polls` | List polls in a group |
| `groupme_create_poll` | Create a new poll |
| `groupme_get_poll` | Get poll details |"""

poll_replacement = """### 📊 Polls (7 tools)
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
- Avoid hardcoding private group IDs; instead, resolve groups by name using `groupme_create_poll_in_subgroup_by_name`."""

content = content.replace(poll_target, poll_replacement)

with open("README.md", "w", encoding="utf-8") as f:
    f.write(content)
