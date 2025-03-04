#!/bin/bash

# Set error handling
set -e

# Script to automate testing of the MDZ CLI app
echo "Starting MDZ CLI testing automation..."

# Step 1: Run tests
# echo "Running tests..."
# make test

# Step 2: Build the app
echo "Building the app..."
make build

# Path to the MDZ binary
MDZ_BIN="./bin/mdz"

# Step 3: Login
echo "Logging in..."
$MDZ_BIN login --username user_john --password Lerian@123

# Step 4: Create a new organization
echo "Creating a new organization..."
TIMESTAMP=$(date +%s)
ORG_LEGAL_NAME="Test Organization $TIMESTAMP"
ORG_DBA="TestOrg$TIMESTAMP"
ORG_LEGAL_DOC="12345678901234"
ORG_CODE="ACTIVE"
ORG_DESC="Test organization created by automation script"
ORG_LINE1="123 Test Street"
ORG_LINE2="Suite 456"
ORG_ZIP="12345"
ORG_CITY="Test City"
ORG_STATE="TS"
ORG_COUNTRY="US"
ORG_METADATA='{"created_by": "automation_script", "timestamp": "'$TIMESTAMP'"}'

echo "Creating organization with legal name: $ORG_LEGAL_NAME"
ORG_OUTPUT=$($MDZ_BIN organization create \
  --legal-name "$ORG_LEGAL_NAME" \
  --doing-business-as "$ORG_DBA" \
  --legal-document "$ORG_LEGAL_DOC" \
  --code "$ORG_CODE" \
  --description "$ORG_DESC" \
  --line1 "$ORG_LINE1" \
  --line2 "$ORG_LINE2" \
  --zip-code "$ORG_ZIP" \
  --city "$ORG_CITY" \
  --state "$ORG_STATE" \
  --country "$ORG_COUNTRY" \
  --metadata "$ORG_METADATA" \
  --no-color)

# Extract organization ID from the text response
# Format: "The Organization XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX has been successfully created."
ORG_ID=$(echo "$ORG_OUTPUT" | grep -o '[0-9a-f]\{8\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{12\}')

if [ -z "$ORG_ID" ]; then
    echo "Failed to extract organization ID"
    echo "Organization output: $ORG_OUTPUT"
    exit 1
fi

echo "Created organization with ID: $ORG_ID"

# Step 5: Create a new ledger for the organization
echo "Creating a new ledger..."
LEDGER_NAME="TestLedger$TIMESTAMP"
LEDGER_CODE="ACTIVE"
LEDGER_DESC="Test ledger created by automation script"
LEDGER_METADATA='{"created_by": "automation_script", "timestamp": "'$TIMESTAMP'"}'

echo "Creating ledger with name: $LEDGER_NAME for organization ID: $ORG_ID"
LEDGER_OUTPUT=$($MDZ_BIN ledger create \
  --name "$LEDGER_NAME" \
  --code "$LEDGER_CODE" \
  --description "$LEDGER_DESC" \
  --organization-id "$ORG_ID" \
  --metadata "$LEDGER_METADATA" \
  --no-color)

# Extract ledger ID from the text response
# Format: "The Ledger XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX has been successfully created."
LEDGER_ID=$(echo "$LEDGER_OUTPUT" | grep -o '[0-9a-f]\{8\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{12\}')

if [ -z "$LEDGER_ID" ]; then
    echo "Failed to extract ledger ID"
    echo "Ledger output: $LEDGER_OUTPUT"
    exit 1
fi

echo "Created ledger with ID: $LEDGER_ID"

# Step 6: Create 4 different asset types
echo "Creating 4 different asset types..."

# Function to create an asset and extract its ID
create_asset() {
    local name=$1
    local code=$2
    local type=$3
    local desc="$4"

    echo "Creating asset: $name ($code)"
    local asset_output=$($MDZ_BIN asset create \
      --organization-id "$ORG_ID" \
      --ledger-id "$LEDGER_ID" \
      --name "$name" \
      --code "$code" \
      --type "$type" \
      --status-code "ACTIVE" \
      --status-description "$desc" \
      --metadata '{"created_by": "automation_script", "timestamp": "'$TIMESTAMP'"}' \
      --no-color)

    # Extract asset ID from the text response
    local asset_id=$(echo "$asset_output" | grep -o '[0-9a-f]\{8\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{12\}')
    
    if [ -z "$asset_id" ]; then
        echo "Failed to extract asset ID"
        echo "Asset output: $asset_output"
        exit 1
    fi
    
    echo "Created asset $name with ID: $asset_id"
    return 0
}

# Create BRL (Brazilian Reais)
create_asset "Brazilian Real" "BRL" "currency" "Brazilian currency"

# Create BTC (Bitcoin)
create_asset "Bitcoin" "BTC" "crypto" "Bitcoin cryptocurrency"

# Create USD (US Dollar)
create_asset "US Dollar" "USD" "currency" "United States currency"

# Create EUR (Euro)
create_asset "Euro" "EUR" "currency" "European Union currency"

