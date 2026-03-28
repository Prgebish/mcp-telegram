# Manual Integration Testing

## Step 1: Get Telegram API credentials

1. Go to https://my.telegram.org (log in with your phone number)
2. Click "API development tools"
3. Fill the form:
   - App title: `mcp-telegram`
   - Short name: `mcptg`
   - Platform: Desktop
   - Description: `MCP server`
4. Click "Create application"
5. Copy **App api_id** (a number) and **App api_hash** (a hex string)

## Step 2: Set environment variables

```bash
export TG_APP_ID=36748340
export TG_API_HASH=6bd51243ea6558aaabc633b4e16c6dd5
```

Add these to your `~/.zshrc` or `~/.zshenv` if you want them to persist.

## Step 3: Build the binary

```bash
cd ~/projects/mcp-telegram
go build -o mcp-telegram ./cmd/mcp-telegram
```

Verify it built:

```bash
./mcp-telegram --help
```

## Step 4: Create config.yaml

Create `~/projects/mcp-telegram/config.yaml`:

```yaml
telegram:
  app_id: ${TG_APP_ID}
  api_hash: ${TG_API_HASH}

acl:
  chats:
    # Replace with your real chats:
    - match: "@durov"                    # any user with username
      permissions: [read]
    - match: "user:YOUR_FRIEND_ID"       # a user without username (get ID from tg_dialogs later)
      permissions: [read, draft]
    - match: "channel:YOUR_CHANNEL_ID"   # a channel you're subscribed to
      permissions: [read, mark_read]
    - match: "chat:YOUR_GROUP_ID"        # a group chat
      permissions: [read]
```

You can start with just one chat (e.g. `@durov`) and add more after you see IDs from `tg_dialogs`.

## Step 5: Authenticate with Telegram

```bash
./mcp-telegram auth --config config.yaml
```

This will:
1. Ask for your phone number (format: +79001234567)
2. Send a login code to your Telegram app
3. Ask you to enter the code
4. Ask for 2FA password if you have one enabled
5. Save the session to `~/.config/mcp-telegram/session.json`

**NOTE: The `auth` command is not yet implemented. It needs to be added before testing.**

After auth succeeds, you should see the session file:

```bash
ls -la ~/.config/mcp-telegram/session.json
```

## Step 6: Test via Claude Code

### 6a. Add MCP server config

Create or edit `~/projects/mcp-telegram/.mcp.json`:

```json
{
  "mcpServers": {
    "telegram": {
      "command": "/Users/YOUR_USERNAME/projects/mcp-telegram/mcp-telegram",
      "args": ["--config", "/Users/YOUR_USERNAME/projects/mcp-telegram/config.yaml"],
      "env": {
        "TG_APP_ID": "your_app_id_here",
        "TG_API_HASH": "your_api_hash_here"
      }
    }
  }
}
```

Replace `YOUR_USERNAME` and credential values.

### 6b. Start Claude Code

```bash
cd ~/projects/mcp-telegram
claude
```

Claude Code should show `telegram` in its MCP server list on startup.

### 6c. Run tests

Ask Claude to call each tool. Example prompts:

**tg_me:**
> Call tg_me

Expected: your Telegram ID, name, username.

**tg_dialogs:**
> Call tg_dialogs

Expected: only chats from your ACL whitelist appear. Note the IDs — you can use them to update config.yaml with more chats.

> Call tg_dialogs with only_unread=true

Expected: only whitelisted chats with unread messages.

**tg_history:**
> Call tg_history for @durov

Expected: message history with [ID] sender (date): text format.

> Call tg_history for @durov with offset_id=LAST_ID_FROM_PREVIOUS

Expected: older messages (pagination works).

> Call tg_history for @some_chat_not_in_whitelist

Expected: error "access denied: ... does not have 'read' permission".

> Call tg_history for @totally_nonexistent_username_xyz

Expected: error "cannot resolve chat: ...".

**tg_draft:**
> Call tg_draft for @friend with text "test draft from MCP"

Expected: "Draft saved in @friend". Check your Telegram app — the draft should appear in that chat.

> Call tg_draft for a chat that only has [read] permission

Expected: error "access denied: ... does not have 'draft' permission".

**tg_mark_read:**
> Call tg_mark_read for channel:CHANNEL_ID

Expected: "Marked as read: channel:...". Check Telegram — unread counter should reset.

> Call tg_mark_read for a chat without mark_read permission

Expected: error "access denied: ... does not have 'mark_read' permission".

## Step 7: Test cold start peer resolution

1. Stop Claude Code (Ctrl+C)
2. Start it again: `claude`
3. Immediately ask: `Call tg_history for user:SOME_ID`
4. Expected: works without error (warm-up cache populated on start)

## Step 8: Update config with real IDs

After running `tg_dialogs`, you'll see refs like `user:123456` and `channel:789012`.
Update `config.yaml` with these real IDs and restart to test typed ID resolution.

## Checklist

- [ ] tg_me returns account info
- [ ] tg_dialogs shows only whitelisted chats
- [ ] tg_dialogs only_unread filter works
- [ ] tg_history returns messages by @username
- [ ] tg_history returns messages by user:ID
- [ ] tg_history returns messages by channel:ID
- [ ] tg_history pagination with offset_id works
- [ ] tg_history ACL deny works
- [ ] tg_history unknown peer gives clear error
- [ ] tg_draft saves draft (visible in Telegram app)
- [ ] tg_draft ACL deny works
- [ ] tg_mark_read resets unread counter
- [ ] tg_mark_read ACL deny works
- [ ] Cold start: user:ID works immediately after restart
- [ ] Rate limiting: rapid calls don't cause flood ban
