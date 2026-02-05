# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

source $PWD/pkg/shell/colors.sh

border() {
  local str="$*" # Put all arguments into single string
  local len=${#str}
  local i
  for ((i = 0; i < len + 4; ++i)); do
    printf '-'
  done
  printf "\n $str \n"
  for ((i = 0; i < len + 4; ++i)); do
    printf '-'
  done
  echo
}

lineOk() {
  local str="$1${green} ✔️${normal}"
  printf "${green}${bold}[ok]${normal} $str\n"
}

lineError() {
  local str="${red}$*  ✗${normal}"
  printf "$str\n"
}

title1() {
  local str="$*"
  printf "\n"
  border "📝  ${bold}$str${normal}"
  printf "\n"
}
