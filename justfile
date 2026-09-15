# incus-caddy-config development commands
#
# Quick start:
#   just run <args>     # Run caddy-config
#   just test-local     # Run local unit tests
#   just fix            # Auto-fix linting issues

set dotenv-load
set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

import 'just/mod.just'
import 'just/test.just'
import 'just/build.just'

[private]
default:
    @just --list
