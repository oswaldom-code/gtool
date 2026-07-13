#!/bin/bash

# Disable Karate telemetry
export KARATE_TELEMETRY=false

COMMAND="mvn -B test"

# Filter scenarios by tag, e.g. TAGS="@smoke"
if [ "$TAGS" != "" ]; then
	COMMAND+=" -Dkarate.options=\"--tags $TAGS\""
fi

# Verbose logging profile
if [ "$DEBUG" == true ]; then
	COMMAND+=" -Pdebug"
fi

eval $COMMAND

exit $?
