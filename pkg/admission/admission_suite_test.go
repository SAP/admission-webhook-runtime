/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/sap/admission-webhook-runtime/pkg/admission"
)

func TestWebhooks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Suite")
}

var testEnv *envtest.Environment
var cfg *rest.Config
var ctx context.Context
var cancel context.CancelFunc
var clientset kubernetes.Interface
var recorder *Recorder

const testingNamespace = "testing"

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())
	var err error

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			ValidatingWebhooks: []*admissionv1.ValidatingWebhookConfiguration{
				buildValidatingWebhookConfiguration(),
			},
			MutatingWebhooks: []*admissionv1.MutatingWebhookConfiguration{
				buildMutatingWebhookConfiguration(),
			},
		},
	}
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	webhookInstallOptions := &testEnv.WebhookInstallOptions

	By("initializing kubernetes clientset")
	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	By("initializing webhooks scheme")
	scheme := runtime.NewScheme()
	err = corev1.AddToScheme(scheme)
	// add further api groups if needed
	Expect(err).NotTo(HaveOccurred())

	By("registering webhooks")
	err = admission.RegisterValidatingWebhook[*unstructured.Unstructured](&GenericWebhook{}, nil, log.Log)
	Expect(err).NotTo(HaveOccurred())
	err = admission.RegisterMutatingWebhook[*corev1.ConfigMap](&ConfigMapWebhook{}, scheme, log.Log)
	Expect(err).NotTo(HaveOccurred())
	// add further webhooks if needed

	By("starting webhook server")
	go func() {
		defer GinkgoRecover()
		options := &admission.ServeOptions{
			BindAddress: fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort),
			CertFile:    webhookInstallOptions.LocalServingCertDir + "/tls.crt",
			KeyFile:     webhookInstallOptions.LocalServingCertDir + "/tls.key",
		}
		err := admission.Serve(ctx, options)
		Expect(err).NotTo(HaveOccurred())
	}()

	By("waiting for webhook server to become ready")
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())

	recorder = &Recorder{}

	By("creating testing namespace")
	_, err = clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testingNamespace,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Webhooks", func() {
	Context("Generic Webhook", func() {
		Context("Positive tests", Ordered, func() {
			var name string

			BeforeEach(func() {
				name = "test1"
			})

			It("should be created", func() {
				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
				}
				_, err := clientset.CoreV1().ServiceAccounts(testingNamespace).Create(ctx, sa, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				seen := recorder.HasSeen("generic", actionValidateCreate, fmt.Sprintf("core/v1/ServiceAccount/%s/%s", testingNamespace, name))
				Expect(seen).To(Equal(true))
			})

			It("should be updated", func() {
				sa, err := clientset.CoreV1().ServiceAccounts(testingNamespace).Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = clientset.CoreV1().ServiceAccounts(testingNamespace).Update(ctx, sa, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())

				seen := recorder.HasSeen("generic", actionValidateUpdate, fmt.Sprintf("core/v1/ServiceAccount/%s/%s", testingNamespace, name))
				Expect(seen).To(Equal(true))
			})

			It("should be deleted", func() {
				err := clientset.CoreV1().ServiceAccounts(testingNamespace).Delete(ctx, name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				seen := recorder.HasSeen("generic", actionValidateDelete, fmt.Sprintf("core/v1/ServiceAccount/%s/%s", testingNamespace, name))
				Expect(seen).To(Equal(true))
			})
		})

		Context("Negative tests", Ordered, func() {
			var name string

			BeforeEach(func() {
				name = "test2"
			})

			It("should not be created", func() {
				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
						Annotations: map[string]string{
							"reject-create": "true",
						},
					},
				}
				_, err := clientset.CoreV1().ServiceAccounts(testingNamespace).Create(ctx, sa, metav1.CreateOptions{})
				Expect(err).To(HaveOccurred())

				seen := recorder.HasSeen("generic", actionValidateCreate, fmt.Sprintf("core/v1/ServiceAccount/%s/%s", testingNamespace, name))
				Expect(seen).To(Equal(true))
			})

			It("should not be updated", func() {
				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
						Annotations: map[string]string{
							"reject-update": "true",
							"reject-delete": "true",
						},
					},
				}
				sa, err := clientset.CoreV1().ServiceAccounts(testingNamespace).Create(ctx, sa, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = clientset.CoreV1().ServiceAccounts(testingNamespace).Update(ctx, sa, metav1.UpdateOptions{})
				Expect(err).To(HaveOccurred())

				seen := recorder.HasSeen("generic", actionValidateUpdate, fmt.Sprintf("core/v1/ServiceAccount/%s/%s", testingNamespace, name))
				Expect(seen).To(Equal(true))
			})

			It("should not be deleted", func() {
				err := clientset.CoreV1().ServiceAccounts(testingNamespace).Delete(ctx, name, metav1.DeleteOptions{})
				Expect(err).To(HaveOccurred())

				seen := recorder.HasSeen("generic", actionValidateDelete, fmt.Sprintf("core/v1/ServiceAccount/%s/%s", testingNamespace, name))
				Expect(seen).To(Equal(true))
			})
		})
	})

	Context("ConfigMap Webhook", Ordered, func() {
		var name string

		BeforeEach(func() {
			name = "test"
		})

		It("should mutate the configMap upon creation", func() {
			var err error
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			}
			cm, err = clientset.CoreV1().ConfigMaps(testingNamespace).Create(ctx, cm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, ok := cm.Annotations["created-at"]
			Expect(ok).To(Equal(true))
		})

		It("should mutate the configMap upon update", func() {
			cm, err := clientset.CoreV1().ConfigMaps(testingNamespace).Get(ctx, name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			cm.Data = map[string]string{"key": "value"}
			cm, err = clientset.CoreV1().ConfigMaps(testingNamespace).Update(ctx, cm, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, ok := cm.Annotations["updated-at"]
			Expect(ok).To(Equal(true))
		})
	})
})

