#!/bin/bash

# Disable Karate telemetry
export KARATE_TELEMETRY=false

args=(-B test)

# Dependencies are cached in the image; set MVN_ONLINE=true to allow downloads.
if [ "$MVN_ONLINE" != true ]; then
	args+=(-o)
fi

# Filter scenarios by tag, e.g. TAGS="@smoke". Passed as one argument, never
# evaluated by the shell.
if [ -n "$TAGS" ]; then
	args+=("-Dkarate.options=--tags $TAGS")
fi

# Verbose logging profile
if [ "$DEBUG" == true ]; then
	args+=(-Pdebug)
fi

if [ -n "$URLS_TO_BLOCK" ]; then
	echo "URLS_TO_BLOCK is not supported by this launcher and is ignored" >&2
fi

exec mvn "${args[@]}"
