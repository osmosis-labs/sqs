#!/bin/bash
# This script is used for executing the process at different rates.
# When you are developing locally and running the docker-compose you need to sleep
# If you don't you will have to start the process like 3 times before it will connect to redis.
# This takes the debug variable from pipeline and determines if it needs to be used for the kubernetes deployments.
# The docker-compose automatically sets it to debug.
# One thing to add is to make the code load its debug value from this as well.
# When its Kubernetes deployment you can't mount directly to osmosis dir so we have to copy if the file and directory exist.
echo "Copy the config from the mount point to the osmosis application directory."
# cp /osmosis-mount/config.json /osmosis/config.json || echo "not kubernetes deployment"
if [ "${DEBUG}" == "true" ]; then
  echo "look at contents of the osmosis directory."
  ls -lah /osmosis/
  echo "Log the config for debugging"
  cat /osmosis/config.json
  echo "keep node alive on failure for debugging" >> status
  echo "sleep for 30 seconds to let other services start"
  sleep 30
  echo "start process with debug and keep container running for troubleshooting purposes."
  /bin/sqsd || echo "failed to start" && tail -f status
else
  /bin/sqsd
fi