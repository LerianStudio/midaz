#!/bin/bash

# Function to print step headers
print_step() {
    echo -e "\n--------------------------------------------------"
    echo -e "--- Step $1: $2"
    echo -e "--------------------------------------------------"
}

# Function to print error messages
print_error() {
    echo -e "ERROR: $1"
    echo -e "Suggestion: $2"
}

# Function to print warning messages
print_warning() {
    echo -e "WARNING: $1"
    echo -e "Suggestion: $2"
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to ask for confirmation
confirm_step() {
    echo -e "\nThis step will: $1"
    echo -n "Do you want to proceed? [y/N] "
    read -r response
    if [[ ! $response =~ ^[Yy]$ ]]; then
        echo -e "\nInstallation cancelled by user. Exiting..."
        exit 0
    fi
}

# Clear screen
clear 2>/dev/null || cls 2>/dev/null

# Installation Process Start
cat pkg/shell/logo.txt
echo -e "\n=============================================="
echo -e "           MIDAZ INSTALLATION WIZARD           "
echo -e "==============================================\n"

# Check OS type
OS="$(uname -s)"
echo "System Detection:"
case "${OS}" in
    Linux*)     
        if [[ "$(uname -r)" == *microsoft* ]]; then
            echo "--- Running on Windows Subsystem for Linux (WSL)"
        else
            echo "--- Running on Linux"
        fi
        ;;
    Darwin*)    
        echo "--- Running on macOS"
        ;;
    CYGWIN*|MINGW*|MSYS*)
        echo "--- Running on Windows"
        ;;
    *)
        echo "ERROR: Unsupported operating system: ${OS}"
        echo "Please contact Lerian's support team on GitHub if you need help with this."
        exit 1
        ;;
esac

# Check if git is installed
if ! command_exists git; then
    print_error "Git is not installed" "Please install git first to proceed with the installation"
    exit 1
fi

# Repository setup and update checks
if [ ! -d ".git" ] || ! git remote -v 2>/dev/null | grep -q "lerianstudio/midaz"; then
    print_step "0" "Repository Setup"
    
    DEFAULT_DIR="$HOME/Downloads/midaz"
    
    echo "Default installation directory: ${DEFAULT_DIR}"
    echo -n "Would you like to use a different directory? [y/N] "
    read -r change_dir
    
    if [[ $change_dir =~ ^[Yy]$ ]]; then
        echo -n "Please enter the full path for installation: "
        read -r INSTALL_DIR
    else
        INSTALL_DIR="$DEFAULT_DIR"
    fi
    
    # Create and setup repository
    mkdir -p "$INSTALL_DIR"
    echo "Cloning Midaz repository into: ${INSTALL_DIR}"
    
    if ! git clone https://github.com/lerianstudio/midaz.git "$INSTALL_DIR"; then
        print_error "Failed to clone repository" "Check your internet connection and try again"
        rm -rf "$INSTALL_DIR"
        exit 1
    fi
    
    cd "$INSTALL_DIR"
    echo "Repository cloned successfully"
    echo "Running installation from the cloned repository..."
    
    if [ -f "./install.sh" ]; then
        chmod +x ./install.sh
        exec ./install.sh
    else
        print_error "Installation script not found in repository" "Please check if the repository structure is correct"
        rm -rf "$INSTALL_DIR"
        exit 1
    fi
    exit 1
else
    print_step "0" "Repository Update Check"
    echo "Checking if repository is up to date..."
    git remote update >/dev/null 2>&1
    
    LOCAL=$(git rev-parse @)
    REMOTE=$(git rev-parse @{u})
    
    if [ "$LOCAL" != "$REMOTE" ]; then
        print_warning "Repository is not up to date" "Installer can update the repository automatically or you can proceed with the installation"
        echo -n "Would you like to update automatically? [Y/n] "
        read -r update_repo
        
        if [[ ! $update_repo =~ ^[Nn]$ ]]; then
            if ! git pull origin main; then
                print_error "Failed to update repository" "Please update manually or check your internet connection"
                exit 1
            fi
            echo "Repository updated successfully"
            exec ./install.sh
        else
            echo "Continuing with current version..."
        fi
    else
        echo "Repository is up to date"
    fi
fi

# Installation Process
# -------------------

print_step "1" "System Dependencies Check"
confirm_step "Check your system for required dependencies using 'make check-dependencies'"
echo "Checking system dependencies..."
if ! make check-dependencies; then
    print_warning "Missing dependencies detected" "Most of these dependencies are required to run local 'make' tuning commands. However, if you already have Docker installed, you can continue with the installation using Docker containers."
    echo -n "Continue anyway, installing using Docker containers? [y/N] "
    read -r response
    [[ ! $response =~ ^[Yy]$ ]] && exit 1
fi

print_step "2" "Environment Configuration"
confirm_step "Set up environment files using 'make set-env'"
echo "Setting up environment files..."
if ! make set-env; then
    print_error "Environment setup failed" "Check file permissions and try again"
    exit 1
fi

print_step "3" "Services Initialization"
confirm_step "Start all required services using Docker"
echo "Starting services..."
if ! make up; then
    print_error "Service startup failed" "Check Docker status and logs"
    exit 1
fi
echo "Waiting for services to initialize..."
sleep 10

print_step "4" "MDZ CLI Installation"
echo "You can install the Midaz CLI locally or run it directly from the binary folder"
echo "The binary is already built and available at: components/mdz/bin/mdz"
echo -n "Would you like to install MDZ CLI locally? This will require elevated permissions. [y/N] "
read -r response
if [[ $response =~ ^[Yy]$ ]]; then
    echo "Installing MDZ CLI..."
    if ! make mdz-build; then
        print_error "MDZ CLI installation failed" "Check build logs for details"
        echo "You can still use MDZ CLI from: components/mdz/bin/mdz"
    fi
else
    echo "Skipping MDZ CLI installation. You can find the binary at: components/mdz/bin/mdz"
fi

print_step "5" "Final Status Check"
confirm_step "Verify the installation status of all services"
echo "Verifying installation..."
make status

# Installation Complete
echo -e "\n================================================"
echo -e "            INSTALLATION COMPLETE! 🎉             "
echo -e "================================================"

echo -e "\nAvailable commands:"
echo "-------------------"
echo "  make help    - Show all available commands"
echo "  make status  - Check services status"

# Show MDZ CLI help if installed
if command_exists mdz; then
    echo -n "Would you like to view MDZ CLI help? [y/N] "
    read -r response
    [[ $response =~ ^[Yy]$ ]] && mdz --help
fi