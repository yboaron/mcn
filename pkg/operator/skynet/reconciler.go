/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

Skynet reconciler follows the Submariner controller shape: snapshot status, reconcile child workloads
(reconcileGatewayDaemonSet → reconcileSkynetAgentDeployment), then patch status with conflict requeue.
See: https://github.com/submariner-io/submariner-operator/blob/devel/internal/controllers/submariner/submariner_controller.go
*/

package skynet

import (
	"context"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
	"github.com/aswinsuryana/skynet/pkg/names"
	"github.com/aswinsuryana/skynet/pkg/operator/alloc"
)

// Reconciler watches Skynet CRs and reconciles broker credentials, allocation, status, and agent Deployment.
type Reconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	// AgentImage is used when spec.agent.image is empty (e.g. from RELATED_IMAGE_AGENT on the operator pod).
	AgentImage string
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).V(2).WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)

	var sk skynetv1.Skynet
	if err := r.Client.Get(ctx, req.NamespacedName, &sk); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling Skynet", "ResourceVersion", sk.ResourceVersion)

	initialStatus := sk.Status.DeepCopy()

	if err := reconcileBrokerTokenSecret(ctx, r.Client, r.Scheme, &sk); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "sync broker token secret")
	}

	restCfg, err := r.brokerRESTConfig(ctx, &sk)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "broker rest config")
	}

	brokerDyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "broker dynamic client")
	}

	brokerK8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "broker kubernetes client")
	}

	brokerNS := sk.Spec.BrokerConfig.BrokerNamespace
	if brokerNS == "" {
		brokerNS = "skynet-broker"
	}

	pools, err := alloc.LoadPoolsFromConfigMap(ctx, brokerK8s, brokerNS)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "load broker pool ConfigMap")
	}

	vtep, asn, err := r.computeVtepAndASN(ctx, brokerDyn, brokerNS, pools, &sk, logger)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "compute VTEP/ASN")
	}

	sk.Status.VtepCIDR = vtep
	sk.Status.ASN = asn

	// Same position as submariner_controller Reconcile calling reconcileGatewayDaemonSet (L172).
	agentDep, err := reconcileSkynetAgentDeployment(ctx, r.Client, r.Scheme, &sk, r.agentImage(&sk), pools, logger)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "reconcile skynet-agent Deployment")
	}

	sk.Status.AgentReady = deploymentRolloutComplete(agentDep)
	sk.Status.Phase = skynetv1.SkynetPhaseActive
	sk.Status.ObservedGeneration = sk.Generation

	if !reflect.DeepEqual(sk.Status, *initialStatus) {
		err := r.Client.Status().Update(ctx, &sk)
		if apierrors.IsConflict(err) {
			logger.Info("conflict occurred on status update - requeuing")
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "update Skynet status")
		}
	}

	return ctrl.Result{}, nil
}

func deploymentRolloutComplete(dep *appsv1.Deployment) bool {
	if dep == nil {
		return false
	}
	want := int32(1)
	if dep.Spec.Replicas != nil {
		want = *dep.Spec.Replicas
	}
	if want == 0 {
		return false
	}
	return dep.Status.UpdatedReplicas == want &&
		dep.Status.ReadyReplicas == want &&
		dep.Status.AvailableReplicas == want
}

func (r *Reconciler) brokerRESTConfig(ctx context.Context, sk *skynetv1.Skynet) (*rest.Config, error) {
	ref := sk.Spec.BrokerConfig.TokenSecretRef
	var sec corev1.Secret
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &sec); err != nil {
		return nil, errors.Wrap(err, "get broker token secret")
	}
	token := sec.Data[corev1.ServiceAccountTokenKey]
	if len(token) == 0 {
		token = sec.Data["token"]
	}
	return restConfigForBroker(sk.Spec.BrokerConfig.Server, token, sec.Data["ca.crt"])
}

func (r *Reconciler) computeVtepAndASN(ctx context.Context, broker dynamic.Interface, brokerNS string, pools *alloc.Pools, sk *skynetv1.Skynet, log logr.Logger) (string, int32, error) {
	if sk.Status.VtepCIDR != "" && sk.Status.ASN != 0 {
		return sk.Status.VtepCIDR, sk.Status.ASN, nil
	}

	vAlloc := alloc.NewVtepAllocator(broker, brokerNS, sk.Spec.ClusterID, pools)
	aAlloc := alloc.NewASNAllocator(broker, brokerNS, sk.Spec.ClusterID, pools)
	tmpl := &skynetv1.Cluster{Spec: skynetv1.ClusterSpec{ClusterID: sk.Spec.ClusterID}}

	vtep := sk.Spec.VtepCIDR
	if vtep == "" {
		var err error
		vtep, err = vAlloc.AllocateVtepCIDR(ctx, tmpl)
		if err != nil {
			return "", 0, errors.Wrap(err, "allocate VTEP")
		}
	} else if err := alloc.ValidateVtepCIDR(vtep, pools); err != nil {
		return "", 0, err
	}

	asn := sk.Spec.ASN
	if asn == 0 {
		var err error
		asn, err = aAlloc.AllocateASN(ctx, tmpl)
		if err != nil {
			return "", 0, errors.Wrap(err, "allocate ASN")
		}
	} else if err := alloc.ValidateASN(asn, pools); err != nil {
		return "", 0, err
	}

	if err := ensureBrokerUnique(ctx, broker, brokerNS, sk.Spec.ClusterID, vtep, asn); err != nil {
		return "", 0, err
	}

	log.Info("computed network identifiers", "vtep", vtep, "asn", asn)
	return vtep, asn, nil
}

func ensureBrokerUnique(ctx context.Context, broker dynamic.Interface, brokerNS, clusterID, vtep string, asn int32) error {
	list, err := broker.Resource(skynetv1.ClusterGVR).Namespace(brokerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "list Cluster on broker")
	}

	for i := range list.Items {
		c := &skynetv1.Cluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, c); err != nil {
			continue
		}
		if c.Spec.ClusterID == clusterID {
			continue
		}
		if c.Spec.VtepCIDR == vtep {
			return errors.Errorf("VTEP CIDR %s already used by cluster %s", vtep, c.Spec.ClusterID)
		}
		if c.Spec.ASN == asn {
			return errors.Errorf("ASN %d already used by cluster %s", asn, c.Spec.ClusterID)
		}
	}
	return nil
}

func (r *Reconciler) agentImage(sk *skynetv1.Skynet) string {
	if sk.Spec.Agent != nil && sk.Spec.Agent.Image != "" {
		return sk.Spec.Agent.Image
	}
	if r.AgentImage != "" {
		return r.AgentImage
	}
	return names.DefaultAgentImage
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("skynet-controller").
		For(&skynetv1.Skynet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
