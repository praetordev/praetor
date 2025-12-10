package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/praetordev/praetor/pkg/events"
	"github.com/nats-io/nats.go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Controller struct {
	NatsConn  *nats.Conn
	K8sClient *kubernetes.Clientset
	Namespace string
}

func NewController(natsURL string) (*Controller, error) {
	// 1. Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("nats connect failed: %w", err)
	}

	// 2. Connect to K8s
	// Try in-cluster config first, then fallback to kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		home := os.Getenv("HOME")
		kubeconfig := home + "/.kube/config"
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("k8s config failed: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("k8s clientset failed: %w", err)
	}

	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		ns = "default"
	}

	return &Controller{
		NatsConn:  nc,
		K8sClient: clientset,
		Namespace: ns,
	}, nil
}

func (c *Controller) Start() {
	log.Println("Controller started, listening for job requests...")

	// Subscribe to execution requests
	// QueueSubscribe ensures load balancing if we run multiple controllers (leader election better later)
	c.NatsConn.QueueSubscribe(events.JobRequestSubject, "praetor-controller", func(m *nats.Msg) {
		var req events.ExecutionRequest
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Printf("Failed to unmarshal request: %v", err)
			return
		}
		c.reconcileJob(req)
	})

	select {} // Block forever
}

func (c *Controller) reconcileJob(req events.ExecutionRequest) {
	runID := req.ExecutionRunID.String()
	log.Printf("Reconciling job %d, run %s", req.UnifiedJobID, runID)

	ctx := context.Background()

	// 1. Create Secret with Manifest
	reqBytes, _ := json.Marshal(req)
	secretName := fmt.Sprintf("execution-%s", runID)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		StringData: map[string]string{
			"manifest.json": string(reqBytes),
		},
	}

	_, err := c.K8sClient.CoreV1().Secrets(c.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create secret %s: %v", secretName, err)
		// Should we retry or ack? For now, we log.
		return
	}

	// 2. Create Pod
	podName := fmt.Sprintf("execution-%s", runID)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"app":          "praetor-execution",
				"execution_id": runID,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "executor",
					Image:           "praetor-executor:latest", // Needs to be configurable or same version
					ImagePullPolicy: corev1.PullNever,          // For local Dev/Kind
					Env: []corev1.EnvVar{
						{Name: "PRAETOR_MODE", Value: "oneshot"},
						{Name: "NATS_URL", Value: "nats://praetor-nats:4222"}, // Hardcoded service DNS for now
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "manifest-volume",
							MountPath: "/etc/praetor",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "manifest-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
			},
		},
	}

	_, err = c.K8sClient.CoreV1().Pods(c.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create pod %s: %v", podName, err)
		// Cleanup secret?
		return
	}

	log.Printf("Launched Pod %s for Run %s", podName, runID)
}
