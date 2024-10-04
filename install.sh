#!/bin/sh

set -e

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux) OS='linux' ;;
  darwin) OS='darwin' ;;
  *) echo "Unsupported operating system: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH='amd64' ;;
  aarch64|arm64) ARCH='arm64' ;;
  armv7l) ARCH='arm' ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Set download URL
DOWNLOAD_URL="https://github.com/c4pt0r/tip/releases/download/nightly/tip-${OS}-${ARCH}"

# Set installation directory
INSTALL_DIR="$HOME/.tip/bin"

# Set config file path
CONFIG_FILE="$HOME/.tip/config.toml"

# Function to check if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# Check if curl or wget is available
if command_exists curl; then
  DOWNLOAD_CMD="curl -fsSL -o"
elif command_exists wget; then
  DOWNLOAD_CMD="wget -q -O"
else
  echo "Error: Neither curl nor wget is available. Please install one of them and try again." >&2
  exit 1
fi

# Download the binary
echo "Downloading tip..."
$DOWNLOAD_CMD /tmp/tip "$DOWNLOAD_URL"

# Make the binary executable
chmod +x /tmp/tip

# Move the binary to the installation directory
echo "Installing tip to $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR"

if [ -w "$INSTALL_DIR" ]; then
  mv /tmp/tip "$INSTALL_DIR/tip"
else
  sudo mv /tmp/tip "$INSTALL_DIR/tip"
fi

# Create config file if it doesn't exist
if [ ! -f "$CONFIG_FILE" ]; then
  echo "Creating config file at $CONFIG_FILE..."
  cat > "$CONFIG_FILE" << EOF
# tip Configuration File

# Example configuration options:
# host = "localhost"
# port = "4000"
# user = "root"
# password = ""
# database = "test"

# Add your custom configuration here
EOF
  echo "Config file created. Please edit $CONFIG_FILE with your TiDB connection details."
else
  echo "Config file already exists at $CONFIG_FILE. Skipping creation."
fi

echo "tip has been successfully installed!"
echo "You can now use it by running 'tip' from the command line."
echo "Make sure to edit your config file at $CONFIG_FILE with your TiDB connection details."
echo "You can also add it to your PATH by running 'export PATH=\$PATH:$INSTALL_DIR'"
