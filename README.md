# Protohackers

These are my solutions to [protohackers](https://protohackers.com/),
a series of network programming challenges.
Currently, they're all written in Go with no external dependencies.

Ultimately, these solutions are tested more rigorously against the actual Protohackers server,
but most include some local tests that can be run with `go test .`.
See README.md of each solution for details.

## Deploying
I'm currently taking a simple approach to deploying.
Each solution has a symlink to `bin/deploy.sh`,
which provisions a droplet on Digital Ocean via `doctl`,
builds a Go binary,
deploys it to the droplet,
and tears the droplet down on Ctrl+C.

The script deploys to Digital Ocean's `lon1` datacenter,
which means that (as of June 2023) you'll be co-located with the Protohackers test server.
So if you hit a timeout in a test,
you can be fairly confident that it's not due to network latency.

Requirements:

* Go and `doctl` installed
* `doctl` authenticated to your DigitalOcean account
* An SSH key added to your DigitalOcean account
* Replace the SSH `KEY` and `FINGERPRINT` variables at the top of `bin/deploy.sh` with your own

Then, you can run `./deploy.sh` from the solution's directory, e.g. `cd 5; ./deploy.sh`.