// generic (validating) webhook
type GenericWebhook struct{}

var _ admission.ValidatingWebhook[*unstructured.Unstructured] = &GenericWebhook{}

func (w *GenericWebhook) ValidateCreate(ctx context.Context, object *unstructured.Unstructured) error {
	recorder.Record("generic", actionValidateCreate, object)
	if object.GetAnnotations()["reject-create"] == "true" {
		return fmt.Errorf("rejected as desired")
	}
	return nil
}

func (w *GenericWebhook) ValidateUpdate(ctx context.Context, oldObject *unstructured.Unstructured, newObject *unstructured.Unstructured) error {
	recorder.Record("generic", actionValidateUpdate, newObject)
	if newObject.GetAnnotations()["reject-update"] == "true" {
		return fmt.Errorf("rejected as desired")
	}
	return nil
}

func (w *GenericWebhook) ValidateDelete(ctx context.Context, object *unstructured.Unstructured) error {
	recorder.Record("generic", actionValidateDelete, object)
	if object.GetAnnotations()["reject-delete"] == "true" {
		return fmt.Errorf("rejected as desired")
	}
	return nil
}

// typed (mutating) webhook (for configmaps)
type ConfigMapWebhook struct{}

var _ admission.MutatingWebhook[*corev1.ConfigMap] = &ConfigMapWebhook{}

func (w *ConfigMapWebhook) MutateCreate(ctx context.Context, configMap *corev1.ConfigMap) error {
	recorder.Record("configmap", actionMutateCreate, configMap)
	if configMap.Annotations == nil {
		configMap.Annotations = make(map[string]string)
	}
	configMap.Annotations["created-at"] = time.Now().String()
	return nil
}

func (w *ConfigMapWebhook) MutateUpdate(ctx context.Context, oldConfigMap *corev1.ConfigMap, newConfigMap *corev1.ConfigMap) error {
	recorder.Record("configmap", actionMutateUpdate, newConfigMap)
	if newConfigMap.Annotations == nil {
		newConfigMap.Annotations = make(map[string]string)
	}
	newConfigMap.Annotations["updated-at"] = time.Now().String()
	return nil
}

// webhook invocation recorder
type Activity struct {
	Webhook   string
	Action    int
	Object    client.Object
	Timestamp time.Time
	Count     int
}

type Recorder struct {
	mutex      sync.Mutex
	activities map[string]*Activity
}

const (
	actionValidateCreate = iota
	actionValidateUpdate
	actionValidateDelete
	actionMutateCreate
	actionMutateUpdate
)

func (r *Recorder) Record(webhook string, action int, object client.Object) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := fmt.Sprintf("%s.%d.%s", webhook, action, objectKey(object))
	if r.activities == nil {
		r.activities = make(map[string]*Activity)
	}
	activity, ok := r.activities[key]
	if !ok {
		activity = &Activity{
			Webhook: webhook,
			Action:  action,
			Object:  object,
		}
		r.activities[key] = activity
	}
	activity.Timestamp = time.Now()
	activity.Count++
}

func (r *Recorder) HasSeen(webhook string, action int, objectKey string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := fmt.Sprintf("%s.%d.%s", webhook, action, objectKey)
	_, ok := r.activities[key]
	return ok
}

// assemble validatingwebhookconfiguration descriptor
func buildValidatingWebhookConfiguration() *admissionv1.ValidatingWebhookConfiguration {
	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "validate",
		},
		Webhooks: []admissionv1.ValidatingWebhook{{
			Name:                    "validate-generic.test.local",
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Path: &[]string{"/generic/validate"}[0],
				},
			},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{
					admissionv1.Create,
					admissionv1.Update,
					admissionv1.Delete,
				},
				Rule: admissionv1.Rule{
					APIGroups:   []string{"*"},
					APIVersions: []string{"*"},
					Resources:   []string{"*"},
				},
			}},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": testingNamespace,
				},
			},
			SideEffects: &[]admissionv1.SideEffectClass{admissionv1.SideEffectClassNone}[0],
		}},
	}
}

// assemble mutatingwebhookconfiguration descriptor
func buildMutatingWebhookConfiguration() *admissionv1.MutatingWebhookConfiguration {
	return &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutate",
		},
		Webhooks: []admissionv1.MutatingWebhook{{
			Name:                    "mutate-configmaps.test.local",
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Path: &[]string{"/core/v1/configmap/mutate"}[0],
				},
			},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{
					admissionv1.Create,
					admissionv1.Update,
				},
				Rule: admissionv1.Rule{
					APIGroups:   []string{""},
					APIVersions: []string{"v1"},
					Resources:   []string{"configmaps"},
				},
			}},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": testingNamespace,
				},
			},
			SideEffects: &[]admissionv1.SideEffectClass{admissionv1.SideEffectClassNone}[0],
		}},
	}
}

// get object key of arbitrary object
func objectKey(object client.Object) string {
	gvk := object.GetObjectKind().GroupVersionKind()

	group := gvk.Group
	if group == "" {
		group = "core"
	}
	version := gvk.Version
	kind := gvk.Kind
	namespace := object.GetNamespace()
	if namespace == "" {
		namespace = "-"
	}
	name := object.GetName()

	return strings.Join([]string{group, version, kind, namespace, name}, "/")
}
