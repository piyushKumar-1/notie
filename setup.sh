#!/usr/bin/env bash
# setup.sh — build and install notie for a fresh clone.
#
#   ./setup.sh          interactive setup
#   ./setup.sh --yes    accept all defaults (no prompts)
#
# Steps: check Go → build → install to PATH → create notes dir →
# offer the zsh shell-audit hook → report optional dependencies.

set -euo pipefail
cd "$(dirname "$0")"

YES=false
[[ "${1:-}" == "--yes" || "${1:-}" == "-y" ]] && YES=true

bold=$'\033[1m'; green=$'\033[32m'; yellow=$'\033[33m'; red=$'\033[31m'; reset=$'\033[0m'
ok()   { echo "  ${green}✓${reset} $*"; }
warn() { echo "  ${yellow}·${reset} $*"; }
die()  { echo "  ${red}✗${reset} $*" >&2; exit 1; }

ask() { # ask "question" -> 0 if yes; --yes answers yes to everything
  $YES && return 0
  read -r -p "$1 [Y/n] " reply
  [[ -z "$reply" || "$reply" =~ ^[Yy] ]]
}

echo "${bold}notie setup${reset}"

# download the official Go toolchain into ~/.cache/notie and put it on PATH
# for this script only — used when Go isn't installed and brew isn't available
download_go() {
  local os arch ver cache="$HOME/.cache/notie"
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$(uname -m)" in
    x86_64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) die "unsupported architecture: $(uname -m) — install Go manually from https://go.dev/dl/" ;;
  esac
  ver=$(curl -fsSL 'https://go.dev/VERSION?m=text' 2>/dev/null | head -1) || ver=""
  [[ "$ver" == go* ]] || ver=go1.25.0
  if [[ ! -x "$cache/go/bin/go" ]]; then
    echo "  downloading $ver for $os/$arch (one-time, ~70 MB)…"
    mkdir -p "$cache"
    curl -fL --progress-bar "https://go.dev/dl/${ver}.${os}-${arch}.tar.gz" | tar -xz -C "$cache" \
      || die "download failed — install Go manually from https://go.dev/dl/"
  fi
  export PATH="$cache/go/bin:$PATH"
  ok "using Go toolchain from $cache/go (build-only, not installed system-wide)"
}

# 1. Go toolchain
echo "${bold}1. checking Go${reset}"
if command -v go >/dev/null 2>&1; then
  ok "$(go version)"
elif command -v brew >/dev/null 2>&1 && ask "Go is not installed — install it with Homebrew?"; then
  brew install go
  ok "$(go version)"
elif ask "Go is not installed — download it to ~/.cache/notie just for this build?"; then
  download_go
else
  die "Go is required — install from https://go.dev/dl/ (or: brew install go)"
fi

# 2. Build
echo "${bold}2. building${reset}"
go build -o notie .
ok "built ./notie"

# 3. Install onto PATH
echo "${bold}3. installing${reset}"
install_dir="$HOME/.local/bin"
if [[ -d /usr/local/bin && -w /usr/local/bin ]]; then
  install_dir=/usr/local/bin
fi
mkdir -p "$install_dir"
cp notie "$install_dir/notie"
ok "installed to $install_dir/notie"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) warn "$install_dir is not on your PATH — add to ~/.zshrc:
      export PATH=\"$install_dir:\$PATH\"" ;;
esac

# 4. Notes directory
echo "${bold}4. notes directory${reset}"
notie_dir="${NOTIE_DIR:-$HOME/.notie}"
mkdir -p "$notie_dir"
ok "notes live in $notie_dir"

# 5. zsh shell-audit hook (optional)
echo "${bold}5. shell audit trail (optional)${reset}"
zshrc="$HOME/.zshrc"
hook_marker="# notie shell audit trail"
if [[ -f "$zshrc" ]] && grep -qF "$hook_marker" "$zshrc"; then
  ok "zsh hook already installed in $zshrc"
elif ask "Log every shell command to notie's audit trail (adds a preexec hook to ~/.zshrc)?"; then
  cat >>"$zshrc" <<'EOF'

# notie shell audit trail
_notie_log() { command notie log "$1" >/dev/null 2>&1 }
autoload -Uz add-zsh-hook
add-zsh-hook preexec _notie_log
EOF
  ok "hook added to $zshrc — takes effect in new shells"
else
  warn "skipped — add it later, see README.md"
fi

# 6. Optional dependencies (voice notes & summaries)
echo "${bold}6. optional dependencies${reset}"
if command -v ffmpeg >/dev/null 2>&1; then
  ok "ffmpeg — voice recording available"
else
  warn "ffmpeg missing — voice notes (notie radd) need it: brew install ffmpeg"
fi
if command -v hear >/dev/null 2>&1; then
  ok "hear — on-device transcription (Apple Speech)"
elif command -v whisper-cli >/dev/null 2>&1; then
  ok "whisper-cli — transcription available (needs a model in ~/.cache/whisper)"
else
  warn "no transcriber — voice notes need one: brew install hear   (or: brew install whisper-cpp)"
fi
if command -v claude >/dev/null 2>&1; then
  ok "claude — nicer daily summaries for 'notie cache'"
else
  warn "claude CLI missing — 'notie cache' falls back to joining raw entries"
fi

echo
echo "${bold}done.${reset} try: ${bold}notie add \"set up notie\"${reset}"
