package controller

import (
	"context"
	"k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"secretsbind/internal/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

// BindsReconciler reconciles a Binds object

type NsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=secret.my.domain,resources=binds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=secret.my.domain,resources=binds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=secret.my.domain,resources=binds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Binds object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *NsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("begin Ns resolve", "name", req.Name)
	var ns v1.Namespace
	err := r.Get(context.TODO(), req.NamespacedName, &ns)
	if err != nil && kerror.IsNotFound(err) {
		log.Info("warn", "ns might delete")
		return ctrl.Result{}, nil
	}
	var cm v1.ConfigMapList
	err = r.List(context.TODO(), &cm, client.HasLabels{common.DockerReg})
	if err != nil {
		return ctrl.Result{}, nil
	}

	c := NewCounter(len(cm.Items))
	for _, item := range cm.Items {
		mod := item.DeepCopy()
		mod.Data["status"] = string(NSADD)
		patch := client.MergeFrom(&item)
		err = r.Client.Patch(context.TODO(), mod, patch)

		if err != nil {
			log.Error(err, "cm status patch err at ", "ns", item.Name)
			continue
		}
		c.Add()
	}
	if !c.Result() {
		log.Error(err, "cm status patch not all ")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 2}, nil
	}
	// TODO(user): your logic here
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1.Namespace{}).
		Complete(r)
}

type nsredicate struct{}

func (c nsredicate) Create(createEvent event.CreateEvent) bool {
	//TODO implement me
	return true

}

func (c nsredicate) Delete(deleteEvent event.DeleteEvent) bool {
	//TODO implement me
	return false
}

func (c nsredicate) Update(updateEvent event.UpdateEvent) bool {
	//TODO implement me
	return false
}

func (c nsredicate) Generic(genericEvent event.GenericEvent) bool {
	//TODO implement me
	return false
}
