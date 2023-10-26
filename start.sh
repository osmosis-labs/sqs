#!/bin/bash
# This script is used for executing the process at different rates.
# When you are developing locally and running the docker-compose you need to sleep
# If you don't you will have to start the process like 3 times before it will connect to redis.
# This takes the debug variable from pipeline and determines if it needs to be used for the kubernetes deployments.
# The docker-compose automatically sets it to debug.
# One thing to add is to make the code load its debug value from this as well.
if [ "${DEBUG}" == "true" ]; then
  echo "keep node alive on failure for debugging" >> status
  echo "sleep for 30 seconds to let other services start"
  sleep 30
  /bin/sqsd || echo "failed to start" && tail -f status
else
  /bin/sqsd
fi