# check-pod-pvc

The `check-pod-pvc` utility is designed to check for active pods in a Kubernetes cluster that are using a specified Persistent Volume Claim (PVC). This is particularly useful in scenarios where it's necessary to ensure that a PVC is not in use before performing operations that could affect the data or availability of the PVC.

## Getting Started

To get started with `check-pod-pvc`, you'll need:

- `kubectl` installed and configured
- Access to a Kubernetes cluster
- A kubeconfig file (usually located at `~/.kube/config`)

## Build

```bash
go build -o check-pod-pvc main.go
```

## Usage

```bash
export PVC_NAMESPACE=default-namespace
export PVC_NAME=default-app-pvc
./check-pod-pvc
```