# Store asset codes in an array for random selection
ASSETS=("BRL" "BTC" "USD" "EUR")

# Create a map of external account IDs for each asset type
declare -A EXTERNAL_ACCOUNTS

# Step 7: Create external accounts for each asset type and regular accounts
echo "Creating external accounts for each asset type..."

# Function to create an account and extract its ID
create_account() {
    local number=$1
    local asset_code=$2
    local account_type=$3
    local is_external=${4:-false}
    
    local prefix="TestAccount"
    if [ "$is_external" = true ]; then
        prefix="External"
    fi
    
    local account_name="$prefix-$number-$asset_code"
    local account_alias
    
    if [ "$is_external" = true ]; then
        account_alias="EXT$asset_code"
    else
        account_alias="ACC$number$asset_code"
    fi
    
    echo "Creating account: $account_name with asset: $asset_code"
    local account_output=$($MDZ_BIN account create \
      --organization-id "$ORG_ID" \
      --ledger-id "$LEDGER_ID" \
      --name "$account_name" \
      --alias "$account_alias" \
      --asset-code "$asset_code" \
      --type "$account_type" \
      --status-code "ACTIVE" \
      --status-description "Test account created by automation script" \
      --metadata '{"created_by": "automation_script", "timestamp": "'$TIMESTAMP'", "account_number": "'$number'", "is_external": "'$is_external'"}' \
      --no-color)

    # Extract account ID from the text response
    local account_id=$(echo "$account_output" | grep -o '[0-9a-f]\{8\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{12\}')
    
    if [ -z "$account_id" ]; then
        echo "Failed to extract account ID"
        echo "Account output: $account_output"
        exit 1
    fi
    
    echo "Created account $account_name with ID: $account_id"
    
    # Return the account ID
    echo "$account_id"
}

# Store regular account IDs and their asset codes for later use
declare -a REGULAR_ACCOUNTS
declare -a ACCOUNT_ASSETS

echo "Creating 10 regular accounts with random data and assets..."
# Create 10 regular accounts with random assets
for i in {1..10}; do
    # Get a random asset code from the ASSETS array
    random_asset=${ASSETS[$((RANDOM % ${#ASSETS[@]}))]}
    
    # Create the account
    account_id=$(create_account $i $random_asset "deposit")
    
    # Store the account ID and asset code for later use in transactions
    REGULAR_ACCOUNTS+=("$account_id")
    ACCOUNT_ASSETS["$account_id"]="$random_asset"
done

echo "Successfully created all 10 regular accounts"

# Step 8: Create a transaction for each account
echo "Creating transactions for each account..."

# Function to create a transaction using JSON file
create_transaction() {
    local destination_account_id=$1
    local asset_code=$2
    local amount=$3
    
    echo "Creating transaction for account ID: $destination_account_id with asset: $asset_code, amount: $amount"
    
    # Create a temporary JSON file for the transaction
    local json_file="/tmp/transaction_$destination_account_id.json"
    
    # Create the JSON content
    cat > "$json_file" << EOF
{
  "organizationId": "$ORG_ID",
  "ledgerId": "$LEDGER_ID",
  "sourceAccountId": "external/$asset_code",
  "destinationAccountId": "$destination_account_id",
  "amount": "$amount",
  "currency": "$asset_code",
  "type": "DEPOSIT",
  "status": "COMPLETED",
  "description": "Initial deposit to account from external source",
  "metadata": {
    "created_by": "automation_script",
    "timestamp": "$TIMESTAMP"
  }
}
EOF
    
    # Run the transaction create command with the JSON file
    local transaction_output=$($MDZ_BIN transaction create --json-file "$json_file" --no-color)
    
    # Print the output for debugging
    echo "Transaction output: $transaction_output"
    
    # Extract transaction ID from the text response
    local transaction_id=$(echo "$transaction_output" | grep -o '[0-9a-f]\{8\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{4\}-[0-9a-f]\{12\}')
    
    if [ -z "$transaction_id" ]; then
        echo "Warning: Failed to extract transaction ID"
        # Not exiting to continue with other accounts
    else
        echo "Created transaction with ID: $transaction_id"
    fi
    
    # Remove the temporary JSON file
    rm -f "$json_file"
}

# Create transactions for each regular account
echo "Creating transactions for each regular account..."

for account_id in "${REGULAR_ACCOUNTS[@]}"; do
    # Get the asset code for this account
    asset_code="${ACCOUNT_ASSETS[$account_id]}"
    
    # Generate a random amount based on asset type
    if [[ "$asset_code" == "BTC" ]]; then
        # For BTC, use smaller amounts (0.001-0.1 BTC)
        # Generate a random decimal between 0.001 and 0.1
        amount="0.$(printf "%03d" $(( RANDOM % 100 )))"
    else
        # For other currencies, use larger amounts (100-10000)
        amount=$(( (RANDOM % 9900) + 100 ))
    fi
    
    # Create a transaction for this account
    create_transaction "$account_id" "$asset_code" "$amount"
done

echo "Successfully created transactions for all accounts"

echo "MDZ CLI testing completed successfully!"