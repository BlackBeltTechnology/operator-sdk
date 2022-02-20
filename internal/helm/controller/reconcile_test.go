// Copyright 2018 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	. "github.com/onsi/ginkgo"
	"github.com/operator-framework/operator-sdk/internal/helm/release"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	rpb "helm.sh/helm/v3/pkg/release"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"os"
	"path"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"
)

func TestHasAnnotation(t *testing.T) {
	upgradeForceTests := []struct {
		input       map[string]interface{}
		expectedVal bool
		expectedOut string
		name        string
	}{
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/upgrade-force": "True",
			},
			expectedVal: true,
			name:        "upgrade force base case true",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/upgrade-force": "False",
			},
			expectedVal: false,
			name:        "upgrade force base case false",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/upgrade-force": "1",
			},
			expectedVal: true,
			name:        "upgrade force true as int",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/upgrade-force": "0",
			},
			expectedVal: false,
			name:        "upgrade force false as int",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/wrong-annotation": "true",
			},
			expectedVal: false,
			name:        "upgrade force annotation not set",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/upgrade-force": "invalid",
			},
			expectedVal: false,
			name:        "upgrade force invalid value",
		},
	}

	for _, test := range upgradeForceTests {
		assert.Equal(t, test.expectedVal, hasAnnotation(helmUpgradeForceAnnotation, annotations(test.input)), test.name)
	}

	uninstallWaitTests := []struct {
		input       map[string]interface{}
		expectedVal bool
		expectedOut string
		name        string
	}{
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/uninstall-wait": "True",
			},
			expectedVal: true,
			name:        "uninstall wait base case true",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/uninstall-wait": "False",
			},
			expectedVal: false,
			name:        "uninstall wait base case false",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/uninstall-wait": "1",
			},
			expectedVal: true,
			name:        "uninstall wait true as int",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/uninstall-wait": "0",
			},
			expectedVal: false,
			name:        "uninstall wait false as int",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/wrong-annotation": "true",
			},
			expectedVal: false,
			name:        "uninstall wait annotation not set",
		},
		{
			input: map[string]interface{}{
				"helm.sdk.operatorframework.io/uninstall-wait": "invalid",
			},
			expectedVal: false,
			name:        "uninstall wait invalid value",
		},
	}

	for _, test := range uninstallWaitTests {
		assert.Equal(t, test.expectedVal, hasAnnotation(helmUninstallWaitAnnotation, annotations(test.input)), test.name)
	}
}

func annotations(m map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": m,
			},
		},
	}
}
func newManager() (manager.Manager, error) {

	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		return nil, err
	}
	log.Info("created manager", "manager", mgr)
	return mgr, nil
}

var testenv *envtest.Environment
var cfg *rest.Config
var clientset *kubernetes.Clientset

func startK() error {

	testenv = &envtest.Environment{}
	cwd, _ := os.Getwd()
	testenv.BinaryAssetsDirectory = cwd + "/../../../tools/bin/k8s/1.21.2-linux-amd64"
	log.Info("cwd: ", "msg", cwd)

	var err error
	cfg, err = testenv.Start()
	if err != nil {
		return err
	}
	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	// Prevent the metrics listener being created
	metrics.DefaultBindAddress = "0"

	return nil
}

func setup(tb *testing.T) func(tb *testing.T) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	logf.Log.Info("setup suite")
	err := startK()
	if err != nil {
		tb.Fatalf("cannot start kube: %v", err)
	}

	// Return a function to teardown the test
	return func(tb *testing.T) {
		logf.Log.Info("teardown suite")
		err := testenv.Stop()
		if err != nil {
			tb.Fatalf("cannot stop kube: %v", err)
		}
		metrics.DefaultBindAddress = ":8080"

	}
}

func TestHelmOperatorReconciler_Reconcile(t *testing.T) {
	tearDown := setup(t)
	defer tearDown(t)
	gvk := schema.GroupVersionKind{
		Kind:    "ConfigMap",
		Group:   "",
		Version: "v1",
	}
	// create and return configmap object
	pathPrefix := "../../../testdata/helm/configmap"
	overrideFileName := "values-override.yaml"
	content, err := ioutil.ReadFile(path.Join(pathPrefix, overrideFileName))
	if err != nil {
		t.Fatalf("cannot load valuesoverride file: %v", err)
	}
	values := make(map[string]interface{})
	err = yaml.Unmarshal(content, values)
	if err != nil {
		t.Fatalf("values-override.yaml yaml error: %v", err)
	}
	mmm, err := yaml.Marshal(values)
	cfgmap := &v1.ConfigMap{

		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"app": "foo"},
		},
		Data: map[string]string{"values": string(mmm)},
	}
	marshalled, err := yaml.Marshal(cfgmap)

	t.Logf("cfgmap: \n%s", marshalled)
	_, err = clientset.CoreV1().ConfigMaps(cfgmap.GetNamespace()).Create(context.Background(), cfgmap, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test configmap: %v", err)
	}

	mgr, errr := newManager()
	if errr != nil {
		t.Fatalf("failed to create controller-manager: %v", errr)
	}
	type fields struct {
		Client          client.Client
		EventRecorder   record.EventRecorder
		GVK             schema.GroupVersionKind
		ManagerFactory  release.ManagerFactory
		ReconcilePeriod time.Duration
		OverrideValues  map[string]string
		releaseHook     ReleaseHookFunc
	}
	type args struct {
		ctx     context.Context
		request reconcile.Request
	}
	reconcilePeriod := 5 * time.Second
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "configmap test",
			fields: fields{
				Client:          mgr.GetClient(),
				EventRecorder:   mgr.GetEventRecorderFor(gvk.Kind),
				GVK:             gvk,
				ManagerFactory:  release.NewManagerFactory(mgr, "../../../testdata/helm/configmap/helm-chart"),
				ReconcilePeriod: reconcilePeriod,
				OverrideValues:  map[string]string{},
				releaseHook: func(r *rpb.Release) error {
					return nil
				},
			}, args: args{
				ctx: context.TODO(),
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      cfgmap.GetName(),
						Namespace: cfgmap.GetNamespace(),
					},
				},
			}, want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: reconcilePeriod,
			}, wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				if err != nil {
					t.Errorf("bogus reconcile")
				}
				return err != nil
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := HelmOperatorReconciler{
				Client:          tt.fields.Client,
				EventRecorder:   tt.fields.EventRecorder,
				GVK:             tt.fields.GVK,
				ManagerFactory:  tt.fields.ManagerFactory,
				ReconcilePeriod: tt.fields.ReconcilePeriod,
				OverrideValues:  tt.fields.OverrideValues,
				releaseHook:     tt.fields.releaseHook,
			}
			got, err := r.Reconcile(tt.args.ctx, tt.args.request)
			//if !tt.wantErr(t, err, fmt.Sprintf("Reconcile(%v, %v)", tt.args.ctx, tt.args.request)) {
			//	return
			//}
			if err != nil {
				t.Fatalf("bogus reconcile: %v", err)
			}
			assert.Equalf(t, tt.want, got, "Reconcile(%v, %v)", tt.args.ctx, tt.args.request)
			var updated *v1.ConfigMap
			updated, err = clientset.CoreV1().ConfigMaps(cfgmap.Namespace).Get(context.Background(), cfgmap.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("cannot get cfgmap back: %v", err)
			}
			assert.Contains(t, updated.Annotations, "deploymentStatus")
			if content, err := yaml.Marshal(updated); err == nil {
				t.Logf("updated: \n%s", content)
			} else {
				t.Fatalf("cannot render updated: %v", err)
			}

		})
	}
}
