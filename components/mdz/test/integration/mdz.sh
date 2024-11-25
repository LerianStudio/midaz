#!/bin/bash

# Setting environment variables (if necessary)
MDZ_CMD="mdz"
USERNAME="user_john"
PASSWORD="Lerian@123"
LEGAL_NAME="Soul LLCT"
DOING_BUSINESS_AS="The ledger.io"
LEGAL_DOCUMENT="48784548000104"
STATUS_CODE="ACTIVE"
DESCRIPTION="Test Ledger"
LINE1="Av Santso"
LINE2="VJ 222"
ZIP_CODE="04696040"
CITY="West"
STATE="VJ"
COUNTRY="MG"
METADATA='{"chave1": "valor1", "chave2": 2, "chave3": true}'

# Function to execute commands and capture the output
run_command() {
    echo "Executando: $1"
    OUTPUT=$($1)
    echo "$OUTPUT"
}

login_output=$(run_command "$MDZ_CMD login --username $USERNAME --password $PASSWORD")
echo "$login_output"

create_output=$(run_command "$MDZ_CMD organization create --legal-name $LEGAL_NAME --doing-business-as $DOING_BUSINESS_AS --legal-document $LEGAL_DOCUMENT --code $STATUS_CODE --description $DESCRIPTION --line1 $LINE1 --line2 $LINE2 --zip-code $ZIP_CODE --city $CITY --state $STATE --country $COUNTRY --metadata $METADATA")
echo "$create_output"

list_output=$(run_command "$MDZ_CMD organization list")
echo "$list_output"

ORG_ID=$(echo "$list_output" | grep -oP '[0-9a-fA-F-]{36}' | head -n 1)
if [ -z "$ORG_ID" ]; then
  echo "Erro: No ID found!"
  exit 1
fi
echo "organization id: $ORG_ID"

describe_output=$(run_command "$MDZ_CMD organization describe --organization-id $ORG_ID")
echo "$describe_output"

update_output=$(run_command "$MDZ_CMD organization update --organization-id $ORG_ID --legal-name 'Updated Name' --doing-business-as 'Updated Business' --country 'BR'")
echo "$update_output"

echo "Test completed!"
