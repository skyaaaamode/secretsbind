package controller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"secretsbind/internal/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

var (
	dockerSecName = ".dockerconfigjson"
	status        = "status"
	secrets       = "secrets"
)

// BindsReconciler reconciles a Binds object
type CmReconciler struct {
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
func (r *CmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("begin Cm resolve", "name", req.Name)

	var err error
	var curCm v1.ConfigMap
	// TODO(user): your logic here
	secName := SecretNameSplit(req.Name)

	// 没查询到/或者检测到删除操作，删除所有ns下的secrets
	if err := r.Get(ctx, req.NamespacedName, &curCm); err != nil || curCm.DeletionTimestamp != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		log.Info("begin delete all ns")
		err := r.DeleteAllSecretsNameInNs(secName)
		if err != nil {
			log.Error(err, "unable to fetch ConfigMap,RequeueAfter 2 sec")
			return ctrl.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if _, ok := curCm.Data[secrets]; !ok {
		log.Error(err, "ConfigMap is Empty")
		return ctrl.Result{}, nil
	}

	if _, err := base64.StdEncoding.DecodeString(curCm.Data[secrets]); err != nil {
		log.Error(err, "ConfigMap is Not valid")
		return ctrl.Result{}, nil
	}

	// 新增

	switch curCm.Data[status] {
	case string(INITIAL):
		err = r.bindAllNs(secName, curCm.Data[secrets])
	case string(NSADD):
		err = r.bindAllNs(secName, curCm.Data[secrets])
	case string(UPDATE):
		err = r.UpdateAllNsOnly(secName, curCm.Data[secrets])
	case string(Failed):
		err = r.UpdateAllNs(secName, curCm.Data[secrets])
	case string(ALLOCATED):
		return ctrl.Result{}, nil
	}

	if err != nil {
		err = r.PatchUpdateCmStatus(&curCm, Failed)
		if err != nil {
			log.Error(err, "ConfigMap status update err")
		}
	} else {
		err = r.PatchUpdateCmStatus(&curCm, ALLOCATED)
		if err != nil {
			log.Error(err, "ConfigMap status update err")
		}
	}

	return ctrl.Result{}, nil
}

func (r *CmReconciler) PatchUpdateCmStatus(s *v1.ConfigMap, d DockerRegistrySetStatus) error {

	mod := s.DeepCopy()
	mod.Data[status] = string(d)
	patch := client.MergeFrom(s)
	// 执行 Patch 操作
	return r.Patch(context.Background(), mod, patch)
}

func (r *CmReconciler) PatchSecrets(s *v1.Secret, secrets string) error {

	mod := s.DeepCopy()
	de, _ := base64.StdEncoding.DecodeString(secrets)
	mod.Data[dockerSecName] = de
	patch := client.MergeFrom(s)
	// 执行 Patch 操作
	return r.Patch(context.Background(), mod, patch)
}

// 只全量创建，不全量更新
func (r *CmReconciler) bindAllNs(name, secrets string) error {

	var list v1.NamespaceList
	if err := r.List(context.TODO(), &list); err != nil {
		return errors.New(err.Error())
	}
	msg := make([]string, 0)
	c := NewCounter(len(list.Items))
	for _, ns := range list.Items {
		var set v1.Secret
		err := r.Get(context.TODO(), types.NamespacedName{
			Name:      name,
			Namespace: ns.Name,
		}, &set)
		if err == nil {
			c.Add()
			continue
		}
		sec := buildDockerRegistrySecrets(ns.Name, name, secrets)

		err = r.Create(context.TODO(), sec)
		if err != nil {
			msg = append(msg, buildArrayErrMsg(err, ns.Name, sec.Name))
			continue
		}
		c.Add()
	}
	if !c.Result() {
		return errors.New(strings.Join(msg, "\n"))
	}
	return nil
}
func buildArrayErrMsg(er error, target string, index string, others ...string) string {
	return fmt.Sprintf("err occur at %s of %s,err msg is :%s,others:%s", index, target, er.Error(), others)

}
func buildDockerRegistrySecrets(ns, name, data string) *v1.Secret {

	decoded, _ := base64.StdEncoding.DecodeString(data)

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				common.DockerReg: "1",
			},
		},
		Data: map[string][]byte{
			dockerSecName: decoded,
		},
	}

}

