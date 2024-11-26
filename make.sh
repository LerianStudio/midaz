#!/bin/bash

LOGO=$(cat "$PWD"/pkg/shell/logo.txt)
GITHOOKS_PATH="$PWD"/.githooks
GIT_HOOKS_PATH="$PWD"/.git/hooks

source "$PWD"/pkg/shell/colors.sh
source "$PWD"/pkg/shell/ascii.sh

echo "${bold}${blue}$LOGO${normal}"

makeCmd() {
  cmd=$1
  for DIR in "$PWD"/*; do
    FILE="$DIR"/Makefile
    if [ -f "$FILE" ]; then
      if grep -q "$cmd:" "$FILE"; then
        (
          cd "$DIR" || exit
          echo ""
          border "########### Executing ${magenta}make $1${normal} command in package ${bold}${blue}$DIR${normal} ###########"
          make $cmd
        )
        err=$?
        if [ $err -ne 0 ]; then
          echo -e "\n${bold}${red}An error has occurred during test process ${bold}[FAIL]${norma}\n"
          exit 1
        fi
      fi
    fi
  done
}

setupGitHooks() {
  title1 "Setting up git hooks..."

  find $GITHOOKS_PATH -type f -exec cp {} $GIT_HOOKS_PATH \;
  chmod +x .git/hooks/*

  lineOk "All hooks installed and updated"
}

checkHooks() {
  err=0
  echo "Checking git hooks..."
  for FILE in "$GITHOOKS_PATH"/*; do
    f="$(basename -- $FILE)"
    FILE2="$GIT_HOOKS_PATH"/$f
    if [ -f "$FILE2" ]; then
      if cmp -s "$FILE" "$FILE2"; then
        lineOk "Hook file ${underline}$f${normal} installed and updated"
      else
        lineError "Hook file ${underline}$f${normal} ${red}installed but out-of-date [OUT-OF-DATE]"
        err=1
      fi
    else
      lineError "Hook file ${underline}$f${normal} ${red}not installed [NOT INSTALLED]"
      err=1
    fi
    if [ $err -ne 0 ]; then
      echo -e "\nRun ${bold}make setup-git-hooks${normal} to setup your development environment, then try again.\n"
      exit 1
    fi
  done

  lineOk "\nAll good"
}

lint() {
  title1 "STARTING LINT"
  
  if ! command -v golangci-lint &> /dev/null; then
    echo -e "\n${bold}${red}golangci-lint is not installed. Please install it first.${normal}\n"
    exit 1
  fi
  
  out=$(golangci-lint run --fix ./... 2>&1)
  out_err=$?
  perf_out=$(perfsprint ./... 2>&1)
  perf_err=$?

  echo "$out"
  echo "$perf_out"

  if [ $out_err -ne 0 ]; then
    echo -e "\n${bold}${red}An error has occurred during the lint process: \n $out\n"
    exit 1
  fi
  if [ $perf_err -ne 0 ]; then
    echo -e "\n${bold}${red}An error has occurred during the performance check: \n $perf_out\n"
    exit 1
  fi

  lineOk "Lint and performance checks passed successfully"
}

format() {
  title1 "Formatting all golang source code"
  
  if ! command -v gofmt &> /dev/null; then
    echo -e "\n${bold}${red}gofmt is not installed. Please install Go first.${normal}\n"
    exit 1
  fi
  
  gofmt -w ./
  lineOk "All go files formatted"
}

checkLogs() {
  title1 "STARTING LOGS ANALYZER"
  err=0
  while IFS= read -r path; do
    if grep -q 'err != nil' "$path" && ! grep -qE '(logger\.Error|log\.Error)' "$path" && [[ "$path" != *"_test"* ]]; then
      err=1
      echo "$path"
    fi
  done < <(find . -type f -path '*usecase*/*' -name '*.go')

  if [ $err -eq 1 ]; then
    echo -e "\n${red}You need to log all errors inside usecases after they are handled. ${bold}[WARNING]${normal}\n"
    exit 1
  else
    lineOk "All good"
  fi
}

checkTests() {
  title1 "STARTING TESTS ANALYZER"
  err=false
  subdirs="components/*/internal/services/query components/*/internal/services/command"

  for subdir in $subdirs; do
    while IFS= read -r file; do
      if [[ "$file" != *"_test.go" ]]; then
        test_file="${file%.go}_test.go"
        if [ ! -f "$test_file" ]; then
          echo "Error: There is no test for the file $file"
          err=true
        fi
      fi
    done < <(find "$subdir" -type f -name "*.go")
  done

  if [ "$err" = true ]; then
    echo -e "\n${red}There are files without corresponding test files.${normal}\n"
    exit 1
  else
    lineOk "All tests are in place"
  fi
}

case "$1" in
"setupGitHooks")
  setupGitHooks
  ;;
"checkHooks")
  checkHooks
  ;;
"lint")
  lint
  ;;
"format")
  format
  ;;
"checkLogs")
  checkLogs
  ;;
"checkTests")
  checkTests
  ;;
*)
  echo -e "\n\n\nExecuting with parameter $1"
  makeCmd "$1"
  ;;
esac
