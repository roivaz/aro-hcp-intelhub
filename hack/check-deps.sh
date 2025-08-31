#!/bin/bash

# System dependency checker for ARO-HCP Embedder
# This script verifies that all required system dependencies are installed

set -e

echo "Checking system dependencies..."

# Check for pg_config (PostgreSQL development headers)
echo -n "Checking for pg_config... "
if command -v pg_config >/dev/null 2>&1; then
    echo "✓ found"
else
    echo "✗ missing"
    echo ""
    echo "ERROR: pg_config not found. PostgreSQL development headers are required."
    echo ""
    echo "To install on Ubuntu/Debian:"
    echo "  sudo apt-get install postgresql-server-dev-all libpq-dev"
    echo ""
    echo "To install on RHEL/Fedora/CentOS:"
    echo "  sudo dnf install postgresql-devel libpq-devel"
    echo "  # or: sudo yum install postgresql-devel libpq-devel"
    echo ""
    echo "To install on macOS:"
    echo "  brew install postgresql"
    echo ""
    exit 1
fi

# Check for git
echo -n "Checking for git... "
if command -v git >/dev/null 2>&1; then
    echo "✓ found"
else
    echo "✗ missing"
    echo "ERROR: git is required for repository operations"
    exit 1
fi

# Check for Python development headers
echo -n "Checking for Python development headers... "
if python3-config --cflags >/dev/null 2>&1; then
    echo "✓ found"
else
    echo "✗ missing"
    echo ""
    echo "ERROR: Python development headers not found."
    echo ""
    echo "To install on Ubuntu/Debian:"
    echo "  sudo apt-get install python3-dev"
    echo ""
    echo "To install on RHEL/Fedora/CentOS:"
    echo "  sudo dnf install python3-devel"
    echo "  # or: sudo yum install python3-devel"
    echo ""
    echo "To install on macOS:"
    echo "  # Usually included with Python from brew or python.org"
    echo ""
    exit 1
fi

# Check for build tools
echo -n "Checking for build tools... "
if command -v gcc >/dev/null 2>&1 && command -v g++ >/dev/null 2>&1 && command -v make >/dev/null 2>&1; then
    echo "✓ found"
else
    echo "✗ missing"
    echo ""
    echo "ERROR: Build tools (gcc, g++, make) are required."
    echo ""
    echo "To install on Ubuntu/Debian:"
    echo "  sudo apt-get install build-essential"
    echo ""
    echo "To install on RHEL/Fedora/CentOS:"
    echo "  sudo dnf groupinstall 'Development Tools'"
    echo "  # or: sudo yum groupinstall 'Development Tools'"
    echo ""
    echo "To install on macOS:"
    echo "  xcode-select --install"
    echo ""
    exit 1
fi

echo "All system dependencies are satisfied!"
