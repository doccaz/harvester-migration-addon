// forklift_migrations.go
package main

import (
	"context"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// CreateForkliftMigrationHandler creates a Migration CR to start executing a Forklift Plan
func CreateForkliftMigrationHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Creating Migration for Forklift Plan %s/%s", namespace, name)

		migration := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "forklift.konveyor.io/v1beta1",
				"kind":       "Migration",
				"metadata": map[string]interface{}{
					"name":      name + "-migration",
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"plan": map[string]interface{}{
						"name":      name,
						"namespace": namespace,
					},
				},
			},
		}

		createdObj, err := clients.Dynamic.Resource(forkliftMigrationGVR).Namespace(namespace).Create(context.TODO(), migration, metav1.CreateOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create Forklift Migration: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, createdObj)
	}
}

// DeleteForkliftMigrationHandler deletes an existing Migration CR for a Plan
func DeleteForkliftMigrationHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		migrationName := name + "-migration"
		log.Infof("Deleting Forklift Migration %s/%s", namespace, migrationName)

		err := clients.Dynamic.Resource(forkliftMigrationGVR).Namespace(namespace).Delete(context.TODO(), migrationName, metav1.DeleteOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to delete Forklift Migration: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Migration deleted"})
	}
}

// GetForkliftMigrationStatus returns the status of Migrations for a Plan
func GetForkliftMigrationStatus(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		// List all migrations in the namespace
		list, err := clients.Dynamic.Resource(forkliftMigrationGVR).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to list Forklift Migrations: "+err.Error())
			return
		}

		// Find the migration for this plan (prefer the most recent one)
		var latestMigration map[string]interface{}
		var latestTime string
		for _, item := range list.Items {
			planName, _ := getNestedStringOrWarn(item.Object, "spec", "plan", "name")
			if planName == name {
				created := item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z")
				if created > latestTime {
					latestTime = created
					obj := item.Object
					latestMigration = obj
				}
			}
		}
		if latestMigration != nil {
			httpx.RespondWithJSON(w, http.StatusOK, latestMigration)
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]interface{}{"message": "No migration found for this plan"})
	}
}
