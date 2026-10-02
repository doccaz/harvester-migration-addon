// forklift_logs.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// HandleGetForkliftLogs fetches logs from Forklift controller pods and migration worker pods.
// Forklift labels worker pods with "plan-name" = <planName> and "forklift.app" = virt-v2v | consumer | virt-v2v-inspection.
// Worker pods run in the plan's targetNamespace; hooks run in the plan namespace.
// Controller pods use structured JSON logging with "plan", "migration", "vm" fields.
func HandleGetForkliftLogs(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		planNamespace := vars["namespace"]
		planName := vars["name"]

		forkliftNs := r.URL.Query().Get("forkliftNamespace")
		if forkliftNs == "" {
			forkliftNs = planNamespace
		}
		showAll := r.URL.Query().Get("all") == "true"
		errorsOnly := r.URL.Query().Get("errors") == "true"

		log.Debugf("Fetching Forklift logs for plan %s/%s (forklift ns: %s)", planNamespace, planName, forkliftNs)

		// Get the plan to find target namespace and VM IDs/names
		var targetNamespace string
		var vmIDs []string
		planObj, err := clients.Dynamic.Resource(kube.ForkliftPlanGVR).Namespace(planNamespace).Get(context.TODO(), planName, metav1.GetOptions{})
		if err == nil {
			targetNamespace, _ = kube.NestedStringOrWarn(planObj.Object, "spec", "targetNamespace")
			vms, _, vmsErr := unstructured.NestedSlice(planObj.Object, "spec", "vms")
			if vmsErr != nil {
				log.Warnf("Error reading spec.vms: %v", vmsErr)
			}
			for _, vm := range vms {
				if vmMap, ok := vm.(map[string]interface{}); ok {
					if id, ok := vmMap["id"].(string); ok && id != "" {
						vmIDs = append(vmIDs, id)
					}
				}
			}
		}

		// Related resource names that appear in controller logs
		migrationName := planName + "-migration"
		networkMapName := planName + "-network-map"
		storageMapName := planName + "-storage-map"
		controllerMatchTerms := []string{planName, migrationName, networkMapName, storageMapName}

		var logOutput strings.Builder

		isErrorLine := func(line string) bool {
			l := strings.ToLower(line)
			return strings.Contains(l, "error") || strings.Contains(l, "fail") ||
				strings.Contains(l, "warn") || strings.Contains(l, "critical") ||
				strings.Contains(l, "\"level\":\"error\"") || strings.Contains(l, "\"level\":\"warn\"")
		}

		// fetchAndWriteLogs streams logs from a pod/container, optionally filtering.
		// If filterTerms is nil, all lines are included (worker pods are already plan-specific).
		fetchAndWriteLogs := func(ns string, pod v1.Pod, filterTerms []string, header string) {
			for _, cs := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
				req := clients.Clientset.CoreV1().Pods(ns).GetLogs(pod.Name, &v1.PodLogOptions{
					Container: cs.Name,
				})
				stream, err := req.Stream(context.TODO())
				if err != nil {
					// Skip containers that can't be read (not started, etc.)
					continue
				}

				headerWritten := false
				scanner := bufio.NewScanner(stream)
				buf := make([]byte, 0, 64*1024)
				scanner.Buffer(buf, 1024*1024)

				for scanner.Scan() {
					line := scanner.Text()
					include := showAll

					if !include && filterTerms != nil {
						// Controller pod: only include lines mentioning our plan/resources
						for _, term := range filterTerms {
							if strings.Contains(line, term) {
								include = true
								break
							}
						}
					} else if filterTerms == nil {
						// Worker pod: include everything
						include = true
					}

					if include && errorsOnly && !showAll {
						include = isErrorLine(line)
					}

					if include {
						if !headerWritten && header != "" {
							logOutput.WriteString(header)
							headerWritten = true
						}
						logOutput.WriteString(line + "\n")
					}
				}
				stream.Close()
			}
		}

		// ── 1. Forklift controller pod (filtered by plan name) ──
		// The controller pod is labeled app=forklift-controller in the forklift namespace
		controllerPods, _ := clients.Clientset.CoreV1().Pods(forkliftNs).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=forklift-controller",
		})
		if controllerPods == nil || len(controllerPods.Items) == 0 {
			// Fallback: any pod with "forklift-controller" in the name
			allPods, _ := clients.Clientset.CoreV1().Pods(forkliftNs).List(context.TODO(), metav1.ListOptions{})
			if allPods != nil {
				for _, p := range allPods.Items {
					if strings.Contains(p.Name, "forklift-controller") {
						if controllerPods == nil {
							controllerPods = &v1.PodList{}
						}
						controllerPods.Items = append(controllerPods.Items, p)
					}
				}
			}
		}
		if controllerPods != nil {
			for _, pod := range controllerPods.Items {
				fetchAndWriteLogs(forkliftNs, pod, controllerMatchTerms,
					fmt.Sprintf("\n=== Forklift Controller: %s ===\n", pod.Name))
			}
		}

		// ── 2. Migration worker pods (label: plan-name=<planName>) ──
		// These run in targetNamespace and include virt-v2v, virt-v2v-inspection, consumer pods.
		// Forklift labels them with plan-name=<planName>.
		searchNamespaces := []string{}
		if targetNamespace != "" {
			searchNamespaces = append(searchNamespaces, targetNamespace)
		}
		if planNamespace != targetNamespace {
			searchNamespaces = append(searchNamespaces, planNamespace)
		}

		for _, ns := range searchNamespaces {
			// Direct label query — most efficient
			workerPods, err := clients.Clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "plan-name=" + planName,
			})
			if err == nil {
				for _, pod := range workerPods.Items {
					appLabel := pod.Labels["forklift.app"]
					podType := "Worker"
					switch appLabel {
					case "virt-v2v":
						podType = "virt-v2v Conversion"
					case "virt-v2v-inspection":
						podType = "virt-v2v Inspection"
					case "consumer":
						podType = "Consumer"
					}
					vmID := pod.Labels["vmID"]
					vmInfo := ""
					if vmID != "" {
						vmInfo = fmt.Sprintf(" (VM: %s)", vmID)
					}
					fetchAndWriteLogs(ns, pod, nil,
						fmt.Sprintf("\n=== %s: %s/%s%s ===\n", podType, ns, pod.Name, vmInfo))
				}
			}

			// Also look for populator pods (created by CDI, name prefix "populate-")
			// and pods whose name starts with the plan name (hook jobs, converter jobs)
			allPods, err := clients.Clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
			if err == nil {
				seen := map[string]bool{}
				if workerPods != nil {
					for _, p := range workerPods.Items {
						seen[p.Name] = true
					}
				}
				for _, pod := range allPods.Items {
					if seen[pod.Name] {
						continue
					}
					isRelevant := false
					podType := "Related Pod"

					// Check if pod name starts with planName (hook jobs, converter jobs)
					if strings.HasPrefix(pod.Name, planName+"-") {
						isRelevant = true
						podType = "Plan Pod"
					}

					// Check for populator pods by looking at migration label
					if !isRelevant && strings.HasPrefix(pod.Name, "populate-") {
						if _, hasMigLabel := pod.Labels["migration"]; hasMigLabel {
							// Check if this populator's migration label matches our plan's migration
							isRelevant = true
							podType = "CDI Populator"
						}
					}

					// Check for converter jobs
					if !isRelevant && strings.HasPrefix(pod.Name, "convert-") {
						for _, vmID := range vmIDs {
							if strings.Contains(pod.Name, vmID) {
								isRelevant = true
								podType = "Disk Converter"
								break
							}
						}
					}

					if isRelevant {
						fetchAndWriteLogs(ns, pod, nil,
							fmt.Sprintf("\n=== %s: %s/%s ===\n", podType, ns, pod.Name))
					}
				}
			}
		}

		if logOutput.Len() == 0 {
			fmt.Fprintf(&logOutput, "No matching log entries found for plan '%s'.\n\n", planName)
			logOutput.WriteString("Troubleshooting tips:\n")
			logOutput.WriteString("  1. Disable 'Only relevant' to see all Forklift controller logs\n")
			if targetNamespace != "" {
				fmt.Fprintf(&logOutput, "  2. Check worker pods: kubectl get pods -n %s -l plan-name=%s\n", targetNamespace, planName)
				fmt.Fprintf(&logOutput, "  3. Check populator pods: kubectl get pods -n %s | grep populate-\n", targetNamespace)
			}
			fmt.Fprintf(&logOutput, "  4. Controller logs: kubectl logs -n %s -l app=forklift-controller | grep %s\n", forkliftNs, planName)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(logOutput.String())); err != nil {
			log.Warnf("Failed to write response: %v", err)
		}
	}
}
