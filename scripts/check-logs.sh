#!/bin/bash

# Script to verify proper error logging in usecases

# ===== Configuration =====
# Define directories to check
COMPONENTS=("./components/mdz" "./components/onboarding" "./components/transaction")
SDK_DIRS=("./sdks/go/midaz")

# Define patterns to search for
MISSING_LOG_PATTERN="return err"
PROPER_LOG_PATTERN="log.*Error.*return err"

# ===== Header =====
echo "===== Error Logging Verification ====="

# ===== Component Check =====
echo ""
echo "Checking Components:"
COMPONENT_ISSUES=0

for component in "${COMPONENTS[@]}"; do
    component_name=$(basename "$component")
    echo "- $component_name"
    
    # Skip if no Go files
    if ! find "$component" -name "*.go" -type f | grep -q .; then
        echo "  ℹ️ No Go files found, skipping"
        continue
    fi
    
    # Find all service files that might contain usecases
    SERVICE_FILES=$(find "$component" -path "*/services/*" -name "*.go" | grep -v "_test.go")
    COMPONENT_FILE_ISSUES=0
    
    for file in $SERVICE_FILES; do
        # Use awk to analyze the file more thoroughly
        MISSING_LOGS=$(awk '
            BEGIN { issues = 0; line_num = 0; in_func = 0; has_log = 0; }
            
            # Track line numbers
            { line_num++ }
            
            # Check for function start
            /func/ { in_func = 1; has_log = 0; }
            
            # Check for error logging statements
            /logger\.Error/ || /log\.Error/ || /logger\.Errorf/ || /log\.Errorf/ { 
                has_log = 1; 
                last_log_line = line_num;
            }
            
            # Check for return err statements
            /return err/ { 
                # If we have not seen a logging statement in the last 5 lines, flag it
                if (has_log == 0 || (line_num - last_log_line > 5)) {
                    printf("%d:		%s\n", line_num, $0);
                    issues++;
                }
            }
            
            # Reset when we leave a function
            /^}/ { 
                if (in_func == 1) {
                    in_func = 0; 
                    has_log = 0;
                }
            }
            
            END { exit issues }
        ' "$file")
        
        EXIT_CODE=$?
        
        if [ $EXIT_CODE -ne 0 ]; then
            relative_path=${file#./}
            echo "  ❌ $relative_path:"
            echo "$MISSING_LOGS" | while read -r line; do
                LINE_NUM=$(echo "$line" | cut -d':' -f1)
                CODE=$(echo "$line" | cut -d':' -f2-)
                echo "    Line $LINE_NUM: $CODE"
            done
            COMPONENT_FILE_ISSUES=$((COMPONENT_FILE_ISSUES + 1))
        fi
    done
    
    if [ $COMPONENT_FILE_ISSUES -eq 0 ]; then
        echo "  ✅ No issues found"
    else
        echo "  ⚠️ Found issues in $COMPONENT_FILE_ISSUES files"
        COMPONENT_ISSUES=$((COMPONENT_ISSUES + COMPONENT_FILE_ISSUES))
    fi
done

# ===== SDK Check =====
echo ""
echo "Checking Go SDK:"
SDK_ISSUES=0

for sdk_dir in "${SDK_DIRS[@]}"; do
    # Skip if no Go files or directory doesn't exist
    if [ ! -d "$sdk_dir" ] || ! find "$sdk_dir" -name "*.go" -type f | grep -q .; then
        echo "  ℹ️ No Go files found in SDK, skipping"
        continue
    fi
    
    # Find all service files that might contain error handling
    SDK_FILES=$(find "$sdk_dir" -name "*.go" | grep -v "_test.go" | grep -v "/internal/")
    SDK_FILE_ISSUES=0
    
    for file in $SDK_FILES; do
        # Use awk to analyze the file more thoroughly
        MISSING_LOGS=$(awk '
            BEGIN { issues = 0; line_num = 0; in_func = 0; has_log = 0; }
            
            # Track line numbers
            { line_num++ }
            
            # Check for function start
            /func/ { in_func = 1; has_log = 0; }
            
            # Check for error logging statements
            /logger\.Error/ || /log\.Error/ || /logger\.Errorf/ || /log\.Errorf/ || /fmt\.Errorf/ || /errors\./ { 
                has_log = 1; 
                last_log_line = line_num;
            }
            
            # Check for return err statements
            /return err/ || /return nil, err/ { 
                # If we have not seen a logging statement in the last 5 lines, flag it
                if (has_log == 0 || (line_num - last_log_line > 5)) {
                    printf("%d:		%s\n", line_num, $0);
                    issues++;
                }
            }
            
            # Reset when we leave a function
            /^}/ { 
                if (in_func == 1) {
                    in_func = 0; 
                    has_log = 0;
                }
            }
            
            END { exit issues }
        ' "$file")
        
        EXIT_CODE=$?
        
        if [ $EXIT_CODE -ne 0 ]; then
            relative_path=${file#./}
            echo "  ❌ $relative_path:"
            echo "$MISSING_LOGS" | while read -r line; do
                LINE_NUM=$(echo "$line" | cut -d':' -f1)
                CODE=$(echo "$line" | cut -d':' -f2-)
                echo "    Line $LINE_NUM: $CODE"
            done
            SDK_FILE_ISSUES=$((SDK_FILE_ISSUES + 1))
        fi
    done
    
    if [ $SDK_FILE_ISSUES -eq 0 ]; then
        echo "  ✅ No issues found"
    else
        echo "  ⚠️ Found issues in $SDK_FILE_ISSUES files"
        SDK_ISSUES=$((SDK_ISSUES + SDK_FILE_ISSUES))
    fi
done

# ===== Summary =====
echo ""
echo "===== Summary ====="
TOTAL_ISSUES=$((COMPONENT_ISSUES + SDK_ISSUES))

if [ $TOTAL_ISSUES -eq 0 ]; then
    echo "✅ No error logging issues found across all code"
else
    echo "⚠️ Found $TOTAL_ISSUES potential error logging issues:"
    echo "  - Components: $COMPONENT_ISSUES issues"
    echo "  - Go SDK: $SDK_ISSUES issues"
    echo ""
    echo "➡️ Consider adding proper error logging before returning errors"
fi

exit 0
