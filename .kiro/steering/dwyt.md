<!-- DWYT:START -->
#dwyt

# DWYT - Don't Waste Your Tokens

This project uses DWYT for context and token optimization.

## Available MCPs

- **dwyt_optimizer** — context optimizer and efficiency policy.
- **dwyt_obsidian** — persistent project memory and canonical knowledge.
- **dwyt_codebase** — structural code retrieval.

## Entry Contract

Before broad repository or memory retrieval, call the DWYT Optimizer
(`dwyt_context_plan`) to obtain a context plan, then stay inside its
budget and retrieval boundaries.

Prefer:

- canonical memory over old sessions;
- symbols and line ranges over full files;
- summaries over raw output;
- incremental retrieval over bulk context loading;
- reusing context already obtained over retrieving it again.

Use dwyt_obsidian as the project brain and dwyt_codebase as the source of current
structure. Request raw or full data only when the compact context is
insufficient; raw output stays retrievable by reference
(`dwyt_get_raw`).

Prefix shell commands with `rtk` where supported
(`rtk go test ./...`). RTK reduces terminal output; it is not an MCP.

At the end of a task, persist a compact context snapshot with
`obsidian_save_context` when the task state changed. Set `client`
to the current client (codex, opencode, claude, cursor, kiro, copilot,
windsurf, continue). If saving fails, say so in the final response.

Keep operational answers short: status, changed files, validation, blockers.
Do not truncate an artifact the user asked for.
<!-- DWYT:END -->
