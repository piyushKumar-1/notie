---
okf_version: "0.1"
---

# Files

- [Architecture](architecture.md) - Single-binary design, stdlib-only constraints, data layout under ~/.notie, and markdown file formats used by the notie CLI.
- [notie data model](data-model.md) - Exact file layout and line formats used by notie in ~/.notie — journals, tasks, important/remember notes, shell audit, datecache, and the task id counter.
- [Integrations](integrations.md) - Optional runtime integrations that extend notie beyond the core binary — voice recording, the zsh shell-audit hook, and the Claude Code notie-review skill.
- [Operations](operations.md) - How to build, install, configure, and troubleshoot notie; covering setup.sh, environment variables, optional dependencies, and common failure modes.
- [notie quickstart](quickstart.md) - Entrypoint for understanding the notie local notes CLI — a single Go binary that stores journals, tasks, reminders, and a shell audit trail as plain markdown in ~/.notie.
- [Source map](source-map.md) - File-by-file guide to the notie codebase — what each source file owns, key functions, and where to start when changing behavior.
- [notie testing](testing.md) - How to test notie changes today — manual checks, regression risks, and guidance for adding tests to the dependency-free Go codebase.
- [TUI systems](tui.md) - The three terminal UIs in notie — day browser, task list, and notes list — including shared vim-inspired key bindings and raw-mode terminal handling.
- [Core workflows](workflows.md) - End-to-end workflows for adding journal entries, retroactive dating, managing tasks, building the date cache, and displaying stored data in notie.
