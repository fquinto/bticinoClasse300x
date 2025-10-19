#!/usr/bin/env bash

set -e

if ! command -v python3 &>/dev/null; then
	echo "python3 is not installed and is required. Install it with the package manager of your choice."
	exit 1
fi

typeset -r THIS="$(realpath "$0")"
typeset -r THIS_DIR="${THIS%/*}"
typeset -r THIS_NAME="${THIS##*/}"

cd "${THIS_DIR}"

# Set up local Python virtual environment if user hasn't one active
if [ -z "$VIRTUAL_ENV" ]; then
	[ -f .venv/pyvenv.cfg ] || python -m venv --system-site-packages .venv
	source .venv/bin/activate
fi

pip install --upgrade pip
pip install -r requirements.txt
python main.py
