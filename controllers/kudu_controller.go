/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bigdatav1alpha1 "github.com/kubernetesbigdataeg/kudu-operator/api/v1alpha1"
)

const kuduFinalizer = "bigdata.kubernetesbigdataeg.org/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableKudu represents the status of the Deployment reconciliation
	typeAvailableKudu = "Available"
	// typeDegradedKudu represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedKudu = "Degraded"
)

// KuduReconciler reconciles a Kudu object
type KuduReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=kudus,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=kudus/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=kudus/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps;services,verbs=get;list;create;watch
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *KuduReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	//
	// 1. Control-loop: checking if kudu CR exists
	//
	// Fetch the Kudu instance
	// The purpose is check if the Custom Resource for the Kind Kudu
	// is applied on the cluster if not we return nil to stop the reconciliation
	kudu := &bigdatav1alpha1.Kudu{}
	err := r.Get(ctx, req.NamespacedName, kudu)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("kudu resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get kudu")
		return ctrl.Result{}, err
	}

	//
	// 2. Control-loop: Status to Unknown
	//
	// Let's just set the status as Unknown when no status are available
	if kudu.Status.Conditions == nil || len(kudu.Status.Conditions) == 0 {
		meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeAvailableKudu, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, kudu); err != nil {
			log.Error(err, "Failed to update Kudu status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the kudu Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, kudu); err != nil {
			log.Error(err, "Failed to re-fetch kudu")
			return ctrl.Result{}, err
		}
	}

	//
	// 3. Control-loop: Let's add a finalizer
	//
	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(kudu, kuduFinalizer) {
		log.Info("Adding Finalizer for Kudu")
		if ok := controllerutil.AddFinalizer(kudu, kuduFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, kudu); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	//
	// 4. Control-loop: Instance marked for deletion
	//
	// Check if the Kudu instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isKuduMarkedToBeDeleted := kudu.GetDeletionTimestamp() != nil
	if isKuduMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(kudu, kuduFinalizer) {
			log.Info("Performing Finalizer Operations for Kudu before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeDegradedKudu,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", kudu.Name)})

			if err := r.Status().Update(ctx, kudu); err != nil {
				log.Error(err, "Failed to update Kudu status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForKudu(kudu)

			// TODO(user): If you add operations to the doFinalizerOperationsForKudu method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the kudu Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, kudu); err != nil {
				log.Error(err, "Failed to re-fetch kudu")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeDegradedKudu,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", kudu.Name)})

			if err := r.Status().Update(ctx, kudu); err != nil {
				log.Error(err, "Failed to update Kudu status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for Kudu after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(kudu, kuduFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for Kudu")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, kudu); err != nil {
				log.Error(err, "Failed to remove finalizer for Kudu")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	//
	// 5. Control-loop: Let's deploy/ensure our managed resources for kudu
	// - ConfigMap,
	// - Service ClusterIP,
	// - Service ClusterIP NodePort,
	// - Service ClusterIP Tserver,
	// - StatefulSet Master
	// - StatefulSet Tserver
	//

	// Crea o actualiza ConfigMap
	configMapFound := &corev1.ConfigMap{}
	if err := r.ensureResource(ctx, kudu, r.defaultConfigMapForKudu, configMapFound, "kudu-config", "ConfigMap"); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	svcMasterFound := &corev1.Service{}
	if err := r.ensureResource(ctx, kudu, r.serviceMasterForKudu, svcMasterFound, "kudu-master-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	svcMasterUiFound := &corev1.Service{}
	if err := r.ensureResource(ctx, kudu, r.serviceUiForKudu, svcMasterUiFound, "kudu-ui-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	svcTserverFound := &corev1.Service{}
	if err := r.ensureResource(ctx, kudu, r.serviceTserverForKudu, svcTserverFound, "kudu-tserver-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// StatefulSet
	stsMasterFound := &appsv1.StatefulSet{}
	if err := r.ensureResource(ctx, kudu, r.statefulSetMasterForKudu, stsMasterFound, "kudu-master", "StatefulSet"); err != nil {
		return ctrl.Result{}, err
	}

	// StatefulSet
	stsTserverFound := &appsv1.StatefulSet{}
	if err := r.ensureResource(ctx, kudu, r.statefulSetTserverForKudu, stsTserverFound, "kudu-tserver", "StatefulSet"); err != nil {
		return ctrl.Result{}, err
	}

	//
	// 6. Control-loop: Check the number of replicas
	//
	// The CRD API is defining that the Kudu type, have a KuduSpec.Size field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	masterSize := kudu.Spec.MasterSize
	if stsMasterFound.Spec.Replicas == nil {
		log.Error(nil, "Spec is not initialized for Master StatefulSet", "StatefulSet.Namespace", stsMasterFound.Namespace, "StatefulSet.Name", stsMasterFound.Name)
		return ctrl.Result{}, fmt.Errorf("spec is not initialized for StatefulSet %s/%s", stsMasterFound.Namespace, stsMasterFound.Name)
	}
	if *stsMasterFound.Spec.Replicas != masterSize {
		stsMasterFound.Spec.Replicas = &masterSize
		if err = r.Update(ctx, stsMasterFound); err != nil {
			log.Error(err, "Failed to update StatefulSet",
				"StatefulSet.Namespace", stsMasterFound.Namespace, "StatefulSet.Name", stsMasterFound.Name)

			// Re-fetch the kudu Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, kudu); err != nil {
				log.Error(err, "Failed to re-fetch kudu")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeAvailableKudu,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", kudu.Name, err)})

			if err := r.Status().Update(ctx, kudu); err != nil {
				log.Error(err, "Failed to update Kudu status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	tserverSize := kudu.Spec.TserverSize
	if stsTserverFound.Spec.Replicas == nil {
		log.Error(nil, "Spec is not initialized for Tserver StatefulSet", "StatefulSet.Namespace", stsTserverFound.Namespace, "StatefulSet.Name", stsTserverFound.Name)
		return ctrl.Result{}, fmt.Errorf("spec is not initialized for StatefulSet %s/%s", stsTserverFound.Namespace, stsTserverFound.Name)
	}
	if *stsTserverFound.Spec.Replicas != tserverSize {
		stsTserverFound.Spec.Replicas = &tserverSize
		if err = r.Update(ctx, stsTserverFound); err != nil {
			log.Error(err, "Failed to update StatefulSet",
				"StatefulSet.Namespace", stsTserverFound.Namespace, "StatefulSet.Name", stsTserverFound.Name)

			if err := r.Get(ctx, req.NamespacedName, kudu); err != nil {
				log.Error(err, "Failed to re-fetch kudu")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeAvailableKudu,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", kudu.Name, err)})

			if err := r.Status().Update(ctx, kudu); err != nil {
				log.Error(err, "Failed to update Kudu status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	//
	// 7. Control-loop: Let's update the status
	//
	// The following implementation will update the status
	meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeAvailableKudu,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", kudu.Name, masterSize)})

	if err := r.Status().Update(ctx, kudu); err != nil {
		log.Error(err, "Failed to update Kudu status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// finalizeKudu will perform the required operations before delete the CR.
func (r *KuduReconciler) doFinalizerOperationsForKudu(cr *bigdatav1alpha1.Kudu) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *KuduReconciler) defaultConfigMapForKudu(Kudu *bigdatav1alpha1.Kudu, resourceName string) (client.Object, error) {

	configMapData := make(map[string]string, 0)
	kuduEnv := `
	export KUDU__kudumaster__master_addresses=kudu-master-0.kudu-master-svc.default.svc.cluster.local:7051,kudu-master-1.kudu-master-svc.default.svc.cluster.local:7051,kudu-master-2.kudu-master-svc.default.svc.cluster.local:7051
    export KUDU__kudumaster__webserver_doc_root=/opt/kudu/usr/local/www
    export KUDU__kudumaster__logtostderr=false
    export KUDU__kudumaster__log_dir=/var/log/kudu
    export KUDU__kudumaster__fs_wal_dir=/var/lib/kudu/wal
    export KUDU__kudumaster__fs_data_dirs=/var/lib/kudu/data
    export KUDU__kudumaster__rpc_encryption=optional
    export KUDU__kudumaster__rpc_authentication=optional
    export KUDU__kudumaster__rpc_negotiation_timeout_ms=5000
    export KUDU__kudumaster__hive_metastore_uris=thrift://hive-svc.default.svc.cluster.local:9083
    export KUDU__kudutserver__tserver_master_addrs=kudu-master-0.kudu-master-svc.default.svc.cluster.local:7051,kudu-master-1.kudu-master-svc.default.svc.cluster.local:7051,kudu-master-2.kudu-master-svc.default.svc.cluster.local:7051
    export KUDU__kudutserver__webserver_doc_root=/opt/kudu/usr/local/www
    export KUDU__kudutserver__logtostderr=false
    export KUDU__kudutserver__log_dir=/var/log/kudu
    export KUDU__kudutserver__fs_wal_dir=/var/lib/kudu/wal
    export KUDU__kudutserver__fs_data_dirs=/var/lib/kudu/data
    export KUDU__kudutserver__rpc_encryption=optional
    export KUDU__kudutserver__rpc_authentication=optional
    export KUDU__kudutserver__rpc_negotiation_timeout_ms=5000
	`

	configMapData["kudu.env"] = kuduEnv
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Kudu.Namespace,
		},
		Data: configMapData,
	}

	if err := ctrl.SetControllerReference(Kudu, configMap, r.Scheme); err != nil {
		return nil, err
	}

	return configMap, nil
}

func (r *KuduReconciler) serviceMasterForKudu(Kudu *bigdatav1alpha1.Kudu, resourceName string) (client.Object, error) {

	labels := labelsForKudu(Kudu.Name, "kudu-master")
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Kudu.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "ui",
					Port: 8051,
				},
				{
					Name: "rpc-port",
					Port: 7051,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(Kudu, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

func (r *KuduReconciler) serviceUiForKudu(Kudu *bigdatav1alpha1.Kudu, resourceName string) (client.Object, error) {

	labels := labelsForKudu(Kudu.Name, "kudu-master")
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Kudu.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:     "kudud-ui",
				Port:     8051,
				NodePort: 30204,
			}},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	if err := ctrl.SetControllerReference(Kudu, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

func (r *KuduReconciler) serviceTserverForKudu(Kudu *bigdatav1alpha1.Kudu, resourceName string) (client.Object, error) {

	labels := labelsForKudu(Kudu.Name, "kudu-tserver")
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Kudu.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "ui",
					Port: 8051,
				},
				{
					Name: "rpc-port",
					Port: 7051,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(Kudu, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

// statefulSetForKudu returns a Kudu StatefulSet object
func (r *KuduReconciler) statefulSetMasterForKudu(kudu *bigdatav1alpha1.Kudu, resourceName string) (client.Object, error) {

	labels := labelsForKudu(kudu.Name, "kudu-master")

	replicas := kudu.Spec.MasterSize

	fastdisks := "fast-disks"

	// Get the Operand image
	image, err := imageForKudu()
	if err != nil {
		return nil, err
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: kudu.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "kudu-master-svc",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          "RollingUpdate",
				RollingUpdate: nil,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:           image,
							Name:            "kudu",
							ImagePullPolicy: corev1.PullAlways,
							Args:            []string{"master"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "master-ui",
									ContainerPort: 8051,
								},
								{
									Name:          "master-rpc",
									ContainerPort: 7051,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "GET_HOSTS_FROM",
									Value: "dns",
								},
								{
									Name: "POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/var/lib/kudu/",
								},
								{
									Name:      "kudu-config-volume",
									MountPath: "/etc/environments",
								},
								{
									Name:      "kudu-logs",
									MountPath: "/tmp/",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "kudu-master-logs",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/kudu/kudu-master.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kudu-logs",
									MountPath: "/tmp/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "kudu-config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "kudu-config",
									},
								},
							},
						},
						{
							Name: "kudu-logs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "datadir",
					Labels: labels,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("150Mi"),
						},
					},
					StorageClassName: &fastdisks,
				},
			}},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(kudu, sts, r.Scheme); err != nil {
		return nil, err
	}
	return sts, nil
}

func (r *KuduReconciler) statefulSetTserverForKudu(kudu *bigdatav1alpha1.Kudu, resourceName string) (client.Object, error) {

	labels := labelsForKudu(kudu.Name, "kudu-tserver")

	replicas := kudu.Spec.TserverSize

	fastdisks := "fast-disks"

	// Get the Operand image
	image, err := imageForKudu()
	if err != nil {
		return nil, err
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: kudu.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "kudu-tserver-svc",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          "RollingUpdate",
				RollingUpdate: nil,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:           image,
							Name:            "kudu",
							ImagePullPolicy: corev1.PullAlways,
							Args:            []string{"tserver"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "tserver-ui",
									ContainerPort: 8051,
								},
								{
									Name:          "tserver-rpc",
									ContainerPort: 7051,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "GET_HOSTS_FROM",
									Value: "dns",
								},
								{
									Name: "POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "datadir",
									MountPath: "/var/lib/kudu/",
								},
								{
									Name:      "kudu-config-volume",
									MountPath: "/etc/environments",
								},
								{
									Name:      "kudu-logs",
									MountPath: "/tmp/",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "kudu-tserver-logs",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/kudu/kudu-tserver.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kudu-logs",
									MountPath: "/tmp/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "kudu-config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "kudu-config",
									},
								},
							},
						},
						{
							Name: "kudu-logs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "datadir",
					Labels: labels,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("150Mi"),
						},
					},
					StorageClassName: &fastdisks,
				},
			}},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(kudu, sts, r.Scheme); err != nil {
		return nil, err
	}
	return sts, nil
}

// labelsForKudu returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForKudu(name string, app string) map[string]string {
	var imageTag string
	image, err := imageForKudu()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{
		"app.kubernetes.io/name":       "Kudu",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "kudu-operator",
		"app.kubernetes.io/created-by": "controller-manager",
		"app":                          app,
	}
}

// imageForKudu gets the Operand image which is managed by this controller
// from the KUDU_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForKudu() (string, error) {
	/*var imageEnvVar = "KUDU_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("unable to find %s environment variable with the image", imageEnvVar)
	}*/
	image := "docker.io/kubernetesbigdataeg/kudu:1.17.0-2"
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *KuduReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bigdatav1alpha1.Kudu{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *KuduReconciler) ensureResource(ctx context.Context, kudu *bigdatav1alpha1.Kudu, createResourceFunc func(*bigdatav1alpha1.Kudu, string) (client.Object, error), foundResource client.Object, resourceName string, resourceType string) error {
	log := log.FromContext(ctx)
	err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: kudu.Namespace}, foundResource)
	if err != nil && apierrors.IsNotFound(err) {
		resource, err := createResourceFunc(kudu, resourceName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to define new %s resource for Kudu", resourceType))

			// The following implementation will update the status
			meta.SetStatusCondition(&kudu.Status.Conditions, metav1.Condition{Type: typeAvailableKudu,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create %s for the custom resource (%s): (%s)", resourceType, kudu.Name, err)})

			if err := r.Status().Update(ctx, kudu); err != nil {
				log.Error(err, "Failed to update Kudu status")
				return err
			}

			return err
		}

		log.Info(fmt.Sprintf("Creating a new %s", resourceType),
			fmt.Sprintf("%s.Namespace", resourceType), resource.GetNamespace(), fmt.Sprintf("%s.Name", resourceType), resource.GetName())

		if err = r.Create(ctx, resource); err != nil {
			log.Error(err, fmt.Sprintf("Failed to create new %s", resourceType),
				fmt.Sprintf("%s.Namespace", resourceType), resource.GetNamespace(), fmt.Sprintf("%s.Name", resourceType), resource.GetName())
			return err
		}

		time.Sleep(5 * time.Second)

		if err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: kudu.Namespace}, foundResource); err != nil {
			log.Error(err, fmt.Sprintf("Failed to get newly created %s", resourceType))
			return err
		}

	} else if err != nil {
		log.Error(err, fmt.Sprintf("Failed to get %s", resourceType))
		return err
	}

	return nil
}
