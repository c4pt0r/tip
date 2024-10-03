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
DOWNLOAD_URL="https://github.com/c4pt0r/tidbcli/releases/download/nightly/tidb-cli-${OS}-${ARCH}"

# Set installation directory
INSTALL_DIR="$HOME/.tidbcli/bin"

# Set config file path
CONFIG_FILE="$HOME/.tidbcli/config.toml"

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
echo "Downloading TiDB CLI..."
$DOWNLOAD_CMD /tmp/tidb-cli "$DOWNLOAD_URL"

# Make the binary executable
chmod +x /tmp/tidb-cli

# Move the binary to the installation directory
echo "Installing TiDB CLI to $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR"

if [ -w "$INSTALL_DIR" ]; then
  mv /tmp/tidb-cli "$INSTALL_DIR/tidb-cli"
else
  sudo mv /tmp/tidb-cli "$INSTALL_DIR/tidb-cli"
fi

# Create config file if it doesn't exist
if [ ! -f "$CONFIG_FILE" ]; then
  echo "Creating config file at $CONFIG_FILE..."
  cat > "$CONFIG_FILE" << EOF
# TiDB CLI Configuration File

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

echo "TiDB CLI has been successfully installed!"
echo "You can now use it by running 'tidb-cli' from the command line."
echo "Make sure to edit your config file at $CONFIG_FILE with your TiDB connection details."
echo "You can also add it to your PATH by running 'export PATH=\$PATH:$INSTALL_DIR'"
