#!/bin/bash
#
# This script provides utility functions for creating ASCII-art formatted output
# in shell scripts, such as borders, titles, and status lines. It depends on
# 'colors.sh' for color definitions.

source "$PWD/pkg/shell/colors.sh"

# border prints a string with a border of dashes above and below it.
#
# Arguments:
#   $*: The string to be enclosed in the border.
#
# Example:
#   border "Hello, World!"
#
# Output:
#   -----------------
#    Hello, World!
#   -----------------
#
border() {
  local str="$*" # Put all arguments into single string
  local len=${#str}
  local i
  for ((i = 0; i < len + 4; ++i)); do
    printf '-'
  done
  printf "\\n| $str |\\n"
  for ((i = 0; i < len + 4; ++i)); do
    printf '-'
  done
  echo
}

# lineOk prints a success message with a green checkmark.
#
# Arguments:
#   $1: The success message to display.
#
# Example:
#   lineOk "Task completed successfully."
#
# Output:
#   [ok] Task completed successfully. ✔️
#
lineOk() {
  local str="$1${green} ✔️${normal}"
  printf "${green}${bold}[ok]${normal} $str\\n"
}

# lineError prints an error message with a red cross.
#
# Arguments:
#   $*: The error message to display.
#
# Example:
#   lineError "File not found."
#
# Output:
#   File not found.  ✗
#
lineError() {
  local str="${red}$*  ✗${normal}"
  printf "$str\\n"
}

# title1 prints a formatted title with a border.
#
# Arguments:
#   $*: The title string.
#
# Example:
#   title1 "Initializing System"
#
# Output:
#
#   -------------------------
#   | 📝  Initializing System |
#   -------------------------
#
title1() {
  local str="$*"
  printf "\\n"
  border "📝  ${bold}$str${normal}"
  printf "\\n"
}
