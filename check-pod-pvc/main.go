package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {

    var kubeconfig *string
    if home := homedir.HomeDir(); home != "" {
        kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
    } else {
        kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
    }
    flag.Parse()

    // use kubeconfig to access k8s
    config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
    if err != nil {
        panic(err.Error())
    }

    // create client to interact with k8s
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err.Error())
    }

    // pvc name and namespace to check
    namespace := os.Getenv("PVC_NAMESPACE")
    if namespace == "" {
        namespace = "default-namespace"
    }

    pvcName := os.Getenv("PVC_NAME")
    if pvcName == "" {
        pvcName = "default-app-pvc"
    }

    currentPodName := os.Getenv("CURRENT_POD_NAME")

    // Check if namespace exists
    _, err = clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
    if err != nil {
        fmt.Printf("Error: Namespace %s does not exist. %v\n", namespace, err)
        os.Exit(1)
    }

    // Check if PVC exists in the namespace
    _, err = clientset.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
    if err != nil {
        fmt.Printf("Error: PVC %s does not exist in namespace %s. %v\n", pvcName, namespace, err)
        os.Exit(1)
    }

    pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
    if err != nil {
        panic(err.Error())
    }

    activePodsFound := false
    fmt.Printf("Checking for pods actively using %s, excluding current pod %s:\n", pvcName, currentPodName)
    for _, pod := range pods.Items {
        if pod.Name == currentPodName {
            continue // skip the current pod
        }

        if pod.Status.Phase != "Running" && pod.Status.Phase != "Pending" {
            continue // skip pods that are not running or pending
        }
        for _, volume := range pod.Spec.Volumes {
            if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == pvcName {
                fmt.Println(pod.Name)
                activePodsFound = true
                break
            }
        }
    }

    if activePodsFound {
        fmt.Println("Error: active pods found using PVC. Exiting to prevent job start...")
        os.Exit(1)
    } else {
        fmt.Println("No active pods found using PVC. Proceeding with job...")
    }

}
