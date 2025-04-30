#!/usr/bin/env bash
set -euo pipefail

# Check if running in GitHub Actions
if [[ -n "${GITHUB_ACTIONS:-}" ]] || [[ -n "${GITHUB_WORKFLOW:-}" ]]; then
  echo "Running in GitHub Actions workflow, skipping Node.js version check."
  exit 0
fi

# ─── STYLING AND CONSTANTS ────────────────────────────────────
# Colors and formatting
BOLD="$(tput bold)"
RESET="$(tput sgr0)"
RED="$(tput setaf 1)"
GREEN="$(tput setaf 2)"
YELLOW="$(tput setaf 3)"
CYAN="$(tput setaf 6)"

# Symbols
CHECK="✔"
CROSS="✖"
ARROW="➜"
WARNING="❗"

# Box dimensions
WIDTH=60
CONTENT_WIDTH=$((WIDTH - 2)) # Accounting for both border characters

# ─── UTILITY FUNCTIONS ────────────────────────────────────────
strip_ansi() {
  echo -e "$1" | sed 's/\x1B\[[0-9;]*[JKmsu]//g'
}

visible_width() {
  local stripped
  stripped=$(strip_ansi "$1")
  echo "${#stripped}"
}

pad_to_width() {
  local text="$1"
  local visible_len
  visible_len=$(visible_width "$text")
  local padding=$((CONTENT_WIDTH - visible_len))
  
  if [ "$padding" -gt 0 ]; then
    printf "%s%${padding}s" "$text" ""
  else
    echo "$text"
  fi
}

wrap_and_box() {
  local text="$1"
  local word line=""
  
  for word in $text; do
    local test_line="$line $word"
    local w=$(visible_width "$test_line")
    if [ "$w" -ge "$CONTENT_WIDTH" ]; then
      content=$(echo "$line" | sed 's/^ *//')
      padded_content=$(pad_to_width "$content")
      printf "${CYAN}│${RESET}%s${CYAN}│${RESET}\n" "$padded_content"
      line="$word"
    else
      line="$test_line"
    fi
  done
  
  if [[ -n "$line" ]]; then
    content=$(echo "$line" | sed 's/^ *//')
    padded_content=$(pad_to_width "$content")
    printf "${CYAN}│${RESET}%s${CYAN}│${RESET}\n" "$padded_content"
  fi
}

draw_border() {
  local line
  line="$(printf '─%.0s' $(seq 1 "$CONTENT_WIDTH"))"
  case "$1" in
    top) echo -e "${CYAN}╭${line}╮${RESET}" ;;
    mid) echo -e "${CYAN}├${line}┤${RESET}" ;;
    bot) echo -e "${CYAN}╰${line}╯${RESET}" ;;
  esac
}

display_box() {
  local title="$1"
  shift
  
  draw_border top
  
  if [ -n "$title" ]; then
    wrap_and_box "${title}"
    draw_border mid
  fi
  
  for message in "$@"; do
    wrap_and_box "$message"
  done
  
  draw_border bot
}

# ─── MAIN EXECUTION ────────────────────────────────────────────
export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
if [ -s "$NVM_DIR/nvm.sh" ]; then
  . "$NVM_DIR/nvm.sh"
else
  draw_border top
  wrap_and_box "${RED}${BOLD}${CROSS} ERROR: NVM not found at $NVM_DIR"
  wrap_and_box ""
  wrap_and_box "${YELLOW}• Please install NVM to continue:"
  wrap_and_box "  ${ARROW} https://github.com/nvm-sh/nvm"
  draw_border bot
  exit 0
fi

CURRENT="$(node -v 2>/dev/null | sed 's/^v//')"
set +u

# Get latest LTS version
LTS_VERSION="$(nvm version-remote --lts 2>/dev/null | sed 's/^v//' || true)"

# Fallback to parsing nvm ls-remote if version-remote doesn't work
if [ -z "$LTS_VERSION" ]; then
  LTS_RAW="$(nvm ls-remote --lts | tail -1 || true)"
  LTS_VERSION="$(echo "$LTS_RAW" | awk '{print $1}' | sed 's/^v//')"
fi

set -u
LATEST="$LTS_VERSION"

if [ -z "$CURRENT" ] || [ -z "$LATEST" ]; then
  draw_border top
  wrap_and_box "${RED}${BOLD}${CROSS} ERROR: Could not detect Node versions."
  wrap_and_box ""
  wrap_and_box "${YELLOW}• Try manually:"
  wrap_and_box "${ARROW} nvm install --lts && nvm use --lts"
  draw_border bot
  exit 0
fi  

if [ "$CURRENT" != "$LATEST" ]; then
  draw_border top
  wrap_and_box "${YELLOW}${BOLD}${ARROW} NODE VERSION MISMATCH!"
  draw_border mid
  wrap_and_box "Current version:       v$CURRENT"
  wrap_and_box "Latest LTS version:    v$LATEST"
  draw_border mid
  wrap_and_box "Installing latest LTS..."
  nvm install --lts >/dev/null 2>&1
  wrap_and_box "${GREEN}${BOLD}${CHECK} LTS installed successfully."
  draw_border mid
  wrap_and_box "Setting latest LTS version as default..."
  nvm alias default lts/* >/dev/null 2>&1
  wrap_and_box "${GREEN}${BOLD}${CHECK} LTS set as default successfully."

  draw_border mid
  wrap_and_box "${ARROW} You're almost ready!"
  wrap_and_box "Run the following command to use it in this session:"
  wrap_and_box "${BOLD}nvm use --lts"

  draw_border mid
  wrap_and_box "This step is REQUIRED before continuing."
  wrap_and_box "The app may not work properly if you skip it."
  draw_border mid
  wrap_and_box "${ARROW} Then run: ${BOLD}npm install"
  draw_border bot
  exit 0
fi

draw_border top
wrap_and_box "${GREEN}${CHECK} Node.js v$CURRENT is up-to-date (LTS)."
draw_border bot
