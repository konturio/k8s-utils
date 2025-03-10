package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var startupTime time.Time

// JobReconciler monitors job completion and restarts the target deployment
type JobReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	MonitoredNamespace   string
	TargetDeploymentName string
	JobNameRegex         *regexp.Regexp
}

func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var job batchv1.Job
	// Get the job that triggered the reconciliation
	if err := r.Get(ctx, req.NamespacedName, &job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the job has already been processed
	if job.Annotations != nil {
		if processed, ok := job.Annotations["job-watcher/processed"]; ok && processed == "true" {
			log.Info("Job already processed, ignoring", "job", req.NamespacedName)
			return ctrl.Result{}, nil
		}
	}

	if !r.JobNameRegex.MatchString(job.GetName()) {
		log.Info("Job name does not match pattern, ignoring", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	jobCompleted := false
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			jobCompleted = true
			break
		}
	}
	if !jobCompleted {
		return ctrl.Result{}, nil
	}

	if job.CreationTimestamp.Time.Before(startupTime) {
		log.Info("Ignoring job created before controller startup", "job", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Get the target deployment from the specified namespace
	var deployment appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: r.TargetDeploymentName, Namespace: r.MonitoredNamespace}, &deployment); err != nil {
		log.Error(err, "Failed to get target deployment", "deployment", r.TargetDeploymentName, "namespace", r.MonitoredNamespace)
		return ctrl.Result{}, err
	}

	// Patch the annotation to trigger a rollout restart
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`,
		time.Now().Format(time.RFC3339)))
	if err := r.Patch(ctx, &deployment, client.RawPatch(types.MergePatchType, patch)); err != nil {
		log.Error(err, "Failed to patch target deployment", "deployment", r.TargetDeploymentName, "namespace", r.MonitoredNamespace)
		return ctrl.Result{}, err
	}

	// Adding an annotation to the completed job
	jobPatch := []byte(`{"metadata":{"annotations":{"job-watcher/processed":"true"}}}`)
	if err := r.Patch(ctx, &job, client.RawPatch(types.MergePatchType, jobPatch)); err != nil {
		log.Error(err, "Failed to mark job as processed", "job", req.NamespacedName)
		return ctrl.Result{}, err
	}

	log.Info("Target deployment restarted due to job completion", "deployment", r.TargetDeploymentName, "namespace", r.MonitoredNamespace)
	return ctrl.Result{}, nil
}

// SetupWithManager configures the controller with the manager and filters events (job: namespace + pattern)
func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
	WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetNamespace() == r.MonitoredNamespace && r.JobNameRegex.MatchString(obj.GetName())
	})).
		Complete(r)
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	startupTime = time.Now()

	monitoredNamespace := os.Getenv("MONITORED_NAMESPACE")
	if monitoredNamespace == "" {
		monitoredNamespace = "dev-namespace"
	}
	targetDeploymentName := os.Getenv("TARGET_DEPLOYMENT_NAME")
	if targetDeploymentName == "" {
		targetDeploymentName = "dev-deployment"
	}
	jobNamePattern := os.Getenv("JOB_NAME_PATTERN")
	if jobNamePattern == "" {
		jobNamePattern = "^dev-job-.+$"
	}
	compiledRegex, err := regexp.Compile(jobNamePattern)
	if err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Invalid JOB_NAME_PATTERN: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to add batchv1 to scheme: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to add appsv1 to scheme: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to add corev1 to scheme: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}

	// Create manager with health probe on port 9440
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: ":9440",
	})
	if err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to start manager: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}

	// Initialize the controller with settings from env variables
	if err = (&JobReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		MonitoredNamespace:   monitoredNamespace,
		TargetDeploymentName: targetDeploymentName,
		JobNameRegex:         compiledRegex,
	}).SetupWithManager(mgr); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to create controller: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to set health check: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Unable to set ready check: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}

	fmt.Println("Starting manager..")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Error occurred while running the manager: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}
