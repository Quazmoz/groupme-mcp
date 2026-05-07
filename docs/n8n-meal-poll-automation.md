# Automating GroupMe Meal Polls with n8n

This guide explains how to use the GroupMe MCP Server with n8n to schedule automated meal polls in a GroupMe subgroup/topic.

## Prerequisites
1. n8n installed and configured.
2. GroupMe MCP Server configured in your n8n environment.
3. Your GroupMe `GROUPME_ACCESS_TOKEN` stored securely in n8n credentials or environment variables.

## Overview
GroupMe polls created via MCP can specify `poll_type` (single vs. multi-choice) and `visibility` (public vs. anonymous).

By default, polls are multi-choice and public. Public visibility is required if organizers need to see who voted.

For scheduled automations, we highly recommend passing an absolute `expiration_unix` timestamp rather than a relative duration. Your n8n workflow can compute this dynamically based on your local timezone.

## Example Tools to Use
For meal planning in a specific subtopic, use the `groupme_create_poll_in_subgroup_by_name` tool.

This avoids hardcoding any private Group IDs in your workflow.

### Generic Example Schedule
Suppose you want to automate weekly polls:
- Monday 8:45 AM America/New_York (Dinner in Zion tonight?)
- Wednesday 8:45 AM America/New_York
- Thursday 8:45 AM America/New_York (Sabbath and regular dinner polls)
- Friday 8:45 AM America/New_York (Sunday polls)

Use n8n Schedule Triggers set to `America/New_York`.

### Workflow Variables
Use placeholders in your n8n workflow instead of raw values:
- `PARENT_GROUP_NAME` = `[YOUR GROUP NAME]`
- `SUBGROUP_TOPIC` = `[YOUR SUBGROUP/TOPIC NAME]`

### Example Poll Patterns

#### Regular Dinner Poll (Single Choice)
- **Subject**: `Dinner in Zion tonight?`
- **Options**: `["Yes", "No"]`
- **Poll Type**: `single`
- **Visibility**: `public`
- **Closes**: Same day at 4:00 PM America/New_York

**MCP Call Payload:**
```json
{
  "tool": "groupme_create_poll_in_subgroup_by_name",
  "arguments": {
    "parent_group_name": "[YOUR GROUP NAME]",
    "subgroup_topic": "[YOUR SUBGROUP/TOPIC NAME]",
    "subject": "Dinner in Zion tonight?",
    "options": "[\"Yes\", \"No\"]",
    "poll_type": "single",
    "visibility": "public",
    "expiration_unix": 1770000000
  }
}
```

#### Thursday Extra Dinner Poll
- **Subject**: `Thursday Dinner`
- **Options**: `["Yes", "No"]`
- **Poll Type**: `single`
- **Visibility**: `public`
- **Closes**: Same day at 4:00 PM America/New_York

#### Thursday Sabbath Polls (Multi-Choice)
- **Subject**: `Sabbath Lunch` (or `Sabbath Dinner`)
- **Options**: `["Option 1", "Option 2", "Option 3"]`
- **Poll Type**: `multi`
- **Visibility**: `public`
- **Closes**: Same day at 8:00 PM America/New_York

**MCP Call Payload:**
```json
{
  "tool": "groupme_create_poll_in_subgroup_by_name",
  "arguments": {
    "parent_group_name": "[YOUR GROUP NAME]",
    "subgroup_topic": "[YOUR SUBGROUP/TOPIC NAME]",
    "subject": "Sabbath Lunch",
    "options": "[\"Option 1\", \"Option 2\", \"Option 3\"]",
    "poll_type": "multi",
    "visibility": "public",
    "expiration_unix": 1770000000
  }
}
```

#### Friday Sunday Polls (Multi-Choice)
- **Subject**: `Sunday Lunch` (or `Sunday Dinner`)
- **Options**: `["Option 1", "Option 2", "Option 3"]`
- **Poll Type**: `multi`
- **Visibility**: `public`
- **Closes**: Same day at 8:00 PM America/New_York

## Recommendations
- Always parse the `expiration_unix` properly in your n8n "Date & Time" or Code node before passing it to the MCP call.
- Do not commit your `.env` or any export containing your real GroupMe access token.
