#!/bin/bash

# Exit on error, undefined variables, and pipe failures
set -euo pipefail

echo "Cleaning..."
rm -f redunce redunce.db
echo "Done."
