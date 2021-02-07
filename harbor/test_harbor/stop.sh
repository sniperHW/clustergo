#!/bin/sh
ps aux | grep teacher | grep -v 'grep' | grep -v 'Unity' | awk '{print $2}' | xargs kill -9
