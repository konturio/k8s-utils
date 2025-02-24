# Job Watcher Controller

This is a Go controller that monitors Kubernetes Jobs and triggers a rollout restart of a target Deployment when a Job completes successfully. It is built using the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) library.

## Features

- Monitors Jobs in a specified namespace.
- Checks that a Job has completed successfully (using the `JobComplete` condition).
- Ignores Jobs created before the controller started.
- Triggers a rollout restart on a target Deployment by patching its pod template annotation.
- Configurable via environment variables:
    - `MONITORED_NAMESPACE` (default: `dev-namespace`)
    - `TARGET_DEPLOYMENT_NAME` (default: `dev-deployment`)
    - `JOB_NAME_PATTERN` (default: `dev-job`)

## Prerequisites

- Go v1.24

## Building

1. **Clone the repository** and navigate to its directory:

   ```bash
   go build -ldflags="-w -s" -o job-watcher-controller
