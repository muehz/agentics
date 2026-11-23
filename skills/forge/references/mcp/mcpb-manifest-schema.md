# MCPB Manifest Schema

MCPB (MCP Bundle) packages a local stdio MCP server with its runtime, so users can drag-install it without their own
Node.js or Python setup.

## Bundle Contents

```
my-server.mcpb/
├── manifest.json       # required — describes the server
├── server/             # required — the server code and dependencies
│   ├── index.js        # entry point
│   ├── package.json
│   └── node_modules/   # bundled dependencies (or python equivalents)
├── icon.png            # optional — 128x128 or 256x256
└── README.md           # optional — surfaced during install
```

## manifest.json

```json
{
    "manifest_version": "0.3",
    "name": "my-server",
    "display_name": "My Server",
    "version": "1.2.0",
    "description": "Short tagline that appears in the install dialog",
    "author": {
        "name": "Your Name",
        "email": "you@example.com",
        "url": "https://example.com"
    },
    "homepage": "https://github.com/you/my-server",
    "license": "MIT",
    "server": {
        "type": "node",
        "entry_point": "server/index.js",
        "mcp_config": {
            "command": "node",
            "args": ["${__dirname}/server/index.js"]
        }
    },
    "user_config": {
        "api_key": {
            "type": "string",
            "title": "API key",
            "description": "Your API key for the upstream service",
            "required": true,
            "sensitive": true
        },
        "workspace_path": {
            "type": "directory",
            "title": "Workspace",
            "description": "Local workspace to operate on",
            "required": false,
            "default": "${HOME}/Documents"
        }
    }
}
```

## Field Reference

### Required

| Field                | Type   | Notes                                                     |
| -------------------- | ------ | --------------------------------------------------------- |
| `manifest_version`   | string | Spec version this manifest conforms to; currently `"0.3"` |
| `name`               | string | Lowercase, hyphenated, unique per author                  |
| `version`            | string | Semver                                                    |
| `description`        | string | Under 100 chars; shown in the install dialog              |
| `server`             | object | Server type and launch config                             |
| `server.entry_point` | string | Path to the server's entry file inside the bundle         |

### server.type

| Value    | Runtime expected                          |
| -------- | ----------------------------------------- |
| `node`   | Node.js bundled in `server/node_modules/` |
| `python` | Python with `requirements.txt` resolved   |
| `binary` | Pre-compiled binary in `server/`          |

### server.mcp_config

This object holds the launch command. Use `${__dirname}` for paths inside the bundle.

```json
"mcp_config": {
  "command": "node",
  "args": ["${__dirname}/server/index.js"],
  "env": {
    "NODE_ENV": "production"
  }
}
```

For Python:

```json
"mcp_config": {
  "command": "python",
  "args": ["${__dirname}/server/main.py"]
}
```

### user_config

These are install-time settings. The host UI prompts the user for each field.

| Type        | Behavior                                                   |
| ----------- | ---------------------------------------------------------- |
| `string`    | Text input; mark `sensitive: true` for passwords/keys      |
| `number`    | Numeric input                                              |
| `boolean`   | Checkbox                                                   |
| `directory` | Folder picker; `default` can use `${HOME}`, `${DOCUMENTS}` |
| `file`      | File picker                                                |

There is no `select`/dropdown type — the enum is limited to the five types above.

User-config values are **not** auto-injected as env vars (there is no `MCPB_<KEY>` mechanism). Reference them explicitly
with `${user_config.KEY}` template substitution, wired into `server.mcp_config.env` or `args`:

```json
"mcp_config": {
  "command": "node",
  "args": ["${__dirname}/server/index.js"],
  "env": {
    "API_KEY": "${user_config.api_key}"
  }
}
```

## Build Process

```bash
# 1. Bundle dependencies (Node.js)
(cd my-server/server && npm install --production)

# 2. Pack the bundle
npx @anthropic-ai/mcpb pack ./my-server my-server.mcpb

# 3. Test by drag-installing into Claude Desktop
# (drop the .mcpb file onto the Claude Desktop window)
```

For Python, freeze the dependencies. Bundle the Python interpreter, or rely on a system Python with documented version
constraints.

## Versioning

- Bump `manifest_version` only on breaking schema changes (rare)
- Bump `version` on every release, with semver
- Users get a non-blocking prompt on update, not auto-update

## Sign the Bundle (Optional)

For public distribution, sign the bundle with `npx @anthropic-ai/mcpb sign`. Signed bundles install without warnings.
Unsigned bundles trigger a confirmation dialog.

## Common Mistakes

- `node_modules` bundled with platform-specific binaries → fails on a different OS. Rebuild per platform, or document
  the constraints.
- Hardcoded paths instead of `${__dirname}` → break when the bundle is installed to a non-default location
- No `sensitive: true` on user_config sensitive fields → values appear in plain text in the UI
- No reasonable defaults for `directory` fields → forces users to navigate filesystems mid-install
- Distribution without a signature → users see a scary warning dialog
