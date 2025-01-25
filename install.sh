#!/bin/bash

# Color definitions
BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'
BOLD='\033[1m'

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to install gum based on OS
install_gum() {
    if command_exists go; then
        echo -e "${BLUE}Installing gum via Go (no elevated privileges needed)${NC}"
        go install github.com/charmbracelet/gum@latest
        return
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "${BLUE}Installing gum via Homebrew (no elevated privileges needed)${NC}"
        brew install gum
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Check for different package managers
        if command_exists nix-env; then
            echo -e "${BLUE}Installing gum via Nix (no elevated privileges needed)${NC}"
            nix-env -iA nixpkgs.gum
        elif command_exists flox; then
            echo -e "${BLUE}Installing gum via Flox (no elevated privileges needed)${NC}"
            flox install gum
        elif command_exists apt; then
            echo -e "${BLUE}Installing gum via apt. Elevated privileges are needed to:${NC}"
            echo "- Create system directory in /etc/apt/keyrings"
            echo "- Write GPG key and repo config to system directories" 
            echo "- Update package lists and install system packages"
            sudo mkdir -p /etc/apt/keyrings
            curl -fsSL https://repo.charm.sh/apt/gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/charm.gpg
            echo "deb [signed-by=/etc/apt/keyrings/charm.gpg] https://repo.charm.sh/apt/ * *" | sudo tee /etc/apt/sources.list.d/charm.list
            sudo apt update && sudo apt install gum
        elif command_exists pacman; then
            echo -e "${BLUE}Installing gum via pacman. Elevated privileges needed:${NC}"
            echo "- Install packages system-wide"
            sudo pacman -S gum
        elif command_exists dnf || command_exists yum; then
            echo -e "${BLUE}Installing gum via dnf/yum. Elevated privileges needed to:${NC}"
            echo "- Configure repository"
            echo "- Import GPG key"
            echo "- Install packages system-wide"
            echo '[charm]\nname=Charm\nbaseurl=https://repo.charm.sh/yum/\nenabled=1\ngpgcheck=1\ngpgkey=https://repo.charm.sh/yum/gpg.key' | sudo tee /etc/yum.repos.d/charm.repo
            sudo rpm --import https://repo.charm.sh/yum/gpg.key
            if command_exists dnf; then
                sudo dnf install gum
            else
                sudo yum install gum
            fi
        elif command_exists zypper; then
            echo -e "${BLUE}Installing gum via zypper. Elevated privileges needed to:${NC}"
            echo "- Refresh repositories"
            echo "- Install packages system-wide"
            sudo zypper refresh
            sudo zypper install gum
        elif command_exists apk; then
            echo -e "${BLUE}Installing gum via apk. Elevated privileges needed to:${NC}"
            echo "- Install packages system-wide"
            sudo apk add gum
        else
            echo "No supported package manager found"
            exit 1
        fi
    elif [[ "$OSTYPE" == "msys"* ]] || [[ "$OSTYPE" == "cygwin"* ]]; then
        # Windows
        if command_exists winget; then
            echo -e "${BLUE}Installing gum via winget (admin privileges handled automatically)${NC}"
            winget install charmbracelet.gum
        elif command_exists scoop; then
            echo -e "${BLUE}Installing gum via scoop (no elevated privileges needed)${NC}"
            scoop install charm-gum
        elif command_exists choco; then
            echo -e "${BLUE}Installing gum via Chocolatey (admin privileges handled automatically)${NC}"
            choco install gum
        else
            echo "Please install a package manager (winget, scoop, or chocolatey) first"
            exit 1
        fi
    else
        echo "Unsupported operating system"
        exit 1
    fi
}

# Check for gum and offer to install it
if ! command_exists gum; then
    echo -e "${BLUE}${BOLD}Gum is not installed. This tool helps create beautiful interactive CLI workflows.${NC}"
    echo -e "${BLUE}Would you like to install gum? (y/n)${NC}"
    read -r install_gum_response
    if [[ $install_gum_response =~ ^[Yy]$ ]]; then
        install_gum
    else
        echo -e "${RED}This script works best with gum. Exiting...${NC}"
        exit 1
    fi
fi

clear

# Welcome message
gum style --border normal --margin "1" --padding "1" --border-foreground 212 "Welcome to the Midaz Installation Script"

# Step 1: Check Dependencies
gum confirm "Would you like to check system dependencies? This will verify if you have all required tools installed." && {
    echo -e "\n${BLUE}${BOLD}Step 1: Checking Dependencies${NC}"
    gum spin --spinner dot --title "Checking dependencies..." -- make help

    # Ask to proceed only if there are no critical errors
    if make help | grep -q "✗"; then
        echo -e "\n${RED}Some dependencies are missing. Please install them before proceeding.${NC}"
        gum confirm "Would you like to continue anyway?" || exit 1
    fi
}

# Step 2: Environment Setup
gum confirm "Would you like to set up environment files? This will create .env files from examples for all services." && {
    echo -e "\n${BLUE}${BOLD}Step 2: Setting Up Environment Files${NC}"
    gum spin --spinner dot --title "Setting up environment files..." -- make set-env
}

# Step 3: Start Services
gum confirm "Would you like to start all services? This will build and start all Docker containers." && {
    echo -e "\n${BLUE}${BOLD}Step 3: Starting Services${NC}"
    gum spin --spinner dot --title "Starting services..." -- make up

    # Wait for services to be ready
    echo "Waiting for services to be ready..."
    sleep 10
}

# Step 4: Build MDZ CLI
gum confirm "Would you like to build and install the MDZ CLI locally? This will require sudo privileges." && {
    echo -e "\n${BLUE}${BOLD}Step 4: Building MDZ CLI${NC}"
    gum spin --spinner dot --title "Building and installing MDZ CLI..." -- make mdz-build
}

# Step 5: Final Status Check
echo -e "\n${BLUE}${BOLD}Step 5: Checking Final Status${NC}"
gum spin --spinner dot --title "Checking service status..." -- make status

# Installation Complete
gum style \
	--border normal \
	--margin "1" \
	--padding "1" \
	--border-foreground 212 \
	"Installation Complete! 🎉" \
	"" \
	"You can use 'make help' to see available commands." \
	"Use 'make status' to check services status at any time."

# Optional: Show MDZ CLI help if it was installed
if command_exists mdz; then
    gum confirm "Would you like to see the MDZ CLI help?" && {
        mdz --help
    }
fi 