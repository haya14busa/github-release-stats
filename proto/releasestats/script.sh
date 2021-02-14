#!/bin/bash
cd "${0%/*}" # cd to script dir.
docker build -t releasestats/build .
docker run -v `pwd`:/workdir releasestats/build
