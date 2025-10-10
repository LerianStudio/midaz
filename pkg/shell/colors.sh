#!/bin/bash
#
# This script defines a set of variables for colored and formatted text output
# in the terminal. It checks if the terminal supports colors before defining
# the variables to ensure compatibility.
#
# Usage:
#   Source this script in your shell scripts to use the color variables.
#   Example:
#     source "colors.sh"
#     echo "${red}This is an error message.${normal}"
#     echo "${bold}${green}This is a success message.${normal}"

# Check if stdout is a terminal.
if test -t 1; then

    # Check if the terminal supports colors.
    ncolors=$(tput colors)

    if test -n "$ncolors" && test $ncolors -ge 8; then
        bold="$(tput bold)"
        underline="$(tput smul)"
        standout="$(tput smso)"
        normal="$(tput sgr0)"
        black="$(tput setaf 0)"
        red="$(tput setaf 1)"
        green="$(tput setaf 2)"
        yellow="$(tput setaf 3)"
        blue="$(tput setaf 4)"
        magenta="$(tput setaf 5)"
        cyan="$(tput setaf 6)"
        white="$(tput setaf 7)"
    fi
fi