// 只更新，不创建
func (r *CmReconciler) UpdateAllNsOnly(name, data string) error {
	var list v1.NamespaceList
	if err := r.List(context.TODO(), &list); err != nil {
		return errors.New(err.Error())
	}
	c := new(Counter)
	msg := make([]string, 0)
	for _, ns := range list.Items {
		var sec v1.Secret
		err := r.Get(context.TODO(), types.NamespacedName{
			Name:      name,
			Namespace: ns.Name,
		}, &sec)
		if err != nil {
			c.Add()
			continue
		}

		err = r.PatchSecrets(&sec, data)
		if err != nil {
			msg = append(msg, buildArrayErrMsg(err, sec.Name, ns.Name, "patch"))
			continue
		}
		c.Add()
	}
	if !c.Result() {
		return errors.New(strings.Join(msg, ","))
	}
	return nil

}

func (r *CmReconciler) UpdateAllNs(secrets, data string) error {
	var list v1.NamespaceList
	if err := r.List(context.TODO(), &list); err != nil {
		return errors.New(err.Error())
	}
	c := NewCounter(len(list.Items))
	msg := make([]string, 0)
	for _, ns := range list.Items {
		var sec *v1.Secret
		err := r.Get(context.TODO(), types.NamespacedName{
			Name:      secrets,
			Namespace: ns.Name,
		}, sec)
		if err != nil && kerror.IsNotFound(err) {
			sec = buildDockerRegistrySecrets(ns.Name, sec.Name, data)
			err = r.Create(context.TODO(), sec)
			if err != nil {
				msg = append(msg, buildArrayErrMsg(err, sec.Name, ns.Name, "patch-create"))
				continue
			}
			c.Add()
			continue
		}
		err = r.PatchSecrets(sec, data)
		if err != nil {
			msg = append(msg, buildArrayErrMsg(err, sec.Name, ns.Name, "patch"))
			return err
		}
		c.Add()
	}
	if !c.Result() {
		return errors.New(strings.Join(msg, ","))
	}
	return nil

}

func (r *CmReconciler) DeleteAllSecretsNameInNs(secrets string) error {

	var list v1.NamespaceList
	if err := r.List(context.TODO(), &list); err != nil {
		return errors.New(err.Error())
	}
	c := NewCounter(len(list.Items))
	msg := make([]string, 0)
	for _, ns := range list.Items {
		var sec v1.Secret
		err := r.Get(context.TODO(), types.NamespacedName{
			Name:      secrets,
			Namespace: ns.Name,
		}, &sec)
		if err != nil && kerror.IsNotFound(err) {
			c.Add()
			continue
		}
		err = r.Delete(context.TODO(), &sec)
		if err != nil {
			msg = append(msg, buildArrayErrMsg(err, ns.Name, sec.Name))
			return err
		}
		c.Add()
	}
	if !c.Result() {
		return errors.New(strings.Join(msg, ","))
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.

func (r *CmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		For(&v1.ConfigMap{}).
		WithEventFilter(cmPredicate{}).
		Complete(r)
}

//	type Predicate interface {
//		// Create returns true if the Create event should be processed
//		Create(event.CreateEvent) bool
//
//		// Delete returns true if the Delete event should be processed
//		Delete(event.DeleteEvent) bool
//
//		// Update returns true if the Update event should be processed
//		Update(event.UpdateEvent) bool
//
//		// Generic returns true if the Generic event should be processed
//		Generic(event.GenericEvent) bool
//	}
type cmPredicate struct{}

func (c cmPredicate) Create(createEvent event.CreateEvent) bool {
	//TODO implement me

	maps := createEvent.Object.GetLabels()
	if maps != nil {
		if _, ok := maps[common.DockerReg]; ok {
			return true
		}
	}
	return false
}

func (c cmPredicate) Delete(deleteEvent event.DeleteEvent) bool {
	//TODO implement me
	maps := deleteEvent.Object.GetLabels()
	if maps != nil {
		if _, ok := maps[common.DockerReg]; ok {
			return true
		}
	}
	return false
}

func (c cmPredicate) Update(updateEvent event.UpdateEvent) bool {
	//TODO implement me
	maps := updateEvent.ObjectNew.GetLabels()
	if maps != nil {
		if _, ok := maps[common.DockerReg]; ok {
			return true
		}
	}
	return false
}

func (c cmPredicate) Generic(genericEvent event.GenericEvent) bool {
	//TODO implement me
	maps := genericEvent.Object.GetLabels()
	if maps != nil {
		if _, ok := maps[common.DockerReg]; ok {
			return true
		}
	}
	return false
}
