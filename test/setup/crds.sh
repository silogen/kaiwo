#!/bin/bash

set -e

make generate
make manifests
